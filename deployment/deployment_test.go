package deployment_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/stretchr/testify/require"

	"github.com/EngineerBetter/concourse-up/deployment"
)

type mockCreateBucket struct {
	s3iface.S3API
	in   *s3.CreateBucketInput
	resp *s3.CreateBucketOutput
	err  error
}

func (m *mockCreateBucket) CreateBucket(in *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
	m.in = in
	return m.resp, m.err
}

type mockTerraform struct {
	creds   *credentials.Credentials
	hcl     []byte
	outputs map[string]string
	err     error
}

func (m *mockTerraform) Apply(c *credentials.Credentials, hcl []byte) (map[string]string, error) {
	m.creds = c
	m.hcl = hcl
	return m.outputs, m.err
}

func TestNew_success(t *testing.T) {
	mockS3 := &mockCreateBucket{}
	mockTerraform := &mockTerraform{}
	creds := credentials.NewStaticCredentials("foo", "bar", "baz")
	_, err := deployment.New("myDeployment",
		deployment.MockAWS(creds, mockS3, "myAWSRegion"),
		deployment.MockTerraform(mockTerraform),
	)
	require.NoError(t, err)
	require.Equal(t, &s3.CreateBucketInput{
		Bucket: aws.String("concourse-up-myDeployment-myAWSRegion-config"),
	}, mockS3.in)
	require.Equal(t, mockTerraform.creds, creds)
}

func TestNew_createBucketFailure(t *testing.T) {
	mockS3 := &mockCreateBucket{
		err: errors.New("an error"),
	}
	_, err := deployment.New("myDeployment", deployment.MockAWS(mockS3, "myAWSRegion"))
	require.EqualError(t, err, "creating config bucket: an error")
}
