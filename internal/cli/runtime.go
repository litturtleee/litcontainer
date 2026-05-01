package cli

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"litcontainer/internal/cgroups"
	"litcontainer/internal/client"
	"litcontainer/internal/filesys"
	"litcontainer/internal/logger"
	"litcontainer/internal/runtime"
	"os"
	"os/exec"
	runtime2 "runtime"
	"strings"
	"syscall"
)

var RuntimeRunCommand = cli.Command{
	Name:      "run",
	Usage:     "create a new container",
	ArgsUsage: "<container-id>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bundle",
			Usage: "path to the bundle directory",
		},
	},

	Action: func(context *cli.Context) error {
		// 错误处理
		type cleanupFn func()
		var cleanups []cleanupFn
		addCleanup := func(cleanup cleanupFn) {
			cleanups = append(cleanups, cleanup)
		}
		runCleanups := func() {
			for i := 0; i < len(cleanups); i++ {
				cleanups[i]()
			}
		}
		success := false
		defer func() {
			if !success {
				runCleanups()
			}
		}()

		// 解析加载spec
		bundle := context.String("bundle")
		if bundle == "" {
			return fmt.Errorf("bundle is required, %w", client.ErrInvalidArguments)
		}
		id := context.Args().First()
		if id == "" {
			return fmt.Errorf("container id is required")
		}
		spec, err := runtime.LoadSpec(bundle)
		if err != nil {
			return fmt.Errorf("load spec failed: %w", err)
		}
		if spec.Linux == nil {
			return fmt.Errorf("invalid spec: linux is required")
		}
		var cloneFlags uintptr
		if len(spec.Linux.Namespaces) > 0 {
			cloneFlags = runtime.CloneFlags(spec.Linux.Namespaces)
		}
		// 创建pipe
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		// 创建并启动initCmd
		self, _ := os.Executable()
		initCmd := exec.Command(self, "init")
		initCmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: cloneFlags,
		}
		initCmd.ExtraFiles = []*os.File{r}
		initCmd.Stdin, initCmd.Stdout, initCmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := initCmd.Start(); err != nil {
			return fmt.Errorf("start init cmd failed: %w", err)
		}
		r.Close()
		addCleanup(func() {
			initCmd.Process.Kill()
			initCmd.Process.Wait()
		})
		// 保存state
		state := &runtime.ContainerState{
			ID:      id,
			Version: spec.Version,
			Bundle:  bundle,
			PID:     initCmd.Process.Pid,
			Status:  "created",
		}
		if err := runtime.WriteState(state); err != nil {
			return fmt.Errorf("write state failed: %w", err)
		}
		addCleanup(func() { runtime.RemoveState(id) })

		// 配置cgroup
		cgroup, err := cgroups.NewCGroupManager(spec.Linux.CgroupsPath)
		if err != nil {
			return fmt.Errorf("new cgroup manager failed: %w", err)
		}
		addCleanup(func() { cgroup.Cleanup() })
		if err := cgroup.Apply(initCmd.Process.Pid); err != nil {
			return fmt.Errorf("apply cgroup failed: %w", err)
		}
		if spec.Linux.Resources != nil {
			if spec.Linux.Resources.CPU != nil {
				if err := cgroup.SetCPULimitRaw(spec.Linux.Resources.CPU.Quota,
					spec.Linux.Resources.CPU.Period); err != nil {
					return fmt.Errorf("set cpu limit failed: %w", err)
				}
			}
			if spec.Linux.Resources.Memory != nil {
				if err := cgroup.SetMemoryLimitRaw(spec.Linux.Resources.Memory.Limit); err != nil {
					return fmt.Errorf("set memory limit failed: %w", err)
				}
			}
		}
		// prestart hooks
		if spec.Hooks != nil {
			if err := runtime.RunHooks(spec.Hooks.Prestart, state); err != nil {
				return fmt.Errorf("run prestart hooks failed: %w", err)
			}
		}
		// send config to init process
		specJSON, _ := json.Marshal(spec)
		if _, err := w.Write(specJSON); err != nil {
			return fmt.Errorf("send spec.json to init process failed: %w", err)
		}
		w.Close()
		// 更新state
		state.Status = "running"
		if err := runtime.WriteState(state); err != nil {
			logger.Error("write state failed: %v", err)
		}
		// wait
		if err := initCmd.Wait(); err != nil {
			logger.Error("init process exited: %v", err)
		}
		// poststop hooks
		state.Status = "stopped"
		state.PID = 0
		if spec.Hooks != nil {
			if err := runtime.RunHooks(spec.Hooks.Poststop, state); err != nil {
				logger.Error("run poststop hooks failed: %v", err)
			}
		}
		success = true
		runCleanups()
		// exit
		exitCode := -1
		if initCmd.ProcessState != nil {
			exitCode = initCmd.ProcessState.ExitCode()
		}
		os.Exit(exitCode)
		return nil
	},
}

