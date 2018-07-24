package deployment

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

func (d *Deployment) deploy() error {
	bucketName := fmt.Sprintf("concourse-up-%s-%s-config", d.name, d.region)
	_, err := d.s3Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return errors.Wrap(err, "creating config bucket")
	}
	return nil
}
