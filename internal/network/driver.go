package network

import "net"

type Driver interface {
	Name() string
	Create(name string, subnet *net.IPNet) (*Network, error)
	Delete(network *Network) error
	Connect(network *Network, endpoint *Endpoint) error
	Disconnect(network *Network, endpoint *Endpoint) error
}

var drivers = make(map[string]Driver)

func RegisterDriver(name string, driver Driver) {
	drivers[name] = driver
}

func GetDriver(name string) (Driver, error) {
	driver, ok := drivers[name]
	if !ok {
		return nil, ErrNetworkDriverNotFound
	}
	return driver, nil
}
