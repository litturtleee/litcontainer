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
	"path/filepath"
	runtime2 "runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
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
		runtime2.LockOSThread()
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

		success := false
		var cp *containerProcess
		defer func() {
			if !success && cp != nil {
				cp.cleanup()
			}
		}()

		cp, err = setupContainer(spec, bundle, id, "")
		if err != nil {
			return fmt.Errorf("setup container failed: %w", err)
		}

		if err := triggerStart(id); err != nil {
			return fmt.Errorf("trigger start failed: %w", err)
		}

		if err := cp.initCmd.Wait(); err != nil {
			return fmt.Errorf("wait init process failed: %w", err)
		}

		deleteContainer(id, false)
		success = true

		// exit
		exitCode := 0
		if cp.initCmd.ProcessState != nil {
			exitCode = cp.initCmd.ProcessState.ExitCode()
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
		var initBootStrap runtime.InitBootstrap
		if err := json.NewDecoder(pipe).Decode(&initBootStrap); err != nil {
			logger.Error("Failed to decode spec.json: %v", err)
			return fmt.Errorf("failed to decode serverconfig.json, %w", err)
		}
		spec := initBootStrap.Spec
		// 打开fifo, O_WRONLY阻塞等待reader， O_RDONLY阻塞等待writer, O_RDWR不阻塞
		file, err := os.OpenFile(initBootStrap.FifoPath, os.O_RDWR, 0)
		if err != nil {
			logger.Error("Failed to open fifo: %v", err)
			return fmt.Errorf("failed to open fifo, %w", err)
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
		// 等待start
		buf := make([]byte, 1)
		if _, err := file.Read(buf); err != nil {
			logger.Error("Failed to read pipe: %v", err)
			return fmt.Errorf("failed to read pipe, %w", err)
		}
		file.Close()

		if err := syscall.Exec(path, spec.Process.Args, spec.Process.Env); err != nil {
			logger.Error("Failed to exec: %v", err)
			return fmt.Errorf("failed to exec, %w", err)
		}
		return nil
	},
}

var RuntimeCreateCommand = cli.Command{
	Name:      "create",
	Usage:     "create a container",
	ArgsUsage: "<container-id>",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "bundle", Usage: "path to the bundle directory"},
		&cli.StringFlag{Name: "pid-file", Usage: "path to file to write init pid"},
	},
	Action: func(context *cli.Context) error {
		runtime2.LockOSThread()
		bundle := context.String("bundle")
		pidFile := context.String("pid-file")
		id := context.Args().First()

		spec, err := runtime.LoadSpec(bundle)
		if err != nil {
			return fmt.Errorf("load spec failed: %w", err)
		}
		containerProcess, err := setupContainer(spec, bundle, id, pidFile)
		if err != nil {
			containerProcess.cleanup()
			return fmt.Errorf("setup container failed: %w", err)
		}
		// 解除 Go 运行时对 init 进程的控制权
		containerProcess.initCmd.Process.Release()
		return nil
	},
}

var RuntimeStartCommand = cli.Command{
	Name:      "start",
	Usage:     "start a created container",
	ArgsUsage: "<container-id>",
	Action: func(context *cli.Context) error {
		id := context.Args().First()
		return triggerStart(id)
	},
}

var RuntimeDeleteCommand = cli.Command{
	Name:      "delete",
	Usage:     "delete a container",
	ArgsUsage: "<container-id>",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "force", Usage: "force delete"},
	},
	Action: func(context *cli.Context) error {
		force := context.Bool("force")
		id := context.Args().First()
		return deleteContainer(id, force)
	},
}

type containerProcess struct {
	initCmd *exec.Cmd
	cgroup  *cgroups.CGroupManager
	state   *runtime.ContainerState

	fifoPath string
	cleanup  func()
}

type cleanupFn func()

