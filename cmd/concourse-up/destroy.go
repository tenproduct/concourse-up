package main

import "github.com/urfave/cli"

var destroyCmd = cli.Command{
	Name:      "destroy",
	Aliases:   []string{"x"},
	Usage:     "Destroys a Concourse",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:   "region",
			Value:  "eu-west-1",
			Usage:  "(optional) AWS region",
			EnvVar: "AWS_REGION",
		},
		cli.StringFlag{
			Name:   "iaas",
			Usage:  "(optional) IAAS, can be AWS or GCP",
			EnvVar: "IAAS",
			Value:  "AWS",
			Hidden: true,
		},
	},
	Action: func(c *cli.Context) error { return nil },
}
