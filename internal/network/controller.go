package network

import (
	"encoding/json"
	"fmt"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"litcontainer/internal/db"
	"litcontainer/internal/logger"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	DefaultNetworkDBPath = "/var/lib/litcontainer/network/files/local-kv.db"

	DefaultNetworkTable = "litcontainer_network"
	AllocatedIPKeyTable = "allocated_ip"
)

var defaultController *Controller

func Init(dbPath string) error {
	c, err := NewController(dbPath)
	if err != nil {
		return err
	}
	err = c.db.CreateBucketIfNotExists(DefaultNetworkTable)
	if err != nil {
		logger.Error("init bolt db failed: %v", err)
		return err
	}
	err = c.db.CreateBucketIfNotExists(AllocatedIPKeyTable)
	if err != nil {
		logger.Error("init bolt db failed: %v", err)
		return err
	}
	defaultController = c
	return nil
}

func GetController() *Controller {
	if defaultController == nil {
		panic("network controller not initialized")
	}
	return defaultController
}

type Controller struct {
	db *db.BoltDB
	mu sync.RWMutex
}

func NewController(dbPath string) (*Controller, error) {
	boltDB, err := db.NewBoltDB(dbPath)
	if err != nil {
		return nil, err
	}
	return &Controller{db: boltDB}, nil
}

func (c *Controller) Close() error {
	return c.db.Close()
}

// Create
// 1.解析用户输入的子网信息，确保格式
// 2.调用指定的网络驱动创建网络
// 3.将网络信息保存到数据库中，便于后续管理
func (c *Controller) Create(name, driverType, subnet string) error {
	nw, err := c.Get(name)
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
	allocatedIP, err := c.allocateIP(ipNet)
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

	// 存表
	nwStr, _ := json.Marshal(nw)
	return c.db.Put(DefaultNetworkTable, name, nwStr)
}
func (c *Controller) Delete(name string) error {
	nw, err := c.Get(name)
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return err
	}
	if nw == nil {
		logger.Error("network %s not found", name)
		return ErrNetworkNotFound
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

	err = c.db.Delete(DefaultNetworkTable, name)
	if err != nil {
		logger.Error("delete network %s failed: %v", name, err)
		return err
	}
	_, cidrNet, _ := net.ParseCIDR(nw.IpRange.String())
	return c.db.Delete(AllocatedIPKeyTable, cidrNet.String())
}
func (c *Controller) List() ([]*Network, error) {
	datas, err := c.db.GetAll(DefaultNetworkTable)
	if err != nil {
		logger.Error("get all network from db failed: %v", err)
		return nil, err
	}
	networks := make([]*Network, 0, len(datas))
	for _, data := range datas {
		var network Network
		err = json.Unmarshal(data, &network)
		if err != nil {
			logger.Error("unmarshal network failed: %v", err)
			continue
		}
		networks = append(networks, &network)
	}

	return networks, nil
}

// 原 GetNetworkFromDB 改名
func (c *Controller) Get(name string) (*Network, error) {
	jsonStr, err := c.db.Get(DefaultNetworkTable, name)
	if err != nil {
		logger.Error("get network from db failed: %v", err)
		return nil, err
	}
	if jsonStr == nil {
		logger.Debug("network %s not found", name)
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
func (c *Controller) Connect(name string, epCfg *ContainerEndpointConfig) (*net.IP, error) {
	nw, err := c.Get(name)
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
	allocatedIP, err := c.allocateIP(sub)
	if err != nil {
		logger.Error("allocate ip failed: %v", err)
		return nil, err
	}
	logger.Info("Connect network: %v, ip: %s", nw, allocatedIP.String())

	// 创建endpoint
	endpoint := &Endpoint{
		ID:          epCfg.ID,
		IPAddress:   allocatedIP,
		Network:     nw,
		PortMapping: epCfg.PortMappings,
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
	err = configEndpointNetwork(endpoint, epCfg)
	if err != nil {
		logger.Error("serverconfig endpoint network failed: %v", err)
		return nil, err
	}

	// 在宿主ns设置iptables
	err = configPortMapping(endpoint, epCfg)
	if err != nil {
		logger.Error("serverconfig port mapping failed: %v", err)
		return nil, err
	}
	logger.Info("connect container %+v to endpoint %+v success", *epCfg, *endpoint)
	return &allocatedIP, nil
}
func (c *Controller) Disconnect(name string, epCfg *ContainerEndpointConfig) error {
	nw, err := c.Get(name)
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
		ID:          epCfg.ID,
		IPAddress:   net.ParseIP(epCfg.IPAddress),
		Network:     nw,
		PortMapping: epCfg.PortMappings,
	}

	// 删除iptables
	// 不需要进容器解除配置，因为容器子进程都退出了
	err = unConfigPortMapping(ep, epCfg)
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
	err = c.releaseIP(ipNet, ep.IPAddress)
	if err != nil {
		logger.Error("release ip failed: %v", err)
		return err
	}
	logger.Info("disconnect container %+v from endpoint %+v success", *epCfg, *ep)
	return nil
}

// --- 内部方法 ---

func configEndpointNetwork(endpoint *Endpoint, epCfg *ContainerEndpointConfig) error {
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
	cleanup, err := configNetNs(&peerLink, epCfg)
	if err != nil {
		logger.Error("config net ns failed: %v", err)
		return err
	}
	defer cleanup()
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
func configNetNs(peerLink *netlink.Link, epc *ContainerEndpointConfig) (func(), error) {
	netNsPath := filepath.Join(NetnsRootDir, epc.ID)
	file, err := os.OpenFile(netNsPath, os.O_RDONLY, 0)
	if err != nil {
		logger.Error("open net ns failed: %v", err)
		return nil, err
	}
	nsFd := file.Fd()

	// 锁定线程
	runtime.LockOSThread()

	cleanup := func() {
		runtime.UnlockOSThread()
		file.Close()
	}
	err = netlink.LinkSetNsFd(*peerLink, int(nsFd))
	if err != nil {
		logger.Error("set link to net ns failed: %v", err)
		cleanup()
		return nil, err
	}

	orignNs, err := netns.Get()
	if err != nil {
		logger.Error("get current net ns failed: %v", err)
		cleanup()
		return nil, err
	}

	cleanup = func() {
		netns.Set(orignNs)
		orignNs.Close()
		runtime.UnlockOSThread()
		file.Close()
	}
	err = netns.Set(netns.NsHandle(nsFd))
	if err != nil {
		logger.Error("set net ns failed: %v", err)
		cleanup()
		return nil, err
	}

	return cleanup, nil
}

// configPortMapping
// 实现DNAT
func configPortMapping(endpoint *Endpoint, epCfg *ContainerEndpointConfig) error {
	for _, pm := range epCfg.PortMappings {
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
func unConfigPortMapping(endpoint *Endpoint, epCfg *ContainerEndpointConfig) error {
	for _, pm := range epCfg.PortMappings {
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
