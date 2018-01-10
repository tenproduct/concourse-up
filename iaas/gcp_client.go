package iaas

import (
	"context"
	"io/ioutil"

	"google.golang.org/api/option"

	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"

	"cloud.google.com/go/storage"
)

type gcpClient struct {
	storage        *storage.Client
	computeService *compute.Service
	project        string
}

func NewGCP(project string) (IClient, error) {
	ctx := context.Background()
	c, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	computeService, err := compute.New(c)
	if err != nil {
		return nil, err
	}
	storageClient, err := storage.NewClient(ctx, option.WithHTTPClient(c))
	if err != nil {
		return nil, err
	}
	return &gcpClient{
		project:        project,
		storage:        storageClient,
		computeService: computeService,
	}, nil
}

func (c *gcpClient) DeleteFile(bucket string, path string) error {
	return c.storage.Bucket(bucket).Object(path).Delete(context.Background())
}

func (c *gcpClient) DeleteVersionedBucket(name string) error {
	return c.storage.Bucket(name).Delete(context.Background())
}

func (c *gcpClient) DeleteVMsInVPC(vpcID string) error {
	l, err := c.computeService.Instances.List(c.project, vpcID).Do()
	if err != nil {
		return err
	}
	ops := make(map[*compute.Operation]bool)
	for _, i := range l.Items {
		op, err := c.computeService.Instances.Delete(c.project, vpcID, i.Name).Do()
		if err != nil {
			return err
		}
		ops[op] = true
	}
	for op := range ops {
		if op.Status == "DONE" {
			delete(ops, op)
		}

	}

	return nil
}

func (c *gcpClient) EnsureBucketExists(name string) error {
	return c.storage.Bucket(name).Create(context.Background(), c.project, nil)
}

func (c *gcpClient) EnsureFileExists(bucket string, path string, defaultContents []byte) (data []byte, ok bool, err error) {
	w := c.storage.Bucket(bucket).Object(path).If(storage.Conditions{DoesNotExist: true}).NewWriter(context.Background())
	defer func() {
		if err1 := w.Close(); err == nil {
			err = err1
		}
	}()
	_, err = w.Write(defaultContents)
	if err != nil {
		return
	}
	r, err := c.storage.Bucket(bucket).Object(path).NewReader(context.Background())
	if err != nil {
		return nil, false, err
	}
	defer r.Close()
	data, err = ioutil.ReadAll(r)
	return nil, true, err
}

func (c *gcpClient) FindLongestMatchingHostedZone(subdomain string) (string, string, error) {
	panic("not implemented")
}

func (c *gcpClient) HasFile(bucket string, path string) (bool, error) {
	_, err := c.storage.Bucket(bucket).Object(path).Attrs(context.Background())
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *gcpClient) LoadFile(bucket string, path string) ([]byte, error) {
	r, err := c.storage.Bucket(bucket).Object(path).NewReader(context.Background())
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ioutil.ReadAll(r)
}

func (c *gcpClient) WriteFile(bucket string, path string, contents []byte) (err error) {
	w := c.storage.Bucket(bucket).Object(path).NewWriter(context.Background())
	defer func() {
		if err1 := w.Close(); err == nil {
			err = err1
		}
	}()
	_, err = w.Write(contents)
	return
}

func (c *gcpClient) Region() string {
	panic("not implemented")
}

func (c *gcpClient) IAAS() string {
	return "gcp"
}
