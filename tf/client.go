package tf

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/EngineerBetter/concourse-up/tf/internal/ziputil"

	"github.com/EngineerBetter/concourse-up/dl"
)

// Client represents a terraform client
type Client struct {
	exec          func(command string, args ...string) *exec.Cmd
	terraformPath string
	confPath      string
	close         func() error
}

// An Option configures a Client
type Option func(*Client) error

// FromURL configures a Client to use a binary from url, with the hex encoded sha256 checksum sum.
// It caches the binary on disk.
func FromURL(url, sum string) Option {
	return func(c *Client) error {
		p, err := dl.Download(url, sum)
		if err != nil {
			return err
		}
		c.terraformPath = p
		return nil
	}
}

// Conf configures a Client to use a set of terraform files. zipData should contain the zip contents of the terraform files.
func Conf(zipData []byte) Option {
	return func(c *Client) error {
		var err error
		c.confPath, err = ioutil.TempDir("", "concourse-up")
		if err != nil {
			return err
		}
		oldClose := c.close
		c.close = func() error {
			err := oldClose()
			if err1 := os.RemoveAll(c.confPath); err == nil {
				err = err1
			}
			return err
		}
		return ziputil.Unzip(c.confPath, zipData)
	}
}

// New prepares a new Client
func New(opts ...Option) (*Client, error) {
	c := new(Client)
	c.close = func() error { return nil }
	c.exec = exec.Command
	c.terraformPath = "terraform"
	for _, o := range opts {
		err := o(c)
		if err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return c, nil
}

// Close cleans up the Client
func (c *Client) Close() error {
	return c.close()
}

// Apply does a "terraform apply" and returns the outputs.
func (c *Client) Apply() (map[string]Output, error) {
	cmd := c.exec(c.terraformPath, "apply")
	cmd.Dir = c.confPath
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	cmd = c.exec(c.terraformPath, "output", "--json")
	cmd.Dir = c.confPath
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var out map[string]Output
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return nil, err
	}
	return out, cmd.Wait()
}

// Output represents a terraform output
type Output struct {
	ValueType string          `json:"type"`
	Value     json.RawMessage `json:"value"`
}

// OutputType represents a possible output type
type OutputType string

// All possible values of OutputType
const (
	OutputTypeString OutputType = "string"
	OutputTypeList   OutputType = "list"
	OutputTypeMap    OutputType = "map"
)

// Type returns the type of o
func (o Output) Type() OutputType {
	return OutputType(o.ValueType)
}

// String returns a string representation of o.
func (o Output) String() (string, error) {
	var s string
	err := json.Unmarshal(o.Value, &s)
	return s, err
}

// List returns the value of o if o is of type OutputTypeList.
func (o Output) List() ([]string, error) {
	var l []string
	err := json.Unmarshal(o.Value, &l)
	return l, err
}

// Map returns the value of o if o is of type OutputTypeMap. Otherwise it panics
func (o Output) Map() (map[string]string, error) {
	var m map[string]string
	err := json.Unmarshal(o.Value, &m)
	return m, err
}
