package network

type ContainerEndpointConfig struct {
	ID           string
	Pid          int
	IPAddress    string
	PortMappings []string
}