func setupContainer(spec *runtime.Spec, bundle, id, pidFile string) (*containerProcess, error) {
	var containerProcess containerProcess
	// 错误处理
	var cleanups []cleanupFn
	addCleanup := func(cleanup cleanupFn) {
		cleanups = append(cleanups, cleanup)
	}
	runCleanups := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}
	containerProcess.cleanup = runCleanups

	// 1.mkfifo
	fifoDir := filepath.Join(runtime.DefaultRuntimeStateRootPath, id)
	if err := os.MkdirAll(fifoDir, 0755); err != nil {
		return &containerProcess, fmt.Errorf("mkdir %s failed: %w", fifoDir, err)
	}
	fifoPath := filepath.Join(fifoDir, "exec.fifo")
	os.Remove(fifoPath)
	if err := syscall.Mkfifo(fifoPath, 0666); err != nil {
		return &containerProcess, fmt.Errorf("mkfifo %s failed: %w", fifoPath, err)
	}
	addCleanup(func() {
		os.Remove(fifoPath)
	})
	containerProcess.fifoPath = fifoPath

	// 2.创建pipe
	r, w, err := os.Pipe()
	if err != nil {
		return &containerProcess, fmt.Errorf("create pipe failed: %w", err)
	}

	// 3.fork init
	self, _ := os.Executable()
	initCmd := exec.Command(self, "init")
	var cloneFlags uintptr
	if len(spec.Linux.Namespaces) > 0 {
		cloneFlags = runtime.CloneFlags(spec.Linux.Namespaces)
	}
	initCmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneFlags,
	}
	initCmd.ExtraFiles = []*os.File{r}
	initCmd.Stdin, initCmd.Stdout, initCmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := initCmd.Start(); err != nil {
		return &containerProcess, fmt.Errorf("start init cmd failed: %w", err)
	}
	r.Close()
	addCleanup(func() {
		initCmd.Process.Kill()
		initCmd.Process.Wait()
	})
	containerProcess.initCmd = initCmd

	// 4.writeRuntimeState
	state := &runtime.ContainerState{
		ID:      id,
		Version: spec.Version,
		Bundle:  bundle,
		PID:     initCmd.Process.Pid,
		Status:  "created",
	}
	if err := runtime.WriteState(state); err != nil {
		return &containerProcess, fmt.Errorf("write state failed: %w", err)
	}
	addCleanup(func() {
		runtime.RemoveState(id)
	})
	containerProcess.state = state

	// 5.cgroup
	// 配置cgroup
	cgroup, err := cgroups.NewCGroupManager(spec.Linux.CgroupsPath)
	if err != nil {
		return &containerProcess, fmt.Errorf("new cgroup manager failed: %w", err)
	}
	containerProcess.cgroup = cgroup
	addCleanup(func() { cgroup.Cleanup() })
	if err := cgroup.Apply(initCmd.Process.Pid); err != nil {
		return &containerProcess, fmt.Errorf("apply cgroup failed: %w", err)
	}
	if spec.Linux.Resources != nil {
		if spec.Linux.Resources.CPU != nil {
			if err := cgroup.SetCPULimitRaw(spec.Linux.Resources.CPU.Quota,
				spec.Linux.Resources.CPU.Period); err != nil {
				return &containerProcess, fmt.Errorf("set cpu limit failed: %w", err)
			}
		}
		if spec.Linux.Resources.Memory != nil {
			if err := cgroup.SetMemoryLimitRaw(spec.Linux.Resources.Memory.Limit); err != nil {
				return &containerProcess, fmt.Errorf("set memory limit failed: %w", err)
			}
		}
	}

	// prestart hook
	if spec.Hooks != nil {
		if err := runtime.RunHooks(spec.Hooks.Prestart, state); err != nil {
			return &containerProcess, fmt.Errorf("run prestart hooks failed: %w", err)
		}
	}

	// 把initBootstrap写到pipe
	initBootstrap := runtime.InitBootstrap{
		Spec:     spec,
		FifoPath: fifoPath,
	}
	initBootJSON, _ := json.Marshal(initBootstrap)
	if _, err := w.Write(initBootJSON); err != nil {
		return &containerProcess, fmt.Errorf("write init bootstrap failed: %w", err)
	}
	w.Close()

	// 写pid-file
	if pidFile != "" {
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(initCmd.Process.Pid)), 0644); err != nil {
			return &containerProcess, fmt.Errorf("write pid file failed: %w", err)
		}
	}

	return &containerProcess, nil
}

func triggerStart(id string) error {
	// loadState
	state, err := runtime.LoadState(id)
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}
	// check state
	if state.Status != "created" {
		return fmt.Errorf("container %s is not in created state", id)
	}
	// open fifo 释放init阻塞
	fifoPath := filepath.Join(runtime.DefaultRuntimeStateRootPath, id, "exec.fifo")
	fifo, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open fifo failed: %w", err)
	}
	defer fifo.Close()

	// start init.exe
	if _, err := fifo.Write([]byte{0}); err != nil {
		return fmt.Errorf("write to fifo failed: %w", err)
	}

	// writeState
	state.Status = "running"
	if err := runtime.WriteState(state); err != nil {
		return fmt.Errorf("write state failed: %w", err)
	}
	return nil
}

func deleteContainer(id string, force bool) error {
	state, err := runtime.LoadState(id)
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	if syscall.Kill(state.PID, 0) == nil {
		if !force {
			return fmt.Errorf("container %s is still running", id)
		}
		syscall.Kill(state.PID, syscall.SIGKILL)
		//  wait
		for i := 0; i < 10; i++ {
			if syscall.Kill(state.PID, 0) != nil {
				break
			}
			time.Sleep(time.Second)
		}
	}

	// poststop hook
	state.Status = "stopped"
	state.PID = 0
	spec, _ := runtime.LoadSpec(state.Bundle)
	if spec.Hooks != nil {
		runtime.RunHooks(spec.Hooks.Poststop, state)
	}

	// 成功了释放资源
	// cgroup cleanup
	cgroup, _ := cgroups.NewCGroupManager(spec.Linux.CgroupsPath)
	cgroup.Cleanup()

	// 删 state 目录（含 fifo）
	runtime.RemoveState(id)

	return nil
}
