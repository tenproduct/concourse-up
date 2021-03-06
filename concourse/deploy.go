package concourse

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"text/template"
	"time"

	"github.com/apparentlymart/go-cidr/cidr"

	"strings"

	"github.com/EngineerBetter/concourse-up/bosh"
	"github.com/EngineerBetter/concourse-up/certs"
	"github.com/EngineerBetter/concourse-up/config"
	"github.com/EngineerBetter/concourse-up/fly"
	"github.com/EngineerBetter/concourse-up/terraform"
	"github.com/xenolf/lego/lego"
	"gopkg.in/yaml.v2"
)

// BoshParams represents the params used and produced by a BOSH deploy
type BoshParams struct {
	CredhubPassword          string
	CredhubAdminClientSecret string
	CredhubCACert            string
	CredhubURL               string
	CredhubUsername          string
	ConcourseUsername        string
	ConcoursePassword        string
	GrafanaPassword          string
	DirectorUsername         string
	DirectorPassword         string
	DirectorCACert           string
}

func stripVersion(tags []string) []string {
	output := []string{}
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "concourse-up-version") {
			output = append(output, tag)
		}
	}
	return output
}

// Deploy deploys a concourse instance
func (client *Client) Deploy() error {
	conf, isDomainUpdated, err := client.getInitialConfig()
	if err != nil {
		return fmt.Errorf("error getting initial config before deploy: [%v]", err)
	}

	r, err := client.checkPreTerraformConfigRequirements(conf, client.deployArgs.SelfUpdate)
	if err != nil {
		return err
	}
	conf.Region = r.Region
	conf.SourceAccessIP = r.SourceAccessIP
	conf.HostedZoneID = r.HostedZoneID
	conf.HostedZoneRecordPrefix = r.HostedZoneRecordPrefix
	conf.Domain = r.Domain

	tfInputVars := client.tfInputVarsFactory.NewInputVars(conf)

	err = client.tfCLI.Apply(tfInputVars)
	if err != nil {
		return err
	}

	tfOutputs, err := client.tfCLI.BuildOutput(tfInputVars)
	if err != nil {
		return err
	}

	err = client.configClient.Update(conf)
	if err != nil {
		return err
	}
	conf.Tags = stripVersion(conf.Tags)
	conf.Tags = append([]string{fmt.Sprintf("concourse-up-version=%s", client.version)}, conf.Tags...)

	conf.Version = client.version

	cr, err := client.checkPreDeployConfigRequirements(client.acmeClientConstructor, isDomainUpdated, conf, tfOutputs)
	if err != nil {
		return err
	}

	conf.Domain = cr.Domain
	conf.DirectorPublicIP = cr.DirectorPublicIP
	conf.DirectorCACert = cr.DirectorCerts.DirectorCACert
	conf.DirectorCert = cr.DirectorCerts.DirectorCert
	conf.DirectorKey = cr.DirectorCerts.DirectorKey
	conf.ConcourseCert = cr.Certs.ConcourseCert
	conf.ConcourseKey = cr.Certs.ConcourseKey
	conf.ConcourseUserProvidedCert = cr.Certs.ConcourseUserProvidedCert
	conf.ConcourseCACert = cr.Certs.ConcourseCACert

	var bp BoshParams
	if client.deployArgs.SelfUpdate {
		bp, err = client.updateBoshAndPipeline(conf, tfOutputs)
	} else {
		bp, err = client.deployBoshAndPipeline(conf, tfOutputs)
	}

	conf.CredhubPassword = bp.CredhubPassword
	conf.CredhubAdminClientSecret = bp.CredhubAdminClientSecret
	conf.CredhubCACert = bp.CredhubCACert
	conf.CredhubURL = bp.CredhubURL
	conf.CredhubUsername = bp.CredhubUsername
	conf.ConcourseUsername = bp.ConcourseUsername
	conf.ConcoursePassword = bp.ConcoursePassword
	conf.GrafanaPassword = bp.GrafanaPassword
	conf.DirectorUsername = bp.DirectorUsername
	conf.DirectorPassword = bp.DirectorPassword
	conf.DirectorCACert = bp.DirectorCACert

	err1 := client.configClient.Update(conf)
	if err == nil {
		err = err1
	}
	return err
}

