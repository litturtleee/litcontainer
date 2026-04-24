package network

import (
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
