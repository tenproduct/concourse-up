package bosh

import (
	"io"

	"github.com/EngineerBetter/concourse-up/bosh/internal/boshcli"
	"github.com/EngineerBetter/concourse-up/bosh/internal/workingdir"
	"github.com/EngineerBetter/concourse-up/config"
	"github.com/EngineerBetter/concourse-up/iaas"
	"github.com/EngineerBetter/concourse-up/terraform"
)

//GCPClient is an GCP specific implementation of IClient
type GCPClient struct {
	config     config.Config
	outputs    terraform.Outputs
	workingdir workingdir.IClient
	stdout     io.Writer
	stderr     io.Writer
	provider   iaas.Provider
	boshCLI    boshcli.ICLI
}

//NewGCPClient returns a GCP specific implementation of IClient
func NewGCPClient(config config.Config, outputs terraform.Outputs, workingdir workingdir.IClient, stdout, stderr io.Writer, provider iaas.Provider, boshCLI boshcli.ICLI) (IClient, error) {
	return &GCPClient{
		config:     config,
		outputs:    outputs,
		workingdir: workingdir,
		stdout:     stdout,
		stderr:     stderr,
		provider:   provider,
		boshCLI:    boshCLI,
	}, nil
}

//Cleanup is GCP specific implementation of Cleanup
func (client *GCPClient) Cleanup() error {
	return client.workingdir.Cleanup()
}