func (client *Client) deployBoshAndPipeline(c config.Config, tfOutputs terraform.Outputs) (BoshParams, error) {
	// When we are deploying for the first time rather than updating
	// ensure that the pipeline is set _after_ the concourse is deployed

	bp := BoshParams{
		CredhubPassword:          c.CredhubPassword,
		CredhubAdminClientSecret: c.CredhubAdminClientSecret,
		CredhubCACert:            c.CredhubCACert,
		CredhubURL:               c.CredhubURL,
		CredhubUsername:          c.CredhubUsername,
		ConcourseUsername:        c.ConcourseUsername,
		ConcoursePassword:        c.ConcoursePassword,
		GrafanaPassword:          c.GrafanaPassword,
		DirectorUsername:         c.DirectorUsername,
		DirectorPassword:         c.DirectorPassword,
		DirectorCACert:           c.DirectorCACert,
	}

	bp, err := client.deployBosh(c, tfOutputs, false)
	if err != nil {
		return bp, err
	}

	flyClient, err := client.flyClientFactory(client.provider, fly.Credentials{
		Target:   c.Deployment,
		API:      fmt.Sprintf("https://%s", c.Domain),
		Username: bp.ConcourseUsername,
		Password: bp.ConcoursePassword,
	},
		client.stdout,
		client.stderr,
		client.versionFile,
	)
	if err != nil {
		return bp, err
	}
	defer flyClient.Cleanup()

	if err := flyClient.SetDefaultPipeline(c, false); err != nil {
		return bp, err
	}

	// This assignment is necessary for the deploy success message
	// It should be removed once we stop passing config everywhere
	c.ConcourseUsername = bp.ConcourseUsername
	c.ConcoursePassword = bp.ConcoursePassword

	return bp, writeDeploySuccessMessage(c, client.stdout)
}

func (client *Client) updateBoshAndPipeline(c config.Config, tfOutputs terraform.Outputs) (BoshParams, error) {
	// If concourse is already running this is an update rather than a fresh deploy
	// When updating we need to deploy the BOSH as the final step in order to
	// Detach from the update, so the update job can exit

	bp := BoshParams{
		CredhubPassword:          c.CredhubPassword,
		CredhubAdminClientSecret: c.CredhubAdminClientSecret,
		CredhubCACert:            c.CredhubCACert,
		CredhubURL:               c.CredhubURL,
		CredhubUsername:          c.CredhubUsername,
		ConcourseUsername:        c.ConcourseUsername,
		ConcoursePassword:        c.ConcoursePassword,
		GrafanaPassword:          c.GrafanaPassword,
		DirectorUsername:         c.DirectorUsername,
		DirectorPassword:         c.DirectorPassword,
		DirectorCACert:           c.DirectorCACert,
	}

	flyClient, err := client.flyClientFactory(client.provider, fly.Credentials{
		Target:   c.Deployment,
		API:      fmt.Sprintf("https://%s", c.Domain),
		Username: c.ConcourseUsername,
		Password: c.ConcoursePassword,
	},
		client.stdout,
		client.stderr,
		client.versionFile,
	)
	if err != nil {
		return bp, err
	}
	defer flyClient.Cleanup()

	concourseAlreadyRunning, err := flyClient.CanConnect()
	if err != nil {
		return bp, err
	}

	if !concourseAlreadyRunning {
		return bp, fmt.Errorf("In detach mode but it seems that concourse is not currently running")
	}

	// Allow a fly version discrepancy since we might be targetting an older Concourse
	if err = flyClient.SetDefaultPipeline(c, true); err != nil {
		return bp, err
	}

	bp, err = client.deployBosh(c, tfOutputs, true)
	if err != nil {
		return bp, err
	}

	_, err = client.stdout.Write([]byte("\nUPGRADE RUNNING IN BACKGROUND\n\n"))

	return bp, err
}

// TerraformRequirements represents the required values for running terraform
type TerraformRequirements struct {
	Region                 string
	SourceAccessIP         string
	HostedZoneID           string
	HostedZoneRecordPrefix string
	Domain                 string
}

