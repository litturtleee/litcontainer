//go:build !linux || !cgo

package container

import (
	"fmt"
	"github.com/urfave/cli"
)

// ExecContainer 运行容器中的进程
// 这里是为了方便编译的空实现
func ExecContainer(args cli.Args) error {
	return fmt.Errorf("exec is only supported on linux with cgo enabled")
}
