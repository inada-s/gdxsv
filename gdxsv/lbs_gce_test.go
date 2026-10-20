package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/compute/v1"
)

// gceFake is an in-memory Compute Engine: instances by zone/name, and a log of the calls.
type gceFake struct {
	instances []*compute.Instance
	calls     []string
	insertErr map[string]error // zone -> error
}

func (f *gceFake) find(zone, name string) *compute.Instance {
	for _, i := range f.instances {
		if i.Name == name && strings.HasSuffix(i.Zone, "/"+zone) {
			return i
		}
	}
	return nil
}

func (f *gceFake) listInstances(_ context.Context, prefix string) ([]*compute.Instance, error) {
	f.calls = append(f.calls, "list "+prefix)
	var out []*compute.Instance
	for _, i := range f.instances {
		if strings.HasPrefix(i.Name, prefix) {
			out = append(out, i)
		}
	}
	return out, nil
}

func (f *gceFake) getInstance(_ context.Context, zone, name string) (*compute.Instance, error) {
	if i := f.find(zone, name); i != nil {
		return i, nil
	}
	return nil, errors.New("not found")
}

func (f *gceFake) setMetadata(_ context.Context, zone, name string, m *compute.Metadata) error {
	f.calls = append(f.calls, "metadata "+zone+"/"+name)
	i := f.find(zone, name)
	if i == nil {
		return errors.New("not found")
	}
	i.Metadata = m
	return nil
}

func (f *gceFake) setScheduling(_ context.Context, zone, name string, s *compute.Scheduling) error {
	f.calls = append(f.calls, "scheduling "+zone+"/"+name)
	f.find(zone, name).Scheduling = s
	return nil
}

func (f *gceFake) startInstance(_ context.Context, zone, name string) error {
	f.calls = append(f.calls, "start "+zone+"/"+name)
	f.find(zone, name).Status = "RUNNING"
	return nil
}

func (f *gceFake) insertInstance(_ context.Context, zone string, inst *compute.Instance) error {
	f.calls = append(f.calls, "insert "+zone+"/"+inst.Name)
	if err := f.insertErr[zone]; err != nil {
		return err
	}
	inst.Zone = "https://www.googleapis.com/compute/v1/projects/p/zones/" + zone
	inst.Status = "RUNNING"
	f.instances = append(f.instances, inst)
	return nil
}

