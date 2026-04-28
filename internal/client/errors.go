package client

import "errors"

var (
	ErrInvalidArguments  = errors.New("invalid arguments")
	ErrDaemonUnreachable = errors.New("daemon unreachable")
)
