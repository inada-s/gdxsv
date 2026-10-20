package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// On-demand battle servers on GCE (GDXSV_GCE_ONDEMAND). The lbs keeps one VM per role and
// region, named gdxsv-<role>-<region>-latest: a running one is reused, a stopped one is
// started again with a fresh startup script, else one is created in the first zone of the
// region that accepts it. A VM that stayed stopped for gceUnusedVMLifetime is deleted.
//
// role "mcs" (battle server): e2-medium in the default network.
// role "relay" (GGPO relay): e2-micro in the dual-stack gceRelayNetwork, so it gets an
// external IPv6 address; it needs GDXSV_RELAY_SECRET, written into its startup script.
//
// The VMs run as gdxsv-service@<project>.iam.gserviceaccount.com, download the latest
// gdxsv release and connect back to GDXSV_LOBBY_PUBLIC_ADDR.

const (
	gceVersion          = "latest"
	gceRelayNetwork     = "gdxsv-relay"
	gceImage            = "projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts"
	gceAllocInterval    = 30 * time.Second
	gceCleanupInterval  = time.Hour
	gceUnusedVMLifetime = 5 * 24 * time.Hour
)

var (
	gceAllocMu       sync.Mutex
	gceAllocTime     = map[string]time.Time{}
	gceCleanupTime   time.Time
	gceServiceMu     sync.Mutex
	gceServiceCached gceAPI
	relaySecretRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{16,}$`)
)

var gcpLocationName = map[string]string{
	"asia-east1":              "Changhua County, Taiwan",
	"asia-east2":              "Hong Kong",
	"asia-northeast1":         "Tokyo, Japan",
	"asia-northeast2":         "Osaka, Japan",
	"asia-northeast3":         "Seoul, South Korea",
	"asia-south1":             "Mumbai, India",
	"asia-southeast1":         "Jurong West, Singapore",
	"australia-southeast1":    "Sydney, Australia",
	"europe-north1":           "Hamina, Finland",
	"europe-west1":            "St. Ghislain, Belgium",
	"europe-west2":            "London, England, UK",
	"europe-west3":            "Frankfurt, Germany",
	"europe-west4":            "Eemshaven, Netherlands",
	"europe-west6":            "Zurich, Switzerland",
	"northamerica-northeast1": "Montreal, Quebec, Canada",
	"southamerica-east1":      "Osasco (Sao Paulo), Brazil",
	"us-central1":             "Council Bluffs, Iowa, USA",
	"us-east1":                "Moncks Corner, South Carolina, USA",
	"us-east4":                "Ashburn, Northern Virginia, USA",
	"us-west1":                "The Dalles, Oregon, USA",
	"us-west2":                "Los Angeles, California, USA",
	"us-west3":                "Salt Lake City, Utah, USA",
}

var gcpRegionGroup = map[string]string{
	"asia-east1":              "asia-east",
	"asia-east2":              "asia-east",
	"asia-northeast1":         "asia-northeast",
	"asia-northeast2":         "asia-northeast",
	"asia-northeast3":         "asia-northeast",
	"asia-south1":             "asia-south",
	"asia-southeast1":         "asia-southeast",
	"australia-southeast1":    "australia-southeast",
	"europe-north1":           "europe-north",
	"europe-west1":            "europe-west",
	"europe-west2":            "europe-west",
	"europe-west3":            "europe-west",
	"europe-west4":            "europe-west",
	"europe-west6":            "europe-west",
	"northamerica-northeast1": "northamerica-northeast",
	"southamerica-east1":      "southamerica-east",
	"us-central1":             "us-central",
	"us-east1":                "us-east",
	"us-east4":                "us-east",
	"us-west1":                "us-west",
	"us-west2":                "us-west",
	"us-west3":                "us-west",
}

// gcpRegionZones: the zones a VM may be created in, tried in order.
// https://cloud.google.com/compute/docs/regions-zones
var gcpRegionZones = map[string][]string{
	"asia-east1":              {"a", "b", "c"},
	"asia-east2":              {"a", "b", "c"},
	"asia-northeast1":         {"a", "b", "c"},
	"asia-northeast2":         {"a", "b", "c"},
	"asia-northeast3":         {"a", "b", "c"},
	"asia-south1":             {"a", "b", "c"},
	"asia-southeast1":         {"a", "b", "c"},
	"australia-southeast1":    {"a", "b", "c"},
	"europe-north1":           {"a", "b", "c"},
	"europe-west1":            {"b", "c", "d"},
	"europe-west2":            {"a", "b", "c"},
	"europe-west3":            {"a", "b", "c"},
	"europe-west4":            {"a", "b", "c"},
	"europe-west6":            {"a", "b", "c"},
	"northamerica-northeast1": {"a", "b", "c"},
	"southamerica-east1":      {"a", "b", "c"},
	"us-central1":             {"a", "b", "c", "f"},
	"us-east1":                {"b", "c", "d"},
	"us-east4":                {"a", "b", "c"},
	"us-west1":                {"a", "b", "c"},
	"us-west2":                {"a", "b", "c"},
	"us-west3":                {"a", "b", "c"},
}

// gceAPI is the part of the Compute Engine API the allocator uses; each call waits for
// its operation. gceFake in the tests implements it too.
type gceAPI interface {
	// listInstances lists the project's instances whose name starts with namePrefix.
	listInstances(ctx context.Context, namePrefix string) ([]*compute.Instance, error)
	getInstance(ctx context.Context, zone, name string) (*compute.Instance, error)
	setMetadata(ctx context.Context, zone, name string, metadata *compute.Metadata) error
	setScheduling(ctx context.Context, zone, name string, scheduling *compute.Scheduling) error
	startInstance(ctx context.Context, zone, name string) error
	insertInstance(ctx context.Context, zone string, instance *compute.Instance) error
	deleteInstance(ctx context.Context, zone, name string) error
}

type gceService struct {
	project string
	svc     *compute.Service
}

func (g *gceService) listInstances(ctx context.Context, namePrefix string) ([]*compute.Instance, error) {
	var list []*compute.Instance
	err := g.svc.Instances.AggregatedList(g.project).Filter(fmt.Sprintf("name eq %s.*", namePrefix)).
		Pages(ctx, func(page *compute.InstanceAggregatedList) error {
			for _, scoped := range page.Items {
				list = append(list, scoped.Instances...)
			}
			return nil
		})
	return list, err
}

func (g *gceService) getInstance(ctx context.Context, zone, name string) (*compute.Instance, error) {
	return g.svc.Instances.Get(g.project, zone, name).Context(ctx).Do()
}

func (g *gceService) setMetadata(ctx context.Context, zone, name string, metadata *compute.Metadata) error {
	op, err := g.svc.Instances.SetMetadata(g.project, zone, name, metadata).Context(ctx).Do()
	if err != nil {
		return err
	}
	return g.wait(ctx, zone, op)
}

func (g *gceService) setScheduling(ctx context.Context, zone, name string, scheduling *compute.Scheduling) error {
	op, err := g.svc.Instances.SetScheduling(g.project, zone, name, scheduling).Context(ctx).Do()
	if err != nil {
		return err
	}
	return g.wait(ctx, zone, op)
}

func (g *gceService) startInstance(ctx context.Context, zone, name string) error {
	op, err := g.svc.Instances.Start(g.project, zone, name).Context(ctx).Do()
	if err != nil {
		return err
	}
	return g.wait(ctx, zone, op)
}

func (g *gceService) insertInstance(ctx context.Context, zone string, instance *compute.Instance) error {
	op, err := g.svc.Instances.Insert(g.project, zone, instance).Context(ctx).Do()
	if err != nil {
		return err
	}
	return g.wait(ctx, zone, op)
}

func (g *gceService) deleteInstance(ctx context.Context, zone, name string) error {
	op, err := g.svc.Instances.Delete(g.project, zone, name).Context(ctx).Do()
	if err != nil {
		return err
	}
	return g.wait(ctx, zone, op)
}

// wait blocks until the zone operation is done and returns its error, if any.
func (g *gceService) wait(ctx context.Context, zone string, op *compute.Operation) error {
	for op.Status != "DONE" {
		var err error
		op, err = g.svc.ZoneOperations.Wait(g.project, zone, op.Name).Context(ctx).Do()
		if err != nil {
			return err
		}
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		e := op.Error.Errors[0]
		return fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return nil
}

func getGceAPI() (gceAPI, error) {
	gceServiceMu.Lock()
	defer gceServiceMu.Unlock()
	if gceServiceCached != nil {
		return gceServiceCached, nil
	}
	var opts []option.ClientOption
	if conf.GCPKeyPath != "" {
		opts = append(opts, option.WithAuthCredentialsFile(option.ServiceAccount, conf.GCPKeyPath))
	}
	svc, err := compute.NewService(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	gceServiceCached = &gceService{project: conf.GCPProjectID, svc: svc}
	return gceServiceCached, nil
}

// GceAllocEnabled reports whether the lbs may start battle servers on GCE.
func GceAllocEnabled() bool {
	return conf.GCEOnDemand && conf.GCPProjectID != ""
}

// GoGceAlloc starts a mcs in the region in the background, at most once per
// gceAllocInterval. It reports whether a request was made.
func GoGceAlloc(region string) bool {
	return GoGceAllocRole(region, "mcs")
}

// GoGceAllocRole starts a VM of the role ("mcs" or "relay") in the region in the
// background, at most once per gceAllocInterval. It reports whether a request was made.
func GoGceAllocRole(region, role string) bool {
	key := "alloc/" + role + "/" + region
	gceAllocMu.Lock()
	if time.Since(gceAllocTime[key]) <= gceAllocInterval {
		gceAllocMu.Unlock()
		return false
	}
	gceAllocTime[key] = time.Now()
	gceAllocMu.Unlock()

	go func() {
		api, err := getGceAPI()
		if err != nil {
			logger.Error("gce alloc: client", zap.Error(err))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		vm, err := gceAlloc(ctx, api, gceAllocParams{
			Project:         conf.GCPProjectID,
			Region:          region,
			Role:            role,
			LobbyPublicAddr: conf.LobbyPublicAddr,
			RelaySecret:     conf.RelaySecret,
		})
		if err != nil {
			logger.Error("gce alloc failed", zap.String("role", role), zap.String("region", region), zap.Error(err))
			return
		}
		logger.Info("gce alloc", zap.String("role", role), zap.String("region", region),
			zap.String("name", vm.Name), zap.String("zone", path.Base(vm.Zone)), zap.String("status", vm.Status))
	}()
	return true
}

type gceAllocParams struct {
	Project         string
	Region          string
	Role            string // "mcs" or "relay"
	LobbyPublicAddr string
	RelaySecret     string
}

func gceVMName(role, region string) string {
	return "gdxsv-" + role + "-" + region + "-" + strings.ReplaceAll(gceVersion, ".", "-")
}

func gceServiceAccount(project string) string {
	return "gdxsv-service@" + project + ".iam.gserviceaccount.com"
}

// gceAlloc makes sure a VM of the role runs in the region and returns it.
func gceAlloc(ctx context.Context, api gceAPI, p gceAllocParams) (*compute.Instance, error) {
	zones, ok := gcpRegionZones[p.Region]
	if !ok {
		return nil, fmt.Errorf("invalid region %q", p.Region)
	}
	if p.Role != "mcs" && p.Role != "relay" {
		return nil, fmt.Errorf("invalid role %q", p.Role)
	}
	if p.Role == "relay" && !relaySecretRe.MatchString(p.RelaySecret) {
		return nil, errors.New("GDXSV_RELAY_SECRET is not set or not plain [A-Za-z0-9_-]{16,}")
	}
	if p.LobbyPublicAddr == "" {
		return nil, errors.New("GDXSV_LOBBY_PUBLIC_ADDR is empty")
	}

	gceRemoveUnusedVMs(ctx, api)

	name := gceVMName(p.Role, p.Region)
	startupScript := gceStartupScript(p)
	metadata := &compute.Metadata{Items: []*compute.MetadataItems{
		{Key: "startup-script", Value: &startupScript},
		{Key: "enable-osconfig", Value: strPtr("TRUE")},
		{Key: "enable-guest-attributes", Value: strPtr("TRUE")},
	}}
	scheduling := &compute.Scheduling{
		OnHostMaintenance: "MIGRATE",
		Preemptible:       false,
		AutomaticRestart:  boolPtr(false),
		ForceSendFields:   []string{"Preemptible", "AutomaticRestart"},
	}

	vms, err := api.listInstances(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	for _, vm := range vms {
		if vm.Name == name && vm.Status == "RUNNING" {
			return vm, nil
		}
	}

	for _, vm := range vms {
		if vm.Name != name || vm.Status != "TERMINATED" {
			continue
		}
		zone := path.Base(vm.Zone)
		logger.Info("gce alloc: starting stopped vm", zap.String("name", name), zap.String("zone", zone))
		if vm.Metadata != nil {
			metadata.Fingerprint = vm.Metadata.Fingerprint
		}
		if err := api.setMetadata(ctx, zone, name, metadata); err != nil {
			logger.Warn("gce alloc: set metadata", zap.String("zone", zone), zap.Error(err))
			continue
		}
		if err := api.setScheduling(ctx, zone, name, scheduling); err != nil {
			logger.Warn("gce alloc: set scheduling", zap.String("zone", zone), zap.Error(err))
			continue
		}
		if err := api.startInstance(ctx, zone, name); err != nil {
			logger.Warn("gce alloc: start", zap.String("zone", zone), zap.Error(err))
			continue
		}
		return api.getInstance(ctx, zone, name)
	}

	var lastErr error
	for _, z := range zones {
		zone := p.Region + "-" + z
		logger.Info("gce alloc: creating vm", zap.String("name", name), zap.String("zone", zone))
		inst := gceNewInstance(p, name, zone, metadata, scheduling)
		if err := api.insertInstance(ctx, zone, inst); err != nil {
			logger.Warn("gce alloc: insert", zap.String("zone", zone), zap.Error(err))
			lastErr = err
			continue
		}
		return api.getInstance(ctx, zone, name)
	}
	return nil, fmt.Errorf("failed to allocate vm in %s: %w", p.Region, lastErr)
}

// gceNewInstance is the VM of the role: the ubuntu image, the role's machine type, tags,
// network and the gdxsv service account.
func gceNewInstance(p gceAllocParams, name, zone string, metadata *compute.Metadata, scheduling *compute.Scheduling) *compute.Instance {
	machineType := "e2-medium"
	tags := []string{"gdxsv-mcs", "http-server"}
	nic := &compute.NetworkInterface{
		Network:       "global/networks/default",
		AccessConfigs: []*compute.AccessConfig{{Type: "ONE_TO_ONE_NAT", Name: "External NAT", NetworkTier: "PREMIUM"}},
	}
	if p.Role == "relay" {
		machineType = "e2-micro"
		tags = []string{"gdxsv-relay"}
		nic = &compute.NetworkInterface{
			Network:           "global/networks/" + gceRelayNetwork,
			Subnetwork:        "regions/" + p.Region + "/subnetworks/" + gceRelayNetwork + "-" + p.Region,
			StackType:         "IPV4_IPV6",
			AccessConfigs:     []*compute.AccessConfig{{Type: "ONE_TO_ONE_NAT", Name: "External NAT", NetworkTier: "PREMIUM"}},
			Ipv6AccessConfigs: []*compute.AccessConfig{{Type: "DIRECT_IPV6", Name: "External IPv6", NetworkTier: "PREMIUM"}},
		}
	}
	return &compute.Instance{
		Name:        name,
		MachineType: "zones/" + zone + "/machineTypes/" + machineType,
		Tags:        &compute.Tags{Items: tags},
		Disks: []*compute.AttachedDisk{{
			Boot:             true,
			AutoDelete:       true,
			InitializeParams: &compute.AttachedDiskInitializeParams{SourceImage: gceImage},
		}},
		NetworkInterfaces: []*compute.NetworkInterface{nic},
		Scheduling:        scheduling,
		Metadata:          metadata,
		ServiceAccounts: []*compute.ServiceAccount{{
			Email:  gceServiceAccount(p.Project),
			Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
		}},
	}
}

// gceRemoveUnusedVMs deletes the gdxsv VMs that have been stopped for gceUnusedVMLifetime,
// at most once per gceCleanupInterval.
func gceRemoveUnusedVMs(ctx context.Context, api gceAPI) {
	gceAllocMu.Lock()
	if time.Since(gceCleanupTime) < gceCleanupInterval {
		gceAllocMu.Unlock()
		return
	}
	gceCleanupTime = time.Now()
	gceAllocMu.Unlock()

	for _, prefix := range []string{"gdxsv-mcs-", "gdxsv-relay-"} {
		vms, err := api.listInstances(ctx, prefix)
		if err != nil {
			logger.Warn("gce cleanup: list", zap.Error(err))
			return
		}
		for _, vm := range vms {
			if vm.Status != "TERMINATED" || vm.LastStartTimestamp == "" {
				continue
			}
			last, err := time.Parse(time.RFC3339, vm.LastStartTimestamp)
			if err != nil || time.Since(last) < gceUnusedVMLifetime {
				continue
			}
			zone := path.Base(vm.Zone)
			logger.Info("gce cleanup: deleting unused vm", zap.String("name", vm.Name), zap.String("zone", zone))
			if err := api.deleteInstance(ctx, zone, vm.Name); err != nil {
				logger.Warn("gce cleanup: delete", zap.String("name", vm.Name), zap.Error(err))
			}
		}
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// gceStartupScript is the VM's startup script: it downloads the gdxsv release and runs
// the role, shutting the VM down when the process ends.
func gceStartupScript(p gceAllocParams) string {
	script := gceMcsStartupScript
	if p.Role == "relay" {
		script = gceRelayStartupScript
	}
	return strings.NewReplacer(
		"__VERSION__", gceVersion,
		"__LOBBY_PUBLIC_ADDR__", p.LobbyPublicAddr,
		"__RELAY_SECRET__", p.RelaySecret,
	).Replace(script)
}

const gceMcsStartupScript = `#!/bin/bash
echo "startup-script"

snap install jq

if grep -xqFe 'ubuntu ALL=NOPASSWD: /sbin/shutdown' /etc/sudoers; then
  echo 'ubuntu ALL=NOPASSWD: /sbin/shutdown' >> /etc/sudoers
fi

mkdir -p /etc/google-fluentd/config.d
cat << 'EOF' > /etc/google-fluentd/config.d/gdxsv.conf
<source>
  @type tail
  format json
  path /var/log/gdxsv-mcs.log
  pos_file /var/lib/google-fluentd/pos/gdxsv.pos
  read_from_head true
  tag gdxsv-mcs
</source>
EOF


cat << 'EOF' > /home/ubuntu/launch-mcs.sh
#!/bin/bash -eux

function finish {
  echo "mcs finished" | logger
  sleep 1
  sudo /sbin/shutdown now
}
trap finish EXIT

readonly VERSION=__VERSION__

if [[ -z $VERSION || $VERSION == "latest" ]]; then
  readonly TAG_NAME=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.tag_name')
  readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.assets[].browser_download_url')
else
  readonly TAG_NAME=$VERSION
  readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/tags/$TAG_NAME | jq -r '.assets[].browser_download_url')
