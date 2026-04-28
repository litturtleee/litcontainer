package cli

import (
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/internal/api/types"
	"litcontainer/internal/client"
)

var NetworkCommands = cli.Command{
	Name:  "network",
	Usage: "manage networks",
	Subcommands: []cli.Command{
		NetworkCreateCommand,
		NetworkListCommand,
		NetworkRemoveCommand,
	},
}

var NetworkCreateCommand = cli.Command{
	Name:  "create",
	Usage: "create a network",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "subnet",
			Usage: "subnet in CIDR format (e.g., 192.168.243.0/24)",
		},
		cli.StringFlag{
			Name:  "driver",
			Usage: "driver of the network",
		},
	},
	Action: func(ctx *cli.Context) error {
		name := ctx.Args().Get(0)
		if name == "" {
			return fmt.Errorf("missing network name, %w", client.ErrInvalidArguments)
		}
		subnet := ctx.String("subnet")
		if subnet == "" {
			return fmt.Errorf("missing subnet, %w", client.ErrInvalidArguments)
		}
		driverType := ctx.String("driver")
		if driverType == "" {
			return fmt.Errorf("missing driver, %w", client.ErrInvalidArguments)
		}

		cli := client.NewClient()
		return cli.NetworkCreate(types.NetworkCreateRequest{
			Name:   name,
			Subnet: subnet,
			Driver: driverType,
		})
	},
}

var NetworkListCommand = cli.Command{
	Name:  "ls",
	Usage: "list all networks",
	Action: func(ctx *cli.Context) error {
		cli := client.NewClient()
		list, err := cli.NetworkList()
		if err != nil {
			return err
		}
		return printNetworksInfo(list)
	},
}

var NetworkRemoveCommand = cli.Command{
	Name:  "rm",
	Usage: "remove a network",
	Action: func(ctx *cli.Context) error {
		name := ctx.Args().Get(0)
		if name == "" {
			return fmt.Errorf("missing network name, %w", client.ErrInvalidArguments)
		}

		cli := client.NewClient()
		return cli.NetworkRemove(name)

	},
}
