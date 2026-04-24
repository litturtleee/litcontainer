package cli

import (
	"fmt"
	"litcontainer/internal/filesys"
	"strings"
)

func parseMountVolume(mountVolumes []string) ([]filesys.MountConfig, error) {
	if len(mountVolumes) == 0 {
		return nil, nil
	}

	mounts := make([]filesys.MountConfig, 0)
	for _, volume := range mountVolumes {
		parts := strings.SplitN(volume, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid volume format: %s", volume)
		}
		mounts = append(
			mounts, filesys.MountConfig{
				Source:      parts[0],
				Destination: parts[1],
			},
		)
	}
	return mounts, nil
}
