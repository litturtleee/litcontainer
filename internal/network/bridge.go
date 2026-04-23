package network

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"litcontainer/internal/logger"
	"net"
	"strings"
)

const BridgePeerNamePrefix = "br-peer-"

type BridgeDriver struct {
}

func init() {
	RegisterDriver("bridge", &BridgeDriver{})
}

func (b *BridgeDriver) Name() string {
	return "bridge"
}

// Create
// 1.创建一个Network对象，包括网络名称、IP范围和驱动类型
// 2.调用init方法初始化网络
// 3.返回创建的网络对象
func (b *BridgeDriver) Create(name string, subnet *net.IPNet) (*Network, error) {
	nw := &Network{
		Name:    name,
		IpRange: subnet,
		Driver:  b.Name(),
	}
	if err := b.init(nw); err != nil {
		logger.Error("init network %s failed: %v", nw.Name, err)
		return nil, err
	}
	return nw, nil
}

func (b *BridgeDriver) Delete(network *Network) error {
	link, err := netlink.LinkByName(network.Name)
	if err != nil {
		logger.Error("get bridge %s failed: %v", network.Name, err)
		return fmt.Errorf("failed to delete network %s, %w", network.Name, err)
	}
	_, ipNet, err := net.ParseCIDR(network.IpRange.String())
	if err != nil {
		logger.Error("parse cidr %s failed: %v", network.IpRange.String(), err)
		return fmt.Errorf("parse cidr %s failed: %w", network.IpRange.String(), err)
	}
	// 删除iptables规则
	if err := DeleteSNATIptables(network.Name, ipNet.String()); err != nil {
		logger.Error("delete iptables rule for bridge %s failed: %v", network.Name, err)
		return fmt.Errorf("delete iptables rule for bridge %s failed: %w", network.Name, err)
	}
	// 删除link
	return netlink.LinkDel(link)
}

func (b *BridgeDriver) Connect(network *Network, endpoint *Endpoint) error {
	bridgeName := network.Name
	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		logger.Error("get bridge %s failed: %v", bridgeName, err)
		return fmt.Errorf("get bridge %s failed: %w", bridgeName, err)
	}

	// 创建veth对
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "veth" + endpoint.ID[:5],
		},
		PeerName: BridgePeerNamePrefix + endpoint.ID[:5],
	}
	endpoint.Device = veth
	logger.Debug("endpoint name: %s, endpoint id: %s", veth.Name, endpoint.ID)

	if err := netlink.LinkAdd(veth); err != nil {
		logger.Error("add veth pair failed: %v", err)
		return fmt.Errorf("add veth pair failed: %w", err)
	}

	// 一端连接到bridge并setup
	err = netlink.LinkSetMaster(veth, br)
	if err != nil {
		logger.Error("link veth pair failed: %v", err)
		return fmt.Errorf("link veth pair failed: %w", err)
	}
	err = netlink.LinkSetUp(veth)
	if err != nil {
		logger.Error("set interface %s up failed: %v", veth.Name, err)
		return fmt.Errorf("set interface %s up failed: %w", veth.Name, err)
	}

	return nil
}

func (b *BridgeDriver) Disconnect(network *Network, endpoint *Endpoint) error {
	vethName := "veth" + endpoint.ID[:5]

	link, err := netlink.LinkByName(vethName)
	if err != nil {
		if _, ok := err.(netlink.LinkNotFoundError); ok {
			logger.Debug("veth link %s not found", vethName)
			return nil
		}
		return fmt.Errorf("get veth link failed: %w", err)
	}

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("delete veth link failed: %w", err)
	}

	logger.Info("Disconnected endpoint %s from network %s, del link %s", endpoint.ID, network.Name, vethName)
	return nil
}

// init 创建bridge网络
func (b *BridgeDriver) init(network *Network) error {
	// 1.创建bridge网络
	if err := createBridge(network.Name); err != nil {
		logger.Error("create bridge %s failed: %v", network.Name, err)
		return fmt.Errorf("create bridge %s failed: %w", network.Name, err)
	}
	// 2.设置bridge网络接口的IP地址
	if err := SetInterfaceIP(network.Name, network.IpRange.String()); err != nil {
		logger.Error("set ip range %s to bridge %s failed: %v", network.IpRange.String(), network.Name, err)
		return fmt.Errorf("set ip range %s to bridge %s failed: %w", network.IpRange.String(), network.Name, err)
	}
	// 3.设置bridge网络接口为up状态
	if err := SetInterfaceUp(network.Name); err != nil {
		logger.Error("set bridge %s up failed: %v", network.Name, err)
		return fmt.Errorf("set bridge %s up failed: %w", network.Name, err)
	}
	// 4.配置iptables的NAT规则
	// 去掉主机号
	_, ipNet, err := net.ParseCIDR(network.IpRange.String())
	if err != nil {
		logger.Error("parse cidr %s failed: %v", network.IpRange.String(), err)
		return fmt.Errorf("parse cidr %s failed: %w", network.IpRange.String(), err)
	}
	err = SetupSNATIptables(network.Name, ipNet.String())
	if err != nil {
		logger.Error("setup SNAT iptables for bridge %s failed: %v", network.Name, err)
		return fmt.Errorf("setup SNAT iptables for bridge %s failed: %w", network.Name, err)
	}
	return nil
}

func createBridge(name string) error {
	inter, err := net.InterfaceByName(name)
	if inter != nil {
		logger.Debug("bridge %s exists", name)
		return nil
	}
	if err != nil && !strings.Contains(err.Error(), "no such network interface") {
		logger.Error("get bridge %s failed: %v", name, err)
		return err
	}

	// 创建一个新的网络链接属性对象
	la := netlink.NewLinkAttrs()
	la.Name = name
	// 创建一个Bridge对象，表示一个网桥网络接口，并将之前配置的属性应用到对象上
	bridge := &netlink.Bridge{LinkAttrs: la}
	// 使用netlink库将创建的网桥添加到系统中(ip link add <name> type bridge)
	if err := netlink.LinkAdd(bridge); err != nil {
		logger.Error("add bridge %s failed: %v", name, err)
		return err
	}
	return nil
}
