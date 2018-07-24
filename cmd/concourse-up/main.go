package main

import (
	"log"
	"os"

	"github.com/urfave/cli"
)

const version = "dev"

func main() {
	app := cli.NewApp()
	app.Name = "Concourse-Up"
	app.Usage = "A CLI tool to deploy Concourse CI"
	app.Version = version
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "non-interactive, n",
			Usage:  "non interactive",
			EnvVar: "NON_INTERACTIVE",
		},
	}
	app.Commands = []cli.Command{
		deployCmd,
		destroyCmd,
		infoCmd,
	}

	cli.AppHelpTemplate += "\nSee 'concourse-up help <command>' to read about a specific command.\n\n" +
		"Built by \x1b[36;1mEngineerBetter https://engineerbetter.com\x1b[0m\n"
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
