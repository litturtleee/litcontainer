package daemon

import "litcontainer/internal/image"

func (d *Daemon) ExportImage(containerName, output string) error {
	return image.Export(containerName, output)
}
