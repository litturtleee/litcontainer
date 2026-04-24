//go:build !linux || !cgo

package container

import (
	"fmt"
)

// ExecContainer 运行容器中的进程
// 这里是为了方便编译的空实现
func ExecContainer(args []string) error {
	return fmt.Errorf("exec is only supported on linux with cgo enabled")
}
