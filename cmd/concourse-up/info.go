package main

import "github.com/urfave/cli"

var infoCmd = cli.Command{
	Name:      "info",
	Aliases:   []string{"i"},
	Usage:     "Fetches information on a deployed environment",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:   "region",
			Value:  "eu-west-1",
			Usage:  "(optional) AWS region",
			EnvVar: "AWS_REGION",
		},
		cli.BoolFlag{
			Name:   "json",
			Usage:  "(optional) Output as json",
			EnvVar: "JSON",
		},
		cli.BoolFlag{
			Name:  "env",
			Usage: "(optional) Output environment variables",
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
