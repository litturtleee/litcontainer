package container

import "errors"

var (
	ErrInitInvalidArgs = errors.New("init invalid arguments")

	ErrContainerNotFound   = errors.New("container not found")
	ErrContainerNotRunning = errors.New("container not running")
	ErrContainerIsRunning  = errors.New("container is running")
)
