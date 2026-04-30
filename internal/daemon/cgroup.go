package daemon

import (
	"fmt"
	"litcontainer/internal/cgroups"
	"litcontainer/internal/version"
	"path/filepath"
)

func SetupCGroup(pid int, containerID, cpuLimit, memoryLimit string) (*cgroups.CGroupManager, error) {
	cgroupPath := filepath.Join(cgroups.CgroupRoot, fmt.Sprintf("%s-%s.scope", version.AppName, containerID))
	cg, _ := cgroups.NewCGroupManager(cgroupPath)
	if memoryLimit != "" {
		if err := cg.SetMemoryLimit(memoryLimit); err != nil {
			return nil, err
		}
	}
	if cpuLimit != "" {
		if err := cg.SetCPULimit(cpuLimit); err != nil {
			return nil, err
		}
	}
	if err := cg.Apply(pid); err != nil {
		return nil, err
	}
	return cg, nil
}
