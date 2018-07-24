package deployment

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

func MockAWS(creds *credentials.Credentials, m s3iface.S3API, region string) Option {
	return func(d *Deployment) error {
		d.creds = creds
		d.region = region
		d.s3Client = m
		return nil
	}
}

type terraformClient interface {
	Apply(*credentials.Credentials, []byte) (map[string]string, error)
}

func MockTerraform(m terraformClient) Option {
	return func(d *Deployment) error {
		d.terraformClient = m
		return nil
	}
}
