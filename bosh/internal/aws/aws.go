package aws

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/EngineerBetter/concourse-up/iaas"
	"github.com/EngineerBetter/concourse-up/resource"
	"github.com/EngineerBetter/concourse-up/util"
	"github.com/EngineerBetter/concourse-up/util/yaml"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// Environment holds all the parameters AWS IAAS needs
type Environment struct {
	AccessKeyID           string
	ATCSecurityGroup      string
	AZ                    string
	BlobstoreBucket       string
	CustomOperations      string
	DBCACert              string
	DBHost                string
	DBName                string
	DBPassword            string
	DBPort                string
	DBUsername            string
	DefaultKeyName        string
	DefaultSecurityGroups []string
	ExternalIP            string
	InternalCIDR          string
	InternalGateway       string
	InternalIP            string
	PrivateCIDR           string
	PrivateCIDRGateway    string
	PrivateCIDRReserved   string
	PrivateKey            string
	PrivateSubnetID       string
	PublicCIDR            string
	PublicCIDRGateway     string
	PublicCIDRReserved    string
	PublicCIDRStatic      string
	PublicSubnetID        string
	Region                string
	S3AWSAccessKeyID      string
	S3AWSSecretAccessKey  string
	SecretAccessKey       string
	Spot                  bool
	VMSecurityGroup       string
	WorkerType            string
}

var allOperations = resource.AWSCPIOps + resource.ExternalIPOps + resource.AWSDirectorCustomOps

// ConfigureDirectorManifestCPI interpolates all the Environment parameters and
// required release versions into ready to use Director manifest
func (e Environment) ConfigureDirectorManifestCPI() (string, error) {
	cpiResource := resource.Get(resource.AWSCPI)
	stemcellResource := resource.Get(resource.AWSStemcell)

	return yaml.Interpolate(resource.DirectorManifest, allOperations+e.CustomOperations, map[string]interface{}{
		"cpi_url":                  cpiResource.URL,
		"cpi_version":              cpiResource.Version,
		"cpi_sha1":                 cpiResource.SHA1,
		"stemcell_url":             stemcellResource.URL,
		"stemcell_sha1":            stemcellResource.SHA1,
		"internal_cidr":            e.InternalCIDR,
		"internal_gw":              e.InternalGateway,
		"internal_ip":              e.InternalIP,
		"access_key_id":            e.AccessKeyID,
		"secret_access_key":        e.SecretAccessKey,
		"region":                   e.Region,
		"az":                       e.AZ,
		"default_key_name":         e.DefaultKeyName,
		"default_security_groups":  e.DefaultSecurityGroups,
		"private_key":              e.PrivateKey,
		"subnet_id":                e.PublicSubnetID,
		"external_ip":              e.ExternalIP,
		"blobstore_bucket":         e.BlobstoreBucket,
		"db_ca_cert":               e.DBCACert,
		"db_host":                  e.DBHost,
		"db_name":                  e.DBName,
		"db_password":              e.DBPassword,
		"db_port":                  e.DBPort,
		"db_username":              e.DBUsername,
		"s3_aws_access_key_id":     e.S3AWSAccessKeyID,
		"s3_aws_secret_access_key": e.S3AWSSecretAccessKey,
	})
}

type awsCloudConfigParams struct {
	ATCSecurityGroupID  string
	AvailabilityZone    string
	PrivateSubnetID     string
	PublicSubnetID      string
	Spot                bool
	VMsSecurityGroupID  string
	WorkerType          string
	PublicCIDR          string
	PublicCIDRStatic    string
	PublicCIDRReserved  string
	PublicCIDRGateway   string
	PrivateCIDR         string
	PrivateCIDRGateway  string
	PrivateCIDRReserved string
}

// IAASCheck returns the IAAS provider
func (e Environment) IAASCheck() iaas.Name {
	return iaas.AWS
}

// ConfigureDirectorCloudConfig inserts values from the environment into the config template passed as argument
func (e Environment) ConfigureDirectorCloudConfig() (string, error) {
	templateParams := awsCloudConfigParams{
		AvailabilityZone:    e.AZ,
		VMsSecurityGroupID:  e.VMSecurityGroup,
		ATCSecurityGroupID:  e.ATCSecurityGroup,
		PublicSubnetID:      e.PublicSubnetID,
		PrivateSubnetID:     e.PrivateSubnetID,
		Spot:                e.Spot,
		WorkerType:          e.WorkerType,
		PublicCIDR:          e.PublicCIDR,
		PublicCIDRGateway:   e.PublicCIDRGateway,
		PublicCIDRReserved:  e.PublicCIDRReserved,
		PublicCIDRStatic:    e.PublicCIDRStatic,
		PrivateCIDR:         e.PrivateCIDR,
		PrivateCIDRGateway:  e.PrivateCIDRGateway,
		PrivateCIDRReserved: e.PrivateCIDRReserved,
	}

	cc, err := util.RenderTemplate("cloud-config", resource.AWSDirectorCloudConfig, templateParams)
	if cc == nil {
		return "", err
	}
	return string(cc), err
}

// ConfigureConcourseStemcell returns the stemcell location string for an AWS specific stemcell for the required concourse version
func (e Environment) ConfigureConcourseStemcell() (string, error) {
	var ops []struct {
		Path  string
		Value json.RawMessage
	}
	err := json.Unmarshal([]byte(resource.AWSReleaseVersions), &ops)
	if err != nil {
		return "", err
	}
	var version string
	for _, op := range ops {
		if op.Path != "/stemcells/alias=xenial/version" {
			continue
		}
		err := json.Unmarshal(op.Value, &version)
		if err != nil {
			return "", err
		}
	}
	if version == "" {
		return "", errors.New("did not find stemcell version in versions.json")
	}
	return fmt.Sprintf("https://s3.amazonaws.com/bosh-aws-light-stemcells/light-bosh-stemcell-%s-aws-xen-hvm-ubuntu-xenial-go_agent.tgz", version), nil
}

// Store holds the abstraction of a aws storage artifact
type Store struct {
	s3     s3iface.S3API
	bucket string
}

// NewStore returns a reference to a new Store
func NewStore(s3 s3iface.S3API, bucket string) *Store {
	return &Store{
		s3:     s3,
		bucket: bucket,
	}
}

// Get returns the contents of a Store element identified with a key
func (s *Store) Get(key string) ([]byte, error) {
	result, err := s.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == s3.ErrCodeNoSuchKey {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	return ioutil.ReadAll(result.Body)
}

// Set stores the contents of a Store element identified with a key
func (s *Store) Set(key string, value []byte) error {
	_, err := s.s3.PutObject(&s3.PutObjectInput{
		Body:   bytes.NewReader(value),
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}
