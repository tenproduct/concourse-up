package iaas

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	clouddns "google.golang.org/api/dns/v1"
	"google.golang.org/api/iterator"

	// PostgreSQL driver required at runtime
	_ "github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/dialers/postgres"
)

// GCPProvider is the concrete implementation of GCP Provider
type GCPProvider struct {
	ctx     context.Context
	storage GCPStorageClient
	region  string
	attrs   map[string]string
}

// GCPOption is the signature of the option function
type GCPOption func(*GCPProvider) error

// GCPStorageClient is the interface with GCP storage client
type GCPStorageClient interface {
	Bucket(name string) *storage.BucketHandle
	Buckets(ctx context.Context, projectID string) *storage.BucketIterator
}

// GCPStorage returns an option function with storage initialised
func GCPStorage() GCPOption {
	return func(c *GCPProvider) error {
		s, err := storage.NewClient(c.ctx)
		if err != nil {
			return err
		}
		c.storage = s
		return nil
	}
}

func newGCP(region string, ops ...GCPOption) (Provider, error) {
	project, path, err := getCredentials()
	if err != nil {
		return nil, err
	}
	attrs := make(map[string]string)
	attrs["project"] = project
	attrs["credentials_path"] = path

	ctx := context.Background()

	g := &GCPProvider{ctx, &storage.Client{}, region, attrs}
	for _, op := range ops {
		if err := op(g); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// GCPDBSizes maps user set size to GCP specific machine type
var GCPDBSizes = map[string]string{
	"small":   "db-g1-small",
	"medium":  "db-custom-2-4096",
	"large":   "db-custom-2-8192",
	"xlarge":  "db-custom-4-16384",
	"2xlarge": "db-custom-8-32768",
	"4xlarge": "db-custom-16-65536",
}

// DBType gets the correct CloudSQL db tier
func (g *GCPProvider) DBType(name string) string {
	return GCPDBSizes[name]
}

// Attr returns GCP specific attribute
func (g *GCPProvider) Attr(key string) (string, error) {
	v, ok := g.attrs[key]
	if !ok {
		return "", fmt.Errorf("iaas:gcp: key %s not found", key)
	}
	return v, nil
}

// DeleteFile deletes a file from GCP bucket
func (g *GCPProvider) DeleteFile(bucket, path string) error {
	o := g.storage.Bucket(bucket).Object(path)

	if err := o.Delete(g.ctx); err != nil {
		return err
	}

	return nil
}

// Choose for the consumer the appropriate output based on the provider
func (g *GCPProvider) Choose(c Choice) interface{} {
	return c.GCP
}

// DeleteVersionedBucket deletes a bucket and its content from GCP
func (g *GCPProvider) DeleteVersionedBucket(name string) error {
	err := g.DeleteFile(name, "config.json")
	if err != nil {
		return err
	}
	err = g.DeleteFile(name, "default.tfstate")
	if err != nil {
		return err
	}
	err = g.DeleteFile(name, "director-creds.yml")
	if err != nil {
		return err
	}
	err = g.DeleteFile(name, "director-state.json")
	if err != nil {
		return err
	}
	err = g.DeleteFile(name, "maintenance.json")
	if err != nil && err.Error() != "storage: object doesn't exist" {
		return err
	}
	err = g.DeleteFile(name, "director-creds-backup.yml")
	if err != nil && err.Error() != "storage: object doesn't exist" {
		return err
	}
	time.Sleep(time.Second)
	if err := g.storage.Bucket(name).Delete(g.ctx); err != nil {
		return err
	}

	return nil
}

// CreateBucket creates a GCP storage bucket with defaults of the US multi-regional location, and a storage class of Standard Storage
func (g *GCPProvider) CreateBucket(name string) error {
	project, err := g.Attr("project")
	if err != nil {
		return err
	}
	if err := g.storage.Bucket(name).Create(g.ctx, project, nil); err != nil {
		return err
	}
	return nil
}

// BucketExists checks if the named bucket exists
func (g *GCPProvider) BucketExists(name string) (bool, error) {
	project, err := g.Attr("project")
	if err != nil {
		return false, err
	}
	it := g.storage.Buckets(g.ctx, project)
	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {

			return false, err
		}
		if battrs.Name == name {

			return true, nil
		}
	}

	return false, nil
}

// HasFile returns true if the specified GCP file exists
func (g *GCPProvider) HasFile(bucket, path string) (bool, error) {
	o := g.storage.Bucket(bucket).Object(path)
	_, err := o.Attrs(g.ctx)

	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// LoadFile loads a file from GCP bucket
func (g *GCPProvider) LoadFile(bucket, path string) ([]byte, error) {
	rc, err := g.storage.Bucket(bucket).Object(path).NewReader(g.ctx)

	if err != nil {
		return nil, err
	}

	defer rc.Close()
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// WriteFile writes the specified file to GCP storage
func (g *GCPProvider) WriteFile(bucket, path string, contents []byte) error {
	wc := g.storage.Bucket(bucket).Object(path).NewWriter(g.ctx)
	defer wc.Close()

	if _, err := wc.Write(contents); err != nil {
		return err
	}

	return nil
}

// Region returns the region used by the Provider
func (g *GCPProvider) Region() string {
	return g.region
}

//TODO: Choose an appropriate zone based on what zones the region has

// Zone returns the zone used by the Provider
func (g *GCPProvider) Zone(input string) string {
	if input != "" {
		return input
	}
	return fmt.Sprintf("%s-b", g.region)
}

// IAAS returns the name of the Provider
func (g *GCPProvider) IAAS() Name {
	return GCP
}

// EnsureFileExists checks for the named file in GCP storage and creates it if it doesn't exist
// Second argument is true if new file was created
func (g *GCPProvider) EnsureFileExists(bucket, path string, defaultContents []byte) ([]byte, bool, error) {
	contents, err := g.LoadFile(bucket, path)

	if err == nil {
		return contents, false, nil
	}

	if err != storage.ErrObjectNotExist {
		return nil, false, err
	}

	err = g.WriteFile(bucket, path, defaultContents)
	if err != nil {
		return nil, false, err
	}
	return defaultContents, true, nil
}

// DeleteVolumes deletes the specified GCP storage volumes
func (g *GCPProvider) DeleteVolumes(volumesToDelete []string, deleteVolume func(ec2Client IEC2, volumeID *string) error) error {
	// @note: This will be covered in a later iteration as we need a deployment to try it
	return errors.New("DeleteVolumes Not Implemented Yet")
}

// CheckForWhitelistedIP checks if the specified IP is whitelisted in the security group
func (g *GCPProvider) CheckForWhitelistedIP(ip, firewallName string) (bool, error) {

	parsedIP := net.ParseIP(ip)

	c, err := google.DefaultClient(g.ctx, compute.CloudPlatformScope)
	if err != nil {
		return false, err
	}

	computeService, err := compute.New(c)
	if err != nil {
		return false, err
	}

	project, err := g.Attr("project")
	if err != nil {
		return false, err
	}

	// gets all compute instances for the project
	req := computeService.Firewalls.List(project)
	var sourceRanges []string
	if err := req.Pages(g.ctx, func(page *compute.FirewallList) error {
		for _, firewall := range page.Items {
			if firewall.Name == firewallName {
				sourceRanges = firewall.SourceRanges
			}
		}
		return nil
	}); err != nil {
		return false, err
	}
	for _, cidr := range sourceRanges {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil {
			return false, err
		}
		if parsedCIDR.Contains(parsedIP) {
			return true, nil
		}
	}
	return false, nil
}

// DeleteVMsInVPC is a placeholder function used with AWS deployments
func (g *GCPProvider) DeleteVMsInVPC(vpcID string) ([]string, error) {
	return []string{}, nil
}

//DeleteVMsInDeployment will delete all vms in a deployment apart from nat instance
func (g *GCPProvider) DeleteVMsInDeployment(zone, project, deployment string) error {
	c, err := google.DefaultClient(g.ctx, compute.CloudPlatformScope)
	if err != nil {
		log.Fatal(err)
	}

	computeService, err := compute.New(c)
	if err != nil {
		log.Fatal(err)
	}

	// gets all compute instances for the project
	req := computeService.Instances.List(project, zone)
	if err := req.Pages(g.ctx, func(page *compute.InstanceList) error {
		for _, instance := range page.Items {
			name := instance.Name
			networkName := instance.NetworkInterfaces[0].Network
			// delete all instances in deployment's network apart from nat instance
			if strings.HasSuffix(networkName, deployment) {
				for _, disk := range instance.Disks {
					fmt.Printf("Marking instance %s volume for deletion\n", name)
					computeService.Instances.SetDiskAutoDelete(project, zone, name, true, disk.DeviceName).Context(g.ctx).Do()
				}
				if !strings.HasSuffix(name, "nat-instance") {
					fmt.Printf("Deleting instance %+v\n", name)
					_, err := computeService.Instances.Delete(project, zone, name).Context(g.ctx).Do()
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	start := time.Now().UTC()
	for {
		found := false
		req = computeService.Instances.List(project, zone)

		if err := req.Pages(g.ctx, func(page *compute.InstanceList) error {
			for _, instance := range page.Items {
				name := instance.Name
				networkName := instance.NetworkInterfaces[0].Network
				if strings.HasSuffix(networkName, deployment) && !strings.HasSuffix(name, "nat-instance") {
					found = true
					fmt.Printf("Waiting for instance %s to be deleted\n", name)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if !found {
			return nil
		}
		if time.Since(start) > time.Second*180 {
			return fmt.Errorf("Instances not deleted after 3 minutes")
		}
		time.Sleep(time.Second * 10)
	}
}

// FindLongestMatchingHostedZone finds the longest hosted zone that matches the given subdomain
func (g *GCPProvider) FindLongestMatchingHostedZone(domain string) (string, string, error) {
	c, err := google.DefaultClient(g.ctx, compute.CloudPlatformScope)
	if err != nil {
		return "", "", err
	}

	cloudDNSService, err := clouddns.New(c)
	if err != nil {
		return "", "", err
	}

	var dnsNameFound, nameFound string
	req := cloudDNSService.ManagedZones.List(g.attrs["project"])
	err = req.Pages(g.ctx, func(page *clouddns.ManagedZonesListResponse) error {
		for _, zone := range page.ManagedZones {
			name := zone.Name
			dnsName := strings.TrimRight(zone.DnsName, ".")
			if strings.HasSuffix(domain, dnsName) && len(dnsName) > len(dnsNameFound) {
				dnsNameFound = dnsName
				nameFound = name
			}
		}
		return nil
	})

	if dnsNameFound == "" || nameFound == "" {
		return "", "", fmt.Errorf("dns zone for domain '%s' was not found in cloudDNS", domain)
	}

	return dnsNameFound, nameFound, err
}
func getCredentials() (string, string, error) {
	credsStruct := make(map[string]interface{})

	path, exists := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	if !exists {
		return "", "", fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS is not set")
	}

	jsonFile, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("File %v not found", path)
	}
	defer jsonFile.Close()
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return "", "", fmt.Errorf("Unable to read file %v", path)
	}
	json.Unmarshal(byteValue, &credsStruct)
	projectID, ok := credsStruct["project_id"]
	if !ok {
		return "", "", fmt.Errorf("project_id not found in %v", path)
	}
	return projectID.(string), path, nil
}

var gcpDB *sql.DB

// WorkerType is a nil setter for workerType
func (g *GCPProvider) WorkerType(w string) {}

// CreateDatabases creates databases on the server
func (g *GCPProvider) CreateDatabases(name, username, password string) error {
	project, err := g.Attr("project")
	if err != nil {
		return err
	}
	conn := fmt.Sprintf("host=%s:%s:%s user=%s dbname=postgres password=%s sslmode=disable", project, g.Region(), name, username, password)

	gcpDB, err := sql.Open("cloudsqlpostgres", conn)
	if err != nil {
		return err
	}
	defer gcpDB.Close()
	dbNames := []string{"concourse_atc", "uaa", "credhub"}
	for _, dbName := range dbNames {
		_, err := gcpDB.Exec("CREATE DATABASE " + dbName)
		if err != nil && !strings.Contains(err.Error(),
			fmt.Sprintf(`pq: database "%s" already exists`, dbName)) {
			return err
		}
	}
	return nil
}
