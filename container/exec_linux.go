//go:build linux && cgo

package container

/*
#define _GNU_SOURCE
#include <sched.h>
#include <unistd.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>

// setns_one opens a namespace file and calls setns. Returns 0 on success, -1 on failure.
static int setns_one(const char* nspath, int nstype) {
	int fd = open(nspath, O_RDONLY);
	if (fd < 0) {
		fprintf(stderr, "setns_one: open(%s) failed: %s\n", nspath, strerror(errno));
		return -1;
	}
	if(setns(fd, nstype) < 0) {
		fprintf(stderr, "setns_one: setns(%s) failed: %s\n", nspath, strerror(errno));
		return -1;
	}
	close(fd);
	return 0;
}

// nsenter_init runs before Go runtime starts (single-threaded), joining all
// namespaces of the target container process. It only activates when the
// LITCONTAINER_EXEC_PID environment variable is set.
__attribute__((__constructor__)) static void nsenter_init(void) {
	const char *pid_str = getenv("LITCONTAINER_EXEC_PID");
	if (pid_str == NULL || pid_str[0] == '\0') {
		return;
	}

	int pid = atoi(pid_str);
	if (pid <= 0) {
		fprintf(stderr, "nsenter_init: invalid pid %s\n", pid_str);
		exit(1);
	}

	struct {
		const char *name;
		int flag;
		int fd;
	} namespaces[] = {
		{"mnt",  0x00020000, -1},
		{"uts",  0x04000000, -1},
		{"ipc",  0x08000000, -1},
		{"net",  0x40000000, -1},
		{"pid",  0x20000000, -1},
	};
	int i;
	int count = sizeof(namespaces) / sizeof(namespaces[0]);
	// 第一步：在 host mount namespace 下打开所有 ns fd
	for (i = 0; i < count; i++) {
		char nspath[256];
		snprintf(nspath, sizeof(nspath), "/proc/%d/ns/%s", pid, namespaces[i].name);
		namespaces[i].fd = open(nspath, O_RDONLY);
		if (namespaces[i].fd < 0) {
			fprintf(stderr, "nsenter_init: open(%s) failed: %s\n", nspath, strerror(errno));
			exit(1);
		}
	}
	// 第二步：逐个 setns
	for (i = 0; i < count; i++) {
		if (setns(namespaces[i].fd, namespaces[i].flag) < 0) {
			fprintf(stderr, "nsenter_init: setns(%s) failed: %s\n", namespaces[i].name, strerror(errno));
			exit(1);
		}
		close(namespaces[i].fd);
	}

	// setns(CLONE_NEWPID) 只对子进程生效，必须 fork 一次
	pid_t child = fork();
	if (child < 0) {
		fprintf(stderr, "nsenter_init: fork failed: %s\n", strerror(errno));
		exit(1);
	}
	if (child > 0) {
		// 父进程：等待子进程，传递退出码
		int status;
		waitpid(child, &status, 0);
		exit(WIFEXITED(status) ? WEXITSTATUS(status) : 1);
	}
	// 子进程：真正在新 PID namespace 中，继续执行 Go runtime
}
*/
import "C"

import (
	"fmt"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"syscall"
)

// ExecContainer 运行容器中的进程
// 基于CGO实现，运行Goroutine前执行nsenter_init函数
func ExecContainer(args []string) error {
	if len(args) == 0 {
		logger.Error("ExecContainer: no command specified")
		return fmt.Errorf("no command specified")
	}

	logger.Debug("namespaces already joined by C constructor, executing command, args: %s", args)

	cmdPath, err := lookupPath(args[0])
	if err != nil {
		return fmt.Errorf("exec container failed, %w", err)
	}

	err = syscall.Exec(cmdPath, args, os.Environ())
	if err != nil {
		logger.Error("exec container failed, %v", err)
		return fmt.Errorf("exec container failed, %w", err)
	}
	return nil
}

func lookupPath(file string) (string, error) {
	if filepath.IsAbs(file) {
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
	}

	pathEnv := os.Getenv("PATH")
	for _, path := range filepath.SplitList(pathEnv) {
		cmdPath := filepath.Join(path, file)
		if st, err := os.Stat(cmdPath); err == nil && !st.IsDir() && (st.Mode()&0111) != 0 {
			return cmdPath, nil
		}
	}
	logger.Error("executable %s not found in PATH", file)
	return "", fmt.Errorf("executable %s not found in PATH", file)
}
