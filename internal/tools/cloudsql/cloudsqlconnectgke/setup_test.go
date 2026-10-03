// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudsqlconnectgke

import (
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

func TestRegionOf(t *testing.T) {
	for in, want := range map[string]string{
		"us-central1-a":           "us-central1",
		"us-central1":             "us-central1",
		"northamerica-northeast1": "northamerica-northeast1",
		"europe-west4-b":          "europe-west4",
	} {
		if got := regionOf(in); got != want {
			t.Errorf("regionOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func gkeTestParams() setupParams {
	return setupParams{
		SQL: &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{Name: "db", Project: "data", Region: "us-central1"}, PSCAllowedConsumers: []string{"other"}},
		Cluster: &clusterInfo{
			Name: "prod", Location: "us-central1-a", Project: "app", VPCNetwork: "shared",
			NetworkURI: "projects/host/global/networks/shared", Subnetwork: "gke-subnet",
			WorkloadIdentity: true, VPCNative: true,
		},
		Identity:  newGKEIdentity("app", "", ""),
		Method:    cloudsqlconnect.MethodAuthProxy,
		Path:      pathPrivateIP,
		Manifests: environmentConfig{EnvironmentConfig: cloudsqlconnect.EnvironmentConfig{EnvironmentVariables: map[string]string{"DB_NAME": "orders", "DB_PORT": "5432"}}},
	}
}

func stepTitles(steps []cloudsqlconnect.SetupStep) []string {
	titles := make([]string, len(steps))
	for i, s := range steps {
		titles[i] = s.Title
	}
	return titles
}

func findStep(steps []cloudsqlconnect.SetupStep, prefix string) *cloudsqlconnect.SetupStep {
	for i := range steps {
		if strings.HasPrefix(steps[i].Title, prefix) {
			return &steps[i]
		}
	}
	return nil
}

func TestGenerateGKESetupSteps(t *testing.T) {
	tcs := []struct {
		desc       string
		mutate     func(*setupParams)
		wantTitles []string
		noTitles   []string
		wantInCmds []string
		notInCmds  []string
	}{
		{
			desc:       "happy path skips cluster changes and namespace creation",
			mutate:     func(*setupParams) {},
			noTitles:   []string{"Enable Workload Identity", "Switch node pools", "Create the namespace", "Resolve the connectivity blocker", "Create a PSC endpoint", "Give private nodes"},
			wantInCmds: []string{"--project=data", "serviceAccount:app.svc.id.goog[default/cloudsql-ksa]", "--from-literal=DB_NAME='orders'", "-c cloud-sql-proxy"},
		},
		{
			desc: "WI off, legacy pools and custom namespace",
			mutate: func(p *setupParams) {
				p.Cluster.WorkloadIdentity = false
				p.Cluster.NodePoolsWithoutGKEMetadata = []string{"a", "b"}
				p.Identity = newGKEIdentity("app", "shop", "orders")
			},
			wantTitles: []string{"Enable Workload Identity", "Switch node pools", "Create the namespace"},
			wantInCmds: []string{"--workload-pool=app.svc.id.goog", "node-pools update a ", "node-pools update b ", "kubectl create namespace shop"},
		},
		{
			desc:      "autopilot never asks to enable WI",
			mutate:    func(p *setupParams) { p.Cluster.WorkloadIdentity, p.Cluster.Autopilot = false, true },
			noTitles:  []string{"Enable Workload Identity"},
			notInCmds: []string{"--workload-pool"},
		},
		{
			desc:       "no path adds a blocker step",
			mutate:     func(p *setupParams) { p.Path = pathNone },
			wantTitles: []string{"Resolve the connectivity blocker"},
			wantInCmds: []string{"--assign-ip", "--network=projects/host/global/networks/shared"},
		},
		{
			desc:       "PSC keeps existing allowed projects and uses the host project",
			mutate:     func(p *setupParams) { p.Path = pathPSC },
			wantTitles: []string{"Allow the cluster's project", "Create a PSC endpoint", "Resolve the instance's DNS name"},
			wantInCmds: []string{"--allowed-psc-projects=other,app", "addresses create cloudsql-psc-ip --project=host --region=us-central1 --subnet=gke-subnet", "SERVICE_ATTACHMENT_URI", "INSTANCE_DNS_NAME"},
		},
		{
			desc: "PSC already allowed and cross-region uses a subnet placeholder",
			mutate: func(p *setupParams) {
				p.Path = pathPSC
				p.SQL.PSCAllowedConsumers = []string{"app"}
				p.Cluster.Location = "europe-west4"
			},
			noTitles:   []string{"Allow the cluster's project"},
			wantInCmds: []string{"--subnet=SUBNET_IN_US_CENTRAL1"},
		},
		{
			desc:       "public IP from private nodes needs Cloud NAT in the host project",
			mutate:     func(p *setupParams) { p.Path, p.Cluster.PrivateCluster = pathPublicIP, true },
			wantTitles: []string{"Give private nodes internet egress"},
			wantInCmds: []string{"routers create cloudsql-nat-router --project=host --network=shared --region=us-central1"},
		},
		{
			desc:      "connector has no proxy log step",
			mutate:    func(p *setupParams) { p.Method = cloudsqlconnect.MethodConnector },
			notInCmds: []string{"cloud-sql-proxy"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			p := gkeTestParams()
			tc.mutate(&p)
			steps := generateGKESetupSteps(p)
			var cmds []string
			for i, s := range steps {
				if s.Order != i+1 {
					t.Errorf("step %q has Order %d, want %d", s.Title, s.Order, i+1)
				}
				for _, bad := range []string{"|", "$(", "<<", "`"} {
					if strings.Contains(s.Command, bad) {
						t.Errorf("step %q command contains shell operator %q: %s", s.Title, bad, s.Command)
					}
				}
				cmds = append(cmds, s.Command)
			}
			all := strings.Join(cmds, "\n")
			for _, want := range tc.wantTitles {
				if findStep(steps, want) == nil {
					t.Errorf("missing step %q in %v", want, stepTitles(steps))
				}
			}
			for _, no := range tc.noTitles {
				if findStep(steps, no) != nil {
					t.Errorf("unexpected step %q in %v", no, stepTitles(steps))
				}
			}
			for _, want := range tc.wantInCmds {
				if !strings.Contains(all, want) {
					t.Errorf("commands missing %q:\n%s", want, all)
				}
			}
			for _, no := range tc.notInCmds {
				if strings.Contains(all, no) {
					t.Errorf("commands unexpectedly contain %q:\n%s", no, all)
				}
			}
		})
	}
}

func TestGKETroubleshooting(t *testing.T) {
	hasSymptom := func(tips []troubleshootingTip, sub string) bool {
		for _, tip := range tips {
			if strings.Contains(tip.Symptom, sub) {
				return true
			}
		}
		return false
	}
	p := gkeTestParams()
	classic := gkeTroubleshooting(p)
	if !hasSymptom(classic, "connection refused") || !hasSymptom(classic, "never completes") || hasSymptom(classic, "stuck in Init") {
		t.Errorf("classic sidecar tips wrong: %+v", classic)
	}
	p.NativeSidecar = true
	native := gkeTroubleshooting(p)
	if !hasSymptom(native, "stuck in Init") || hasSymptom(native, "never completes") {
		t.Errorf("native sidecar tips wrong: %+v", native)
	}
	p.Method = cloudsqlconnect.MethodConnector
	if hasSymptom(gkeTroubleshooting(p), "stuck in Init") {
		t.Error("connector should not get sidecar tips")
	}
}

func TestSummarizeGKE(t *testing.T) {
	rec := cloudsqlconnect.ConnectionRecommendation{Name: "Cloud SQL Auth Proxy sidecar"}
	p := gkeTestParams()

	got := summarizeGKE(p, &cloudsqlconnect.ValidationResult{Valid: true}, rec, 9)
	for _, want := range []string{"Ready to configure", "private IP", "9 setup steps", "Ask the user"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "Cluster changes needed") {
		t.Errorf("no cluster changes expected: %s", got)
	}

	p.Cluster.WorkloadIdentity = false
	p.Cluster.NodePoolsWithoutGKEMetadata = []string{"a"}
	got = summarizeGKE(p, &cloudsqlconnect.ValidationResult{Valid: true}, rec, 11)
	if !strings.Contains(got, "enable Workload Identity; move 1 node pool(s)") {
		t.Errorf("pending changes missing: %s", got)
	}

	p.Path = pathNone
	got = summarizeGKE(p, &cloudsqlconnect.ValidationResult{Valid: false, Issues: []string{"No route."}}, rec, 3)
	if !strings.HasPrefix(got, "Blocked:") || !strings.Contains(got, "No route.") {
		t.Errorf("blocked summary wrong: %s", got)
	}
}