func (client *Client) checkPreTerraformConfigRequirements(conf config.Config, selfUpdate bool) (TerraformRequirements, error) {
	r := TerraformRequirements{
		Region:                 conf.Region,
		SourceAccessIP:         conf.SourceAccessIP,
		HostedZoneID:           conf.HostedZoneID,
		HostedZoneRecordPrefix: conf.HostedZoneRecordPrefix,
		Domain:                 conf.Domain,
	}

	region := client.provider.Region()
	if conf.Region != "" {
		if conf.Region != region {
			return r, fmt.Errorf("found previous deployment in %s. Refusing to deploy to %s as changing regions for existing deployments is not supported", conf.Region, region)
		}
	}

	r.Region = region

	// When in self-update mode do not override the user IP, since we already have access to the worker
	if !selfUpdate {
		var err error
		r.SourceAccessIP, err = client.setUserIP(conf)
		if err != nil {
			return r, err
		}
	}

	zone, err := client.setHostedZone(conf, conf.Domain)
	if err != nil {
		return r, err
	}
	r.HostedZoneID = zone.HostedZoneID
	r.HostedZoneRecordPrefix = zone.HostedZoneRecordPrefix
	r.Domain = zone.Domain

	return r, nil
}

// DirectorCerts represents the certificate of a Director
type DirectorCerts struct {
	DirectorCACert string
	DirectorCert   string
	DirectorKey    string
}

// Certs represents the certificate of a Concourse
type Certs struct {
	ConcourseCert             string
	ConcourseKey              string
	ConcourseUserProvidedCert bool
	ConcourseCACert           string
}

// Requirements represents the pre deployment requirements of a Concourse
type Requirements struct {
	Domain           string
	DirectorPublicIP string
	DirectorCerts    DirectorCerts
	Certs            Certs
}

func (client *Client) checkPreDeployConfigRequirements(c func(u *certs.User) (*lego.Client, error), isDomainUpdated bool, cfg config.Config, tfOutputs terraform.Outputs) (Requirements, error) {
	cr := Requirements{
		Domain:           cfg.Domain,
		DirectorPublicIP: cfg.DirectorPublicIP,
	}

	if cfg.Domain == "" {
		domain, err := tfOutputs.Get("ATCPublicIP")
		if err != nil {
			return cr, err
		}
		cr.Domain = domain
	}

	dc := DirectorCerts{
		DirectorCACert: cfg.DirectorCACert,
		DirectorCert:   cfg.DirectorCert,
		DirectorKey:    cfg.DirectorKey,
	}

	dc, err := client.ensureDirectorCerts(c, dc, cfg.Deployment, tfOutputs, cfg.PublicCIDR)
	if err != nil {
		return cr, err
	}

	cr.DirectorCerts = dc

	cc := Certs{
		ConcourseCert:             cfg.ConcourseCert,
		ConcourseKey:              cfg.ConcourseKey,
		ConcourseUserProvidedCert: cfg.ConcourseUserProvidedCert,
		ConcourseCACert:           cfg.ConcourseCACert,
	}

	cc, err = client.ensureConcourseCerts(c, isDomainUpdated, cc, cfg.Deployment, cr.Domain)
	if err != nil {
		return cr, err
	}

	cr.Certs = cc

	cr.DirectorPublicIP, err = tfOutputs.Get("DirectorPublicIP")
	if err != nil {
		return cr, err
	}

	return cr, nil
}

