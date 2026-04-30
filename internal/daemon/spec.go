package daemon

import (
	"encoding/json"
	"github.com/bytedance/gopkg/util/logger"
	"litcontainer/internal/cgroups"
	"litcontainer/internal/container"
	"litcontainer/internal/filesys"
	"litcontainer/internal/runtime"
	"os"
	"path/filepath"
)

// configToSpec 将容器配置转换为运行时规范
func configToSpec(cfg *container.Config) *runtime.Spec {
	return &runtime.Spec{
		Version:  runtime.OciVersion,
		Hostname: cfg.ID[:12],
		Root: &runtime.Root{
			Path: filesys.GetMountPoint(cfg.ID),
		},
		Process: &runtime.Process{
			Args:     cfg.Command,
			Env:      defaultEnv(cfg),
			Cwd:      "/",
			Terminal: cfg.TTY,
		},
		Mounts: defaultMounts(cfg),
		Linux:  buildLinux(cfg),
	}
}

func defaultEnv(cfg *container.Config) []string {
	base := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=xterm",
		"HOSTNAME=" + cfg.ID[:12],
	}
	return append(base, cfg.Envs...)
}

func defaultMounts(cfg *container.Config) []*runtime.Mount {
	mounts := make([]*runtime.Mount, 0)

	// 添加默认挂载/proc和/dev
	mounts = append(mounts, &runtime.Mount{
		Destination: "/proc",
		Type:        "proc",
		Source:      "proc",
		Options:     []string{"nosuid", "noexec", "nodev"},
	})
	mounts = append(mounts, &runtime.Mount{
		Destination: "/dev",
		Type:        "devtmpfs",
		Source:      "none",
		Options:     []string{"relatime"},
	})

	// 添加volume挂载
	for _, mount := range cfg.Mounts {
		mounts = append(mounts, &runtime.Mount{
			Destination: mount.Destination,
			Type:        "bind",
			Source:      mount.Source,
			Options:     []string{"rbind"},
		})
	}
	return mounts
}

func buildLinux(cfg *container.Config) *runtime.Linux {
	return &runtime.Linux{
		Namespaces: []*runtime.Namespace{
			{Type: "pid"}, {Type: "mount"}, {Type: "uts"},
			{Type: "ipc"}, {Type: "network"},
		},
		CgroupsPath: filepath.Join(cgroups.CgroupRoot, "litcontainer-"+cfg.ID+".scope"),
		Resources: &runtime.Resources{
			CPU:    cpuResourcesFromConfig(cfg),
			Memory: memoryResourcesFromConfig(cfg),
		},
	}
}

func cpuResourcesFromConfig(cfg *container.Config) *runtime.CPUResources {
	if cfg.CPULimit == "" {
		return nil
	}

	quota, period, err := cgroups.ParseCPUs(cfg.CPULimit)
	if err != nil {
		logger.Errorf("failed parse cpuLimit, %v", err)
		return nil
	}
	return &runtime.CPUResources{
		Quota:  int64(quota),
		Period: uint64(period),
	}
}

func memoryResourcesFromConfig(cfg *container.Config) *runtime.MemoryResources {
	if cfg.MemoryLimit == "" {
		return nil
	}

	memory, err := cgroups.ParseMemory(cfg.MemoryLimit)
	if err != nil {
		logger.Errorf("failed parse memoryLimit, %v", err)
		return nil
	}
	return &runtime.MemoryResources{
		Limit: int64(memory),
	}
}

func writeSpec(containerId string, spec *runtime.Spec) error {
	jsonStr, err := json.Marshal(spec)
	if err != nil {
		return err
	}

	dirPath := filepath.Join(container.DefaultLitContainerDir, containerId)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dirPath, runtime.DefaultSpecFileName)
	if err := os.WriteFile(filePath, jsonStr, 0644); err != nil {
		return err
	}
	return nil
}
