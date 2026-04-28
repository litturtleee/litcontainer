package cli

import (
	"errors"
	"github.com/urfave/cli"
	"litcontainer/internal/api/types"
	"litcontainer/internal/client"
	"litcontainer/internal/logger"
)

var ExportCommand = cli.Command{
	Name:  "export",
	Usage: "Package the current running container into a tar file (docker export -o <tarfile> <imageName>)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "o",
			Usage: "Output file name for the tar file",
		},
	},
	Action: func(c *cli.Context) error {
		if len(c.Args()) == 0 {
			logger.Error("Usage: tinydocker export [-o <tarfile>] <containerName>")
			return errors.New("Usage: tinydocker export [-o <tarfile>]  <containerName>")
		}
		containerName := c.Args().Get(0)
		output := c.String("o")
		if output == "" {
			output = "container"
		}

		cli := client.NewClient()
		if err := cli.ExportImage(types.ImageExportRequest{
			ContainerName: containerName,
			OutputName:    output,
		}); err != nil {
			return err
		}
		return nil
	},
}