func (client *Client) ensureDirectorCerts(c func(u *certs.User) (*lego.Client, error), dc DirectorCerts, deployment string, tfOutputs terraform.Outputs, publicCIDR string) (DirectorCerts, error) {
	// If we already have director certificates, don't regenerate as changing them will
	// force a bosh director re-deploy even if there are no other changes
	certs := dc
	if certs.DirectorCACert != "" {
		return certs, nil
	}

	// @Note: Duplicate code retrieving director internal IP needs to find a home
	_, pubCIDR, err1 := net.ParseCIDR(publicCIDR)
	if err1 != nil {
		return certs, nil
	}
	directorInternalIP, err1 := cidr.Host(pubCIDR, 6)
	if err1 != nil {
		return certs, nil
	}

	ip, err := tfOutputs.Get("DirectorPublicIP")
	if err != nil {
		return certs, err
	}
	_, err = client.stdout.Write(
		[]byte(fmt.Sprintf("\nGENERATING BOSH DIRECTOR CERTIFICATE (%s, %s)\n", ip, directorInternalIP.String())))
	if err != nil {
		return certs, err
	}

	directorCerts, err := client.certGenerator(c, deployment, client.provider, ip, directorInternalIP.String())
	if err != nil {
		return certs, err
	}

	certs.DirectorCACert = string(directorCerts.CACert)
	certs.DirectorCert = string(directorCerts.Cert)
	certs.DirectorKey = string(directorCerts.Key)

	return certs, nil
}

func timeTillExpiry(cert string) time.Duration {
	block, _ := pem.Decode([]byte(cert))
	if block == nil {
		return 0
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0
	}
	return time.Until(c.NotAfter)
}

func (client *Client) ensureConcourseCerts(c func(u *certs.User) (*lego.Client, error), domainUpdated bool, cc Certs, deployment, domain string) (Certs, error) {
	certs := cc

	if client.deployArgs.TLSCert != "" {
		certs.ConcourseCert = client.deployArgs.TLSCert
		certs.ConcourseKey = client.deployArgs.TLSKey
		certs.ConcourseUserProvidedCert = true

		return certs, nil
	}

	// Skip concourse re-deploy if certs have already been set,
	// unless domain has changed
	if certs.ConcourseCert != "" && !domainUpdated && timeTillExpiry(certs.ConcourseCert) > 28*24*time.Hour {
		return certs, nil
	}

	// If no domain has been provided by the user, the value of cfg.Domain is set to the ATC's public IP in checkPreDeployConfigRequirements
	Certs, err := client.certGenerator(c, deployment, client.provider, domain)
	if err != nil {
		return certs, err
	}

	certs.ConcourseCert = string(Certs.Cert)
	certs.ConcourseKey = string(Certs.Key)
	certs.ConcourseCACert = string(Certs.CACert)

	return certs, nil
}

func (client *Client) deployBosh(config config.Config, tfOutputs terraform.Outputs, detach bool) (BoshParams, error) {
	bp := BoshParams{
		CredhubPassword:          config.CredhubPassword,
		CredhubAdminClientSecret: config.CredhubAdminClientSecret,
		CredhubCACert:            config.CredhubCACert,
		CredhubURL:               config.CredhubURL,
		CredhubUsername:          config.CredhubUsername,
		ConcourseUsername:        config.ConcourseUsername,
		ConcoursePassword:        config.ConcoursePassword,
		GrafanaPassword:          config.GrafanaPassword,
		DirectorUsername:         config.DirectorUsername,
		DirectorPassword:         config.DirectorPassword,
		DirectorCACert:           config.DirectorCACert,
	}

	boshClient, err := client.buildBoshClient(config, tfOutputs)
	if err != nil {
		return bp, err
	}
	defer boshClient.Cleanup()

	boshStateBytes, err := loadDirectorState(client.configClient)
	if err != nil {
		return bp, err
	}
	boshCredsBytes, err := loadDirectorCreds(client.configClient)
	if err != nil {
		return bp, err
	}

	boshStateBytes, boshCredsBytes, err = boshClient.Deploy(boshStateBytes, boshCredsBytes, detach)
	err1 := client.configClient.StoreAsset(bosh.StateFilename, boshStateBytes)
	if err == nil {
		err = err1
	}
	err1 = client.configClient.StoreAsset(bosh.CredsFilename, boshCredsBytes)
	if err == nil {
		err = err1
	}
	if err != nil {
		return bp, err
	}

	var cc struct {
		CredhubPassword          string `yaml:"credhub_cli_password"`
		CredhubAdminClientSecret string `yaml:"credhub_admin_client_secret"`
		InternalTLS              struct {
			CA string `yaml:"ca"`
		} `yaml:"internal_tls"`
		AtcPassword string `yaml:"atc_password"`
	}

	err = yaml.Unmarshal(boshCredsBytes, &cc)
	if err != nil {
		return bp, err
	}

	bp.CredhubPassword = cc.CredhubPassword
	bp.CredhubAdminClientSecret = cc.CredhubAdminClientSecret
	bp.CredhubCACert = cc.InternalTLS.CA
	bp.CredhubURL = fmt.Sprintf("https://%s:8844/", config.Domain)
	bp.CredhubUsername = "credhub-cli"
	bp.ConcourseUsername = "admin"
	if len(cc.AtcPassword) > 0 {
		bp.ConcoursePassword = cc.AtcPassword
		bp.GrafanaPassword = cc.AtcPassword
	}

	return bp, nil
}

