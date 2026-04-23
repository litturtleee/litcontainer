package network

import "errors"

var (
	ErrNetworkNotFound       = errors.New("network not found")
	ErrNetworkExists         = errors.New("network exists")
	ErrNetworkDriverNotFound = errors.New("network driver not found")
)