func (f *gceFake) deleteInstance(_ context.Context, zone, name string) error {
	f.calls = append(f.calls, "delete "+zone+"/"+name)
	for n, i := range f.instances {
		if i == f.find(zone, name) {
			f.instances = append(f.instances[:n], f.instances[n+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func fakeInstance(zone, name, status string) *compute.Instance {
	return &compute.Instance{
		Name:     name,
		Zone:     "https://www.googleapis.com/compute/v1/projects/p/zones/" + zone,
		Status:   status,
		Metadata: &compute.Metadata{Fingerprint: "fp"},
	}
}

func metadataValue(m *compute.Metadata, key string) string {
	for _, it := range m.Items {
		if it.Key == key && it.Value != nil {
			return *it.Value
		}
	}
	return ""
}

func resetGceState() {
	gceAllocMu.Lock()
	gceCleanupTime = time.Time{}
	gceAllocTime = map[string]time.Time{}
	gceAllocMu.Unlock()
}

var testAllocParams = gceAllocParams{
	Project:         "proj",
	Region:          "asia-east2",
	Role:            "mcs",
	LobbyPublicAddr: "lbs.gdxsv.net:9876",
	RelaySecret:     "0123456789abcdef_relay",
}

func TestGceAllocReusesRunningVM(t *testing.T) {
	resetGceState()
	f := &gceFake{instances: []*compute.Instance{fakeInstance("asia-east2-b", "gdxsv-mcs-asia-east2-latest", "RUNNING")}}
	vm, err := gceAlloc(context.Background(), f, testAllocParams)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Name != "gdxsv-mcs-asia-east2-latest" || strings.Join(f.calls, ",") != "list gdxsv-mcs-,list gdxsv-relay-,list gdxsv-mcs-asia-east2-latest" {
		t.Errorf("vm %s calls %v", vm.Name, f.calls)
	}
}

func TestGceAllocStartsStoppedVM(t *testing.T) {
	resetGceState()
	f := &gceFake{instances: []*compute.Instance{
		fakeInstance("asia-east2-a", "gdxsv-mcs-asia-east2-latest-old", "TERMINATED"), // another name: left alone
		fakeInstance("asia-east2-c", "gdxsv-mcs-asia-east2-latest", "TERMINATED"),
	}}
	vm, err := gceAlloc(context.Background(), f, testAllocParams)
	if err != nil {
		t.Fatal(err)
	}
	want := "metadata asia-east2-c/gdxsv-mcs-asia-east2-latest,scheduling asia-east2-c/gdxsv-mcs-asia-east2-latest,start asia-east2-c/gdxsv-mcs-asia-east2-latest"
	if got := strings.Join(f.calls[3:], ","); got != want || vm.Status != "RUNNING" {
		t.Errorf("calls %v status %s", f.calls, vm.Status)
	}
	if vm.Metadata.Fingerprint != "fp" || !strings.Contains(metadataValue(vm.Metadata, "startup-script"), "GDXSV_LOBBY_PUBLIC_ADDR=lbs.gdxsv.net:9876") {
		t.Errorf("metadata %+v", vm.Metadata)
	}
	if vm.Scheduling.OnHostMaintenance != "MIGRATE" || vm.Scheduling.Preemptible || *vm.Scheduling.AutomaticRestart {
		t.Errorf("scheduling %+v", vm.Scheduling)
	}
}

func TestGceAllocCreatesVM(t *testing.T) {
	resetGceState()
	f := &gceFake{insertErr: map[string]error{"asia-east2-a": errors.New("ZONE_RESOURCE_POOL_EXHAUSTED")}}
	vm, err := gceAlloc(context.Background(), f, testAllocParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(vm.Zone, "/asia-east2-b") || vm.MachineType != "zones/asia-east2-b/machineTypes/e2-medium" {
		t.Errorf("zone %s type %s", vm.Zone, vm.MachineType)
	}
	if strings.Join(vm.Tags.Items, ",") != "gdxsv-mcs,http-server" || vm.NetworkInterfaces[0].Network != "global/networks/default" {
		t.Errorf("tags %v nic %+v", vm.Tags.Items, vm.NetworkInterfaces[0])
	}
	if vm.ServiceAccounts[0].Email != "gdxsv-service@proj.iam.gserviceaccount.com" || !vm.Disks[0].Boot || vm.Disks[0].InitializeParams.SourceImage != gceImage {
		t.Errorf("sa %v disks %+v", vm.ServiceAccounts, vm.Disks[0])
	}
	script := metadataValue(vm.Metadata, "startup-script")
	if !strings.Contains(script, "readonly VERSION=latest") || !strings.Contains(script, "gdxsv -prodlog -cprof=1 mcs") || strings.Contains(script, "__") {
		t.Errorf("script %s", script)
	}
}

func TestGceAllocRelay(t *testing.T) {
	resetGceState()
	p := testAllocParams
	p.Role = "relay"
	f := &gceFake{}
	vm, err := gceAlloc(context.Background(), f, p)
	if err != nil {
		t.Fatal(err)
	}
	nic := vm.NetworkInterfaces[0]
	if vm.Name != "gdxsv-relay-asia-east2-latest" || vm.MachineType != "zones/asia-east2-a/machineTypes/e2-micro" || vm.Tags.Items[0] != "gdxsv-relay" {
		t.Errorf("vm %s %s %v", vm.Name, vm.MachineType, vm.Tags.Items)
	}
	if nic.Network != "global/networks/gdxsv-relay" || nic.Subnetwork != "regions/asia-east2/subnetworks/gdxsv-relay-asia-east2" || nic.StackType != "IPV4_IPV6" || len(nic.Ipv6AccessConfigs) != 1 {
		t.Errorf("nic %+v", nic)
	}
	script := metadataValue(vm.Metadata, "startup-script")
	for _, want := range []string{"GDXSV_RELAY_SECRET=0123456789abcdef_relay", "GDXSV_LOBBY_PUBLIC_ADDR=lbs.gdxsv.net:9876", "GDXSV_RELAY_REGION=${ZONE%-*}", "gdxsv -prodlog relay"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q", want)
		}
	}

	p.RelaySecret = "short"
	if _, err := gceAlloc(context.Background(), f, p); err == nil {
		t.Error("relay with a bad secret allocated")
	}
}

func TestGceAllocRejectsBadParams(t *testing.T) {
	resetGceState()
	f := &gceFake{}
	for _, p := range []gceAllocParams{
		{Project: "p", Region: "mars-1", Role: "mcs", LobbyPublicAddr: "a:1"},
		{Project: "p", Region: "asia-east2", Role: "lbs", LobbyPublicAddr: "a:1"},
		{Project: "p", Region: "asia-east2", Role: "mcs"},
	} {
		if _, err := gceAlloc(context.Background(), f, p); err == nil {
			t.Errorf("%+v allocated", p)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("calls %v", f.calls)
	}
}

func TestGceRemoveUnusedVMs(t *testing.T) {
	resetGceState()
	old := time.Now().Add(-6 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	f := &gceFake{instances: []*compute.Instance{
		fakeInstance("asia-east2-a", "gdxsv-mcs-asia-east2-latest", "TERMINATED"),
		fakeInstance("us-west1-a", "gdxsv-mcs-us-west1-latest", "TERMINATED"),
		fakeInstance("us-west1-b", "gdxsv-relay-us-west1-latest", "TERMINATED"),
		fakeInstance("us-east1-b", "gdxsv-mcs-us-east1-latest", "RUNNING"),
		fakeInstance("us-east1-b", "other-vm", "TERMINATED"),
	}}
	f.instances[0].LastStartTimestamp = recent
	f.instances[1].LastStartTimestamp = old
	f.instances[2].LastStartTimestamp = old
	f.instances[3].LastStartTimestamp = old
	f.instances[4].LastStartTimestamp = old

	gceRemoveUnusedVMs(context.Background(), f)
	var deleted []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "delete ") {
			deleted = append(deleted, c)
		}
	}
	if strings.Join(deleted, ",") != "delete us-west1-a/gdxsv-mcs-us-west1-latest,delete us-west1-b/gdxsv-relay-us-west1-latest" {
		t.Errorf("deleted %v", deleted)
	}

	// once per gceCleanupInterval
	f.calls = nil
	gceRemoveUnusedVMs(context.Background(), f)
	if len(f.calls) != 0 {
		t.Errorf("second cleanup made calls %v", f.calls)
	}
}
