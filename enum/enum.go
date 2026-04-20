package enum

const (
	AppName    = "litcontainer"
	AppVersion = "0.0.1"
	AppUsage   = "A lightweight container runtime"

	ContainerRunningState = "running"
	ContainerStoppedState = "stopped"

	DefaultNetworkDBPath = "/var/lib/litcontainer/network/files/local-kv.db"

	DefaultNetworkTable = "litcontainer_network"
	AllocatedIPKeyTable = "allocated_ip"
)
