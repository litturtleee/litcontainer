package network

import (
	"encoding/json"
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"litcontainer/config"
	"litcontainer/enum"
	"litcontainer/pkg/db"
	"litcontainer/pkg/logger"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"
)

const InterfaceLoName = "lo"

// Network
// opt: 增删查网络
type Network struct {
	Name    string     `json:"name"`
	IpRange *net.IPNet `json:"ipRange"`
	Driver  string     `json:"driver"`
}

// Endpoint
// opt: 连接、断开到网络
type Endpoint struct {
	ID          string           `json:"id"`
	Device      *netlink.Veth    `json:"device"`
	IPAddress   net.IP           `json:"ip"`
	MACAddress  net.HardwareAddr `json:"mac"`
	Network     *Network         `json:"network"`
	PortMapping []string         `json:"portMapping"`
}

// CreateNetwork
// 1.解析用户输入的子网信息，确保格式
// 2.调用指定的网络驱动创建网络
// 3.将网络信息保存到数据库中，便于后续管理
func CreateNetwork(name string, driverType string, subnet string) error {
	nw, err := GetNetworkFromDB(name)
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return err
	}
	if nw != nil {
		logger.Error("network %s already exists", name)
		return ErrNetworkExists
	}
	driver, err := GetDriver(driverType)
	if err != nil {
		logger.Error("get driver %s failed: %v", driver, err)
		return err
	}

	// 解析subnet
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		logger.Error("parse subnet failed: %v", err)
		return err
	}
	// 在子网中分配IP
	allocatedIP, err := AllocateIP(ipNet)
	if err != nil {
		logger.Error("allocate ip failed: %v", err)
		return err
	}
	ipNet.IP = allocatedIP
	nw, err = driver.Create(name, ipNet)
	if err != nil {
		logger.Error("create network %s failed: %v", name, err)
		return err
	}
	logger.Info("create network %s success", name)
	return nw.Save()
}

func ListNetworks() error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "NAME\tIPRANGE\tDRIVER\n")
	var datas map[string][]byte
	err := db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		var getAllErr error
		datas, getAllErr = dbClient.GetAll(enum.DefaultNetworkTable)
		return getAllErr
	})
	if err != nil {
		logger.Error("get all network from db failed: %v", err)
		return err
	}
	for _, data := range datas {
		var network Network
		err = json.Unmarshal(data, &network)
		if err != nil {
			logger.Error("unmarshal network failed: %v", err)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", network.Name, network.IpRange, network.Driver)
	}
	if err := w.Flush(); err != nil {
		logger.Error("flush w failed: %v", err)
		return err
	}
	return nil
}

func DeleteNetwork(name string) error {
	nw, err := GetNetworkFromDB(name)
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return err
	}
	if nw == nil {
		logger.Error("network %s not found", name)
		return ErrNetworkNotFound
	}
	// 释放ip
	err = ReleaseIP(nw.IpRange, nw.IpRange.IP)
	if err != nil {
		logger.Error("release ip failed: %v", err)
		return err
	}

	driver, err := GetDriver(nw.Driver)
	if err != nil {
		logger.Error("get driver %s failed: %v", nw.Driver, err)
		return err
	}
	err = driver.Delete(nw)
	if err != nil {
		logger.Error("delete network %s failed: %v", name, err)
		return err
	}

	return nw.Delete()
}

func GetNetworkFromDB(name string) (*Network, error) {
	var jsonStr []byte
	err := db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		var getErr error
		jsonStr, getErr = dbClient.Get(enum.DefaultNetworkTable, name)
		return getErr
	})
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return nil, err
	}
	if jsonStr == nil {
		logger.Warn("network %s not found", name)
		return nil, nil
	}
	var network Network
	err = json.Unmarshal(jsonStr, &network)
	if err != nil {
		logger.Error("unmarshal network from db failed: %v", err)
		return nil, err
	}
	return &network, nil
}

func (n *Network) Save() error {
	networkStr, err := json.Marshal(n)
	if err != nil {
		logger.Error("marshal network failed: %v", err)
		return err
	}
	if err := db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		return dbClient.Put(enum.DefaultNetworkTable, n.Name, networkStr)
	}); err != nil {
		logger.Error("put network to db failed: %v", err)
		return err
	}
	return nil
}

