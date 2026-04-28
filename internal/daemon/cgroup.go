package daemon

import (
	"fmt"
	"litcontainer/internal/cgroups"
	"litcontainer/internal/version"
)

func SetupCGroup(pid int, containerID, cpuLimit, memoryLimit string) (*cgroups.CGroupManager, error) {
	cg := cgroups.NewCGroupManager(fmt.Sprintf("%s-%s.scope", version.AppName, containerID))
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
