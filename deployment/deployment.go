package deployment

import (
	"net"

	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/pkg/errors"
)

type Deployment struct {
	name            string
	region          string
	creds           *credentials.Credentials
	s3Client        s3iface.S3API
	terraformClient interface {
		Apply(*credentials.Credentials, []byte) (map[string]string, error)
	}
}

type Option func(*Deployment) error

func New(name string, opts ...Option) (*Deployment, error) {
	d := new(Deployment)
	d.name = name
	for _, o := range opts {
		err := o(d)
		if err != nil {
			err = errors.Wrap(err, "applying option")
			return nil, err
		}
	}
	return d, d.deploy()
}

func AWS(region string) Option {
	return func(d *Deployment) error {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})
		if err != nil {
			err = errors.Wrap(err, "creating aws session")
			return err
		}
		d.creds = sess.Config.Credentials
		d.s3Client = s3.New(sess)
		d.region = region
		return nil
	}
}

func CustomDomain(name string) Option {
	return func(d *Deployment) error {
		return errors.New("CustomDomain: unimplemented")
	}
}

func UserProvidedCert(cert, key string) Option {
	return func(d *Deployment) error {
		return errors.New("UserProvidedCert: unimplemented")
	}
}

func IPWhitelist(ips []string) Option {
	return func(d *Deployment) error {
		for _, ip := range ips {
			_, _, err := net.ParseCIDR(ip)
			if err == nil {
				// do a thing
			}
			ip_ := net.ParseIP(ip)
			if ip_ == nil {
				return errors.Errorf("IPWhitelist: could not parse %q as CIDR range or IP address", ip)
			}
			// do a thing
		}
		return errors.New("IPWhitelist: unimplemented")
	}
}

type InstanceType int

const (
	InstanceTypeWeb InstanceType = iota
	InstanceTypeDB
	InstanceTypeWorker
)

type InstanceSize int

const (
	InstanceSizeSmall InstanceSize = iota
	InstanceSizeMedium
	InstanceSizeLarge
	InstanceSizeXLarge
	InstanceSize2XLarge
	InstanceSize4XLarge
	InstanceSize10XLarge
	InstanceSize16XLarge

	InstanceSizeInvalid
)

func InstanceCount(i InstanceType, n int) Option {
	return func(d *Deployment) error {
		return errors.New("WorkerCount: unimplemented")
	}
}

func InstanceClass(i InstanceType, size InstanceSize) Option {
	return func(d *Deployment) error {
		return errors.New("WorkerCount: unimplemented")
	}
}
