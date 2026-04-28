package cli

import "github.com/urfave/cli"

var RuntimeRunCommand = cli.Command{
	Name:  "run",
	Usage: "create a new container",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bundle",
			Usage: "path to the bundle directory",
		},
	},

	Action: func(context *cli.Context) error {

		return nil
	},
}