var RuntimeInitCommand = cli.Command{
	Name:  "init",
	Usage: "initialize a new container",
	Action: func(context *cli.Context) error {
		runtime2.LockOSThread()
		// receive config
		pipe := os.NewFile(3, "pipe")
		defer pipe.Close()
		var spec runtime.Spec
		if err := json.NewDecoder(pipe).Decode(&spec); err != nil {
			logger.Error("Failed to decode spec.json: %v", err)
			return fmt.Errorf("failed to decode serverconfig.json, %w", err)
		}
		// setns(path不为空的)
		nsTypeFlags := map[string]int{
			"pid":     syscall.CLONE_NEWPID,
			"mount":   syscall.CLONE_NEWNS,
			"uts":     syscall.CLONE_NEWUTS,
			"ipc":     syscall.CLONE_NEWIPC,
			"network": syscall.CLONE_NEWNET,
		}
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" {
				fd, err := os.Open(ns.Path)
				if err != nil {
					logger.Error("Failed to open namespace: %v", err)
					return fmt.Errorf("failed to open namespace, %w", err)
				}
				flag, ok := nsTypeFlags[ns.Type]
				if !ok {
					fd.Close()
					logger.Error("Invalid namespace type: %s", ns.Type)
					return fmt.Errorf("invalid namespace type, %w", err)
				}
				if err := unix.Setns(int(fd.Fd()), flag); err != nil {
					fd.Close()
					logger.Error("Failed to set namespace: %v", err)
					return fmt.Errorf("failed to set namespace, %w", err)
				}
				fd.Close()
			}
		}
		// set hostname
		if spec.Hostname != "" {
			if err := syscall.Sethostname([]byte(spec.Hostname)); err != nil {
				logger.Error("Failed to set hostname: %v", err)
				return fmt.Errorf("failed to set hostname, %w", err)
			}
		}
		// 隔断挂载传播
		if err := filesys.SetMountPropagation(); err != nil {
			logger.Error("Failed to set mount propagation: %v", err)
			return fmt.Errorf("failed to set mount propagation, %w", err)
		}
		// 挂载mounts(/proc、/dev等以及volume）
		rootfs := spec.Root.Path
		if err := filesys.MountSpec(rootfs, spec.Mounts); err != nil {
			logger.Error("Failed to mount spec.mounts: %v", err)
			return fmt.Errorf("failed to mount spec.mounts, %w", err)
		}
		// pivot root
		if err := filesys.PivotRoot(rootfs); err != nil {
			logger.Error("Failed to pivot root: %v", err)
			return fmt.Errorf("failed to pivot root, %w", err)
		}
		// exec
		for _, e := range spec.Process.Env {
			if strings.HasPrefix(e, "PATH=") {
				os.Setenv("PATH", strings.TrimPrefix(e, "PATH="))
				break
			}
		}
		path, err := exec.LookPath(spec.Process.Args[0])
		if err != nil {
			logger.Error("Failed to find command: %v", err)
			return fmt.Errorf("failed to find command, %w", err)
		}
		if spec.Process.Cwd != "" {
			os.Chdir(spec.Process.Cwd) // 在容器 rootfs 内 chdir
		}
		if err := syscall.Exec(path, spec.Process.Args, spec.Process.Env); err != nil {
			logger.Error("Failed to exec: %v", err)
			return fmt.Errorf("failed to exec, %w", err)
		}
		return nil
	},
}
