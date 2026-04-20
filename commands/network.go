package commands

import (
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/network"
)

// docker network create --subnet <cidr> --driver <dirver> <name>
// docker network ls
// docker network rm <name>

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
			return fmt.Errorf("missing network name, %w", ErrInvalidArguments)
		}
		subnet := ctx.String("subnet")
		if subnet == "" {
			return fmt.Errorf("missing subnet, %w", ErrInvalidArguments)
		}
		driverType := ctx.String("driver")
		if driverType == "" {
			return fmt.Errorf("missing driver, %w", ErrInvalidArguments)
		}
		return network.CreateNetwork(name, driverType, subnet)
	},
}

var NetworkListCommand = cli.Command{
	Name:  "ls",
	Usage: "list all networks",
	Action: func(ctx *cli.Context) error {
		network.ListNetworks()
		return nil
	},
}

var NetworkRemoveCommand = cli.Command{
	Name:  "rm",
	Usage: "remove a network",
	Action: func(ctx *cli.Context) error {
		name := ctx.Args().Get(0)
		if name == "" {
			return fmt.Errorf("missing network name, %w", ErrInvalidArguments)
		}
		return network.DeleteNetwork(name)
	},
}