func (n *Network) Delete() error {
	err := db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		return dbClient.Delete(enum.DefaultNetworkTable, n.Name)
	})
	if err != nil {
		logger.Error("delete network %s from db failed: %v", n.Name, err)
		return err
	}
	return nil
}

func Connect(name string, containerConfig *config.ContainerConfig) (*net.IP, error) {
	nw, err := GetNetworkFromDB(name)
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return nil, err
	}
	if nw == nil {
		logger.Error("network %s not found", name)
		return nil, fmt.Errorf("network %s not found", name)
	}

	// 解析子网
	_, sub, err := net.ParseCIDR(nw.IpRange.String())
	if err != nil {
		logger.Error("parse subnet failed: %v", err)
		return nil, err
	}
	allocatedIP, err := AllocateIP(sub)
	if err != nil {
		logger.Error("allocate ip failed: %v", err)
		return nil, err
	}
	logger.Info("Connect network: %v, ip: %s", nw, allocatedIP.String())

	// 创建endpoint
	endpoint := &Endpoint{
		ID:          fmt.Sprintf("%s-%s", containerConfig.Id, name),
		IPAddress:   allocatedIP,
		Network:     nw,
		PortMapping: containerConfig.PortMappings,
	}

	// 驱动挂载端点
	driver, err := GetDriver(nw.Driver)
	if err != nil {
		logger.Error("get driver %s failed: %v", nw.Driver, err)
		return nil, err
	}
	// 创建pair挂载到网桥
	err = driver.Connect(nw, endpoint)
	if err != nil {
		logger.Error("connect network %s failed: %v", name, err)
		return nil, err
	}
	// 在容器命名空间中配置网络
	err = configEndpointNetwork(endpoint, containerConfig)
	if err != nil {
		logger.Error("config endpoint network failed: %v", err)
		return nil, err
	}

	// 在宿主ns设置iptables
	err = configPortMapping(endpoint, containerConfig)
	if err != nil {
		logger.Error("config port mapping failed: %v", err)
		return nil, err
	}
	logger.Info("connect container %+v to endpoint %+v success", *containerConfig, *endpoint)
	return &allocatedIP, nil
}

// Disconnect
// 当前项目容器停止就应该做这些
func Disconnect(name string, containerConfig *config.ContainerConfig) error {
	nw, err := GetNetworkFromDB(name)
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return err
	}
	if nw == nil {
		logger.Error("network %s not found", name)
		return fmt.Errorf("network %s not found", name)
	}

	driver, err := GetDriver(nw.Driver)
	if err != nil {
		logger.Error("get driver %s failed: %v", nw.Driver, err)
		return err
	}
	ep := &Endpoint{
		ID:          fmt.Sprintf("%s-%s", containerConfig.Id, name),
		IPAddress:   net.ParseIP(containerConfig.IpAddress),
		Network:     nw,
		PortMapping: containerConfig.PortMappings,
	}

	// 删除iptables
	// 不需要进容器解除配置，因为容器子进程都退出了
	err = unConfigPortMapping(ep, containerConfig)
	if err != nil {
		logger.Error("unconfig port mapping failed: %v", err)
		return err
	}

	// 删除设备
	err = driver.Disconnect(nw, ep)
	if err != nil {
		logger.Error("disconnect network %s failed: %v", name, err)
		return err
	}

	// 释放IP
	_, ipNet, _ := net.ParseCIDR(nw.IpRange.String())
	err = ReleaseIP(ipNet, ep.IPAddress)
	if err != nil {
		logger.Error("release ip failed: %v", err)
		return err
	}
	logger.Info("disconnect container %+v from endpoint %+v success", *containerConfig, *ep)
	return nil
}

// --- 内部方法 ---