fi

if [[ ! -d $TAG_NAME/bin ]]; then
  echo "Downloading $TAG_NAME"
  mkdir -p "$TAG_NAME"
  pushd "$TAG_NAME"
    wget "$DOWNLOAD_URL"
    tar xzvf bin.tgz && rm bin.tgz
  popd
fi

export GDXSV_GCP_PROJECT_ID="" # automatically detected on GCE
export GDXSV_GCP_KEY_PATH="" # automatically detected on GCE
export GDXSV_LOBBY_PUBLIC_ADDR=__LOBBY_PUBLIC_ADDR__
export GDXSV_BATTLE_ADDR=:9877
export GDXSV_BATTLE_REGION=$(basename $(curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/zone))
export GDXSV_BATTLE_PUBLIC_ADDR=$(curl -s https://ipinfo.io/ip):9877
export GDXSV_BATTLE_LOG_PATH=/home/ubuntu/battlelog
mkdir -p $GDXSV_BATTLE_LOG_PATH

"$TAG_NAME"/bin/gdxsv -prodlog -cprof=1 mcs >> /var/log/gdxsv-mcs.log 2>&1
EOF

cat << 'EOF' > /home/ubuntu/upload-battlelog.sh
while :; do
  sleep 1m
  gsutil mv -z pb /home/ubuntu/battlelog/* gs://gdxsv/battlelog/
done
EOF

touch /var/log/gdxsv-mcs.log
truncate -s0 /var/log/gdxsv-mcs.log
chown ubuntu:ubuntu /var/log/gdxsv-mcs.log

chmod +x /home/ubuntu/launch-mcs.sh
chmod +x /home/ubuntu/upload-battlelog.sh

# systemctl restart google-fluentd
su ubuntu -c 'cd /home/ubuntu && nohup ./launch-mcs.sh &'
su ubuntu -c 'cd /home/ubuntu && nohup ./upload-battlelog.sh &'
echo "startup-script done"
`

const gceRelayStartupScript = `#!/bin/bash
echo "startup-script"

snap install jq

cat << 'EOF' > /home/ubuntu/launch-relay.sh
#!/bin/bash -eux

# The lobby stops the relay once no match needs it; stop the VM with it.
function finish {
  echo "relay finished" | logger
  sleep 1
  sudo /sbin/shutdown now
}
trap finish EXIT

readonly VERSION=__VERSION__

if [[ -z $VERSION || $VERSION == "latest" ]]; then
  readonly TAG_NAME=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.tag_name')
  readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.assets[].browser_download_url')
else
  readonly TAG_NAME=$VERSION
  readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/tags/$TAG_NAME | jq -r '.assets[].browser_download_url')
fi

if [[ ! -d $TAG_NAME/bin ]]; then
  echo "Downloading $TAG_NAME"
  mkdir -p "$TAG_NAME"
  pushd "$TAG_NAME"
    wget "$DOWNLOAD_URL"
    tar xzvf bin.tgz && rm bin.tgz
  popd
fi

readonly METADATA=http://metadata.google.internal/computeMetadata/v1/instance
readonly ZONE=$(basename $(curl -s -H "Metadata-Flavor: Google" $METADATA/zone))
export GDXSV_LOBBY_PUBLIC_ADDR=__LOBBY_PUBLIC_ADDR__
export GDXSV_RELAY_ADDR=:9879
export GDXSV_RELAY_SECRET=__RELAY_SECRET__
export GDXSV_RELAY_REGION=${ZONE%-*}
export GDXSV_RELAY_PUBLIC_ADDR=$(curl -s -H "Metadata-Flavor: Google" $METADATA/network-interfaces/0/access-configs/0/external-ip):9879
# The VM's external IPv6 address is on its interface. Skip temporary and deprecated ones.
readonly IPV6=$(ip -6 -o addr show scope global | grep -v -e temporary -e deprecated | awk '{print $4}' | cut -d/ -f1 | head -n1)
if [[ -n $IPV6 ]]; then
  export GDXSV_RELAY_PUBLIC_ADDR6="[$IPV6]:9879"
fi

"$TAG_NAME"/bin/gdxsv -prodlog relay >> /var/log/gdxsv-relay.log 2>&1
EOF

touch /var/log/gdxsv-relay.log
truncate -s0 /var/log/gdxsv-relay.log
chown ubuntu:ubuntu /var/log/gdxsv-relay.log

chmod +x /home/ubuntu/launch-relay.sh

su ubuntu -c 'cd /home/ubuntu && nohup ./launch-relay.sh &'
echo "startup-script done"
`
