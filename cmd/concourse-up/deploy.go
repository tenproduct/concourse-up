package main

import (
	"github.com/EngineerBetter/concourse-up/deployment"
	"github.com/pkg/errors"

	"github.com/urfave/cli"
)

var deployCmd = cli.Command{
	Name:      "deploy",
	Aliases:   []string{"d"},
	Usage:     "Deploys or updates a Concourse",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:   "region",
			Value:  "eu-west-1",
			Usage:  "(optional) AWS region",
			EnvVar: "AWS_REGION",
		},
		cli.StringFlag{
			Name:   "domain",
			Usage:  "(optional) Domain to use as endpoint for Concourse web interface (eg: ci.myproject.com)",
			EnvVar: "DOMAIN",
		},
		cli.StringFlag{
			Name:   "tls-cert",
			Usage:  "(optional) TLS cert to use with Concourse endpoint",
			EnvVar: "TLS_CERT",
		},
		cli.StringFlag{
			Name:   "tls-key",
			Usage:  "(optional) TLS private key to use with Concourse endpoint",
			EnvVar: "TLS_KEY",
		},
		cli.IntFlag{
			Name:   "workers",
			Usage:  "(optional) Number of Concourse worker instances to deploy",
			EnvVar: "WORKERS",
			Value:  1,
		},
		cli.StringFlag{
			Name:   "worker-size",
			Usage:  "(optional) Size of Concourse workers. Can be medium, large, xlarge, 2xlarge, 4xlarge, 10xlarge or 16xlarge",
			EnvVar: "WORKER_SIZE",
			Value:  "xlarge",
		},
		cli.StringFlag{
			Name:   "web-size",
			Usage:  "(optional) Size of Concourse web node. Can be small, medium, large, xlarge, 2xlarge",
			EnvVar: "WEB_SIZE",
			Value:  "small",
		},
		cli.StringFlag{
			Name:   "iaas",
			Usage:  "(optional) IAAS, can be AWS or GCP",
			EnvVar: "IAAS",
			Value:  "AWS",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:   "self-update",
			Usage:  "(optional) Causes Concourse-up to exit as soon as the BOSH deployment starts. May only be used when upgrading an existing deployment",
			EnvVar: "SELF_UPDATE",
			Hidden: true,
		},
		cli.StringFlag{
			Name:   "db-size",
			Usage:  "(optional) Size of Concourse RDS instance. Can be small, medium, large, xlarge, 2xlarge, or 4xlarge",
			EnvVar: "DB_SIZE",
			Value:  "small",
		},
		cli.StringSliceFlag{
			Name:   "allow-ips",
			Usage:  "(optional) Comma seperated list of IP addresses or CIDR ranges to allow access too",
			EnvVar: "ALLOW_IPS",
			Value:  &cli.StringSlice{"0.0.0.0/0"},
		},
	},
	Action: deployAction,
}

func deployAction(c *cli.Context) error {
	deploymentName := c.Args().First()
	if deploymentName == "" {
		return errors.New("usage is `concourse-up deploy <name>`")
	}

	var opts []deployment.Option
	switch iaas := c.String("iaas"); iaas {
	case "aws", "AWS":
		opts = append(opts, deployment.AWS(c.String("region")))
	default:
		return errors.Errorf("unsupported iaas %s", iaas)
	}

	if domain := c.String("domain"); domain != "" {
		opts = append(opts, deployment.CustomDomain(domain))
	}

	if tlsCert := c.String("tls-cert"); tlsCert != "" {
		opts = append(opts, deployment.UserProvidedCert(tlsCert, c.String("tls-key")))
	}

	opts = append(opts, deployment.IPWhitelist(c.StringSlice("allow-ips")))

	opts = append(opts, deployment.InstanceClass(
		deployment.InstanceTypeWorker,
		sizer(c.String("worker-size")),
	))
	opts = append(opts, deployment.InstanceClass(
		deployment.InstanceTypeWeb,
		sizer(c.String("web-size")),
	))
	opts = append(opts, deployment.InstanceClass(
		deployment.InstanceTypeDB,
		sizer(c.String("db-size")),
	))

	opts = append(opts, deployment.InstanceCount(
		deployment.InstanceTypeWorker,
		c.Int("workers"),
	))

	d, err := deployment.New(deploymentName, opts...)
	_ = d
	return err
}

func sizer(size string) deployment.InstanceSize {
	switch size {
	case "small":
		return deployment.InstanceSizeSmall
	case "medium":
		return deployment.InstanceSizeMedium
	case "xlarge":
		return deployment.InstanceSizeXLarge
	case "2xlarge":
		return deployment.InstanceSize2XLarge
	case "4xlarge":
		return deployment.InstanceSize4XLarge
	case "10xlarge":
		return deployment.InstanceSize10XLarge
	case "16xlarge":
		return deployment.InstanceSize16XLarge
	default:
		return deployment.InstanceSizeInvalid
	}
}
