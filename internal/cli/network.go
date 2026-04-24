package cli

import (
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
	"os"
	"text/tabwriter"
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
		return network.GetController().Create(name, driverType, subnet)
	},
}

var NetworkListCommand = cli.Command{
	Name:  "ls",
	Usage: "list all networks",
	Action: func(ctx *cli.Context) error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintf(w, "NAME\tIPRANGE\tDRIVER\n")
		nws, err := network.GetController().List()
		if err != nil {
			return err
		}
		for _, nw := range nws {
			fmt.Fprintf(w, "%s\t%s\t%s\n", nw.Name, nw.IpRange, nw.Driver)
		}
		if err := w.Flush(); err != nil {
			logger.Error("flush w failed: %v", err)
			return err
		}
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
		return network.GetController().Delete(name)
	},
}
