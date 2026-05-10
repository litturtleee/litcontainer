package network

import (
	"fmt"
	"litcontainer/internal/logger"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

const NetnsRootDir = "/run/litcontainer/netns"

// CreateNetns 创建网络命名空间
// 返回netns文件的绝对路径
func CreateNetns(id string) (string, error) {
	if err := os.MkdirAll(NetnsRootDir, 0755); err != nil {
		logger.Error("mkdir %s failed, %v", NetnsRootDir, err)
		return "", fmt.Errorf("mkdir %s failed, %w", id, err)
	}
	nsPath := filepath.Join(NetnsRootDir, id)

	f, err := os.Create(nsPath)
	if err != nil {
		logger.Error("create %s failed, %v", nsPath, err)
		return "", fmt.Errorf("create %s failed, %w", nsPath, err)
	}
	defer f.Close()

	// 如果一个goroutine退出时，仍然锁着某个线程，Goroutine会销毁这个线程，不放回线程池
	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
			logger.Error("unshare failed, %v", err)
			errCh <- fmt.Errorf("unshare failed, %w", err)
			return
		}
		// 把当前线程的net ns bind mount
		// thread-self是内核提供的魔术路径
		if err := syscall.Mount("/proc/thread-self/ns/net", nsPath, "", syscall.MS_BIND, ""); err != nil {
			logger.Error("mount failed, %v", err)
			errCh <- fmt.Errorf("mount failed, %w", err)
			return
		}

		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		os.Remove(nsPath)
		return "", err
	}

	return nsPath, nil
}

// RemoveNetns 删除 netns（释放引用计数 + 删文件）
func RemoveNetns(id string) error {
	nsPath := filepath.Join(NetnsRootDir, id)
	syscall.Unmount(nsPath, syscall.MNT_DETACH) // 即使没挂也不报错（已 detach）
	if err := os.Remove(nsPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