func configEndpointNetwork(endpoint *Endpoint, containerConfig *config.ContainerConfig) error {
	// 获取对端
	maxRetries := 3
	var peerLink netlink.Link
	var err error
	for i := 0; i < maxRetries; i++ {
		peerLink, err = netlink.LinkByName(endpoint.Device.PeerName)
		if err != nil {
			logger.Warn("get peer link failed: %v, sleep 1s then try", err)
			time.Sleep(time.Second)
			continue
		}
		break
	}
	if err != nil {
		logger.Error("get peer link failed: %v", err)
		return err
	}
	// 把对端设置到容器内(defer才会推出ns)
	defer configNetNs(&peerLink, containerConfig)()
	// 设置ip
	ip := net.IPNet{
		IP:   endpoint.IPAddress,
		Mask: endpoint.Network.IpRange.Mask,
	}
	err = SetInterfaceIP(endpoint.Device.PeerName, ip.String())
	if err != nil {
		logger.Error("set interface ip failed: %v", err)
		return err
	}
	// 设置up
	err = SetInterfaceUp(endpoint.Device.PeerName)
	if err != nil {
		logger.Error("set %s interface up failed: %v", endpoint.Device.PeerName, err)
		return err
	}
	// 设置回环up
	err = SetInterfaceUp(InterfaceLoName)
	if err != nil {
		logger.Error("set %s interface up failed: %v", InterfaceLoName, err)
		return err
	}

	// 添加默认路由
	_, ipNet, _ := net.ParseCIDR("0.0.0.0/0")
	// 所有请求都走网关从peer设备
	defaultRoute := &netlink.Route{
		LinkIndex: peerLink.Attrs().Index,
		Gw:        endpoint.Network.IpRange.IP,
		Dst:       ipNet,
	}
	err = netlink.RouteAdd(defaultRoute)
	if err != nil {
		logger.Error("add default route failed: %v", err)
		return err
	}

	return nil
}

// configNetNs
// 锁定线程将设备切换到目标ns中
// 将当前线程设置到目标命名空间
func configNetNs(peerLink *netlink.Link, containerConfig *config.ContainerConfig) func() {
	file, err := os.OpenFile(fmt.Sprintf("/proc/%d/ns/net", containerConfig.Pid), os.O_RDONLY, 0)
	if err != nil {
		logger.Error("open net ns failed: %v", err)
		return nil
	}
	nsFd := file.Fd()

	// 锁定线程
	runtime.LockOSThread()

	err = netlink.LinkSetNsFd(*peerLink, int(nsFd))
	if err != nil {
		logger.Error("set link to net ns failed: %v", err)
		return nil
	}

	orignNs, err := netns.Get()
	if err != nil {
		logger.Error("get current net ns failed: %v", err)
		return nil
	}

	err = netns.Set(netns.NsHandle(nsFd))
	if err != nil {
		logger.Error("set net ns failed: %v", err)
		return nil
	}

	return func() {
		// 恢复ns命名空间
		err = netns.Set(orignNs)
		if err != nil {
			logger.Error("set net ns failed: %v", err)
		}
		err = orignNs.Close()
		if err != nil {
			logger.Error("close net ns failed: %v", err)
		}
		// 恢复线程
		runtime.UnlockOSThread()
		err = file.Close()
		if err != nil {
			logger.Error("close net ns file failed: %v", err)
		}
	}
}

// configPortMapping
// 实现DNAT
func configPortMapping(endpoint *Endpoint, config *config.ContainerConfig) error {
	for _, pm := range config.PortMappings {
		splits := strings.Split(pm, ":")
		if len(splits) != 2 {
			logger.Error("invalid port mapping: %s", pm)
			continue
		}
		iptableCmd := fmt.Sprintf("-t nat -A PREROUTING -p tcp -m tcp --dport %s -j DNAT --to-destination %s:%s",
			splits[0], endpoint.IPAddress.String(), splits[1])
		output, err := exec.Command("iptables", strings.Split(iptableCmd, " ")...).Output()
		if err != nil {
			logger.Error("add iptables rule error:", err, "output:", output)
			continue
		}
		logger.Info("add iptables rule: %s", output)
	}
	return nil
}

// unConfigPortMapping
// 删除设备时删除iptables规则
func unConfigPortMapping(endpoint *Endpoint, config *config.ContainerConfig) error {
	for _, pm := range config.PortMappings {
		splits := strings.Split(pm, ":")
		if len(splits) != 2 {
			logger.Error("invalid port mapping: %s", pm)
			continue
		}
		iptableCmd := fmt.Sprintf("-t nat -D PREROUTING -p tcp -m tcp --dport %s -j DNAT --to-destination %s:%s",
			splits[0], endpoint.IPAddress.String(), splits[1])
		output, err := exec.Command("iptables", strings.Split(iptableCmd, " ")...).Output()
		if err != nil {
			logger.Error("delete iptables rule error:", err, "output:", output)
			continue
		}
		logger.Info("delete iptables rule success, portMapping:%v", pm)
	}
	return nil
}