func (client *Client) setUserIP(c config.Config) (string, error) {
	sourceAccessIP := c.SourceAccessIP
	userIP, err := client.ipChecker()
	if err != nil {
		return sourceAccessIP, err
	}

	if sourceAccessIP != userIP {
		sourceAccessIP = userIP
		_, err = client.stderr.Write([]byte(fmt.Sprintf(
			"\nWARNING: allowing access from local machine (address: %s)\n\n", userIP)))
		if err != nil {
			return sourceAccessIP, err
		}
	}

	return sourceAccessIP, nil
}

// HostedZone represents a DNS hosted zone
type HostedZone struct {
	HostedZoneID           string
	HostedZoneRecordPrefix string
	Domain                 string
}

func (client *Client) setHostedZone(c config.Config, domain string) (HostedZone, error) {
	zone := HostedZone{
		HostedZoneID:           c.HostedZoneID,
		HostedZoneRecordPrefix: c.HostedZoneRecordPrefix,
		Domain:                 c.Domain,
	}
	if domain == "" {
		return zone, nil
	}

	hostedZoneName, hostedZoneID, err := client.provider.FindLongestMatchingHostedZone(domain)
	if err != nil {
		return zone, err
	}
	zone.HostedZoneID = hostedZoneID
	zone.HostedZoneRecordPrefix = strings.TrimSuffix(domain, fmt.Sprintf(".%s", hostedZoneName))
	zone.Domain = domain

	_, err = client.stderr.Write([]byte(fmt.Sprintf(
		"\nWARNING: adding record %s to DNS zone %s with name %s\n\n", domain, hostedZoneName, hostedZoneID)))
	if err != nil {
		return zone, err
	}
	return zone, err
}

//Temporarily sets the IAAS flag just for GCP as we are still defaulting to AWS
const deployMsg = `DEPLOY SUCCESSFUL. Log in with:
fly --target {{.Project}} login{{if not .ConcourseUserProvidedCert}} --insecure{{end}} --concourse-url https://{{.Domain}} --username {{.ConcourseUsername}} --password {{.ConcoursePassword}}

Metrics available at https://{{.Domain}}:3000 using the same username and password

Log into credhub with:
eval "$(concourse-up info --region {{.Region}} {{ if ne .Namespace .Region }} --namespace {{ .Namespace }} {{ end }} --iaas {{ .IAAS }} --env {{.Project}})"
`

func writeDeploySuccessMessage(config config.Config, stdout io.Writer) error {
	t := template.Must(template.New("deploy").Parse(deployMsg))
	return t.Execute(stdout, config)
}

func writeConfigLoadedSuccessMessage(stdout io.Writer) error {
	_, err := stdout.Write([]byte("\nUSING PREVIOUS DEPLOYMENT CONFIG\n"))

	return err
}

func loadDirectorState(configClient config.IClient) ([]byte, error) {
	hasState, err := configClient.HasAsset(bosh.StateFilename)
	if err != nil {
		return nil, err
	}

	if !hasState {
		return nil, nil
	}

	return configClient.LoadAsset(bosh.StateFilename)
}
func loadDirectorCreds(configClient config.IClient) ([]byte, error) {
	hasCreds, err := configClient.HasAsset(bosh.CredsFilename)
	if err != nil {
		return nil, err
	}

	if !hasCreds {
		return nil, nil
	}

	return configClient.LoadAsset(bosh.CredsFilename)
}
