//go:build linux && cgo

package main

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

// nsenter_init 在 Go runtime 启动前执行（单线程态），加入目标容器进程的所有 namespace
// 仅当环境变量 LITCONTAINER_EXEC_PID 被设置时才激活
// 因为是构造函数，所以在 Go runtime 启动前执行，且此时是单线程态，不会有并发问题。
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
		{"mnt",  0x00020000, -1},  // CLONE_NEWNS
		{"uts",  0x04000000, -1},  // CLONE_NEWUTS
		{"ipc",  0x08000000, -1},  // CLONE_NEWIPC
		{"net",  0x40000000, -1},  // CLONE_NEWNET
		{"pid",  0x20000000, -1},  // CLONE_NEWPID
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
		// 父进程：等子进程，透传退出码
		int status;
		waitpid(child, &status, 0);
		exit(WIFEXITED(status) ? WEXITSTATUS(status) : 1);
	}
	// 子进程：已在容器所有 namespace 内，继续走 Go runtime
}
*/
import "C"
