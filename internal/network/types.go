package network

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"net"
)

const InterfaceLoName = "lo"

// Network
// opt: 增删查网络
type Network struct {
	Name    string     `json:"name"`
	IpRange *net.IPNet `json:"ipRange"`
	Driver  string     `json:"driver"`
}

func (n *Network) String() string {
	return fmt.Sprintf("{Name:%s IpRange:%s Driver:%s}", n.Name, n.IpRange, n.Driver)
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

// ContainerEndpointConfig 容器连接网络需要的信息
type ContainerEndpointConfig struct {
	ID           string
	Pid          int
	IPAddress    string
	PortMappings []string
}
