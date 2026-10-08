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
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

func TestValidateGKEConnection(t *testing.T) {
	const net = "projects/my-project/global/networks/default"
	tcs := []struct {
		desc       string
		sqlInfo    *instanceInfo
		gkeInfo    *clusterInfo
		wantValid  bool
		wantChecks map[string]string // check name -> status
	}{
		{
			desc:      "private IP path with workload identity",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PrivateIPEnabled: true, PrivateIPAddress: "10.0.0.5", VPCNetwork: net}},
			gkeInfo:   &clusterInfo{WorkloadIdentity: true, WorkloadPool: "my-project.svc.id.goog", VPCNative: true, NetworkURI: net, Status: "RUNNING"},
			wantValid: true,
			wantChecks: map[string]string{
				"Private IP Reachability": "pass",
				"Network Path":            "pass",
				"Workload Identity":       "pass",
			},
		},
		{
			desc:      "workload identity off and node pools on wrong metadata are warnings, not failures",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PublicIPEnabled: true, PublicIPAddress: "35.1.2.3"}},
			gkeInfo:   &clusterInfo{NodePoolsWithoutGKEMetadata: []string{"default-pool"}, VPCNative: true},
			wantValid: true,
			wantChecks: map[string]string{
				"Workload Identity":  "warn",
				"Node Pool Metadata": "warn",
				"Network Path":       "pass",
			},
		},
		{
			desc:      "routes-based cluster in same VPC cannot use private IP",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PrivateIPEnabled: true, VPCNetwork: net}},
			gkeInfo:   &clusterInfo{Autopilot: true, VPCNative: false, NetworkURI: net},
			wantValid: false,
			wantChecks: map[string]string{
				"VPC-Native Cluster": "warn",
				"Network Path":       "fail",
				"Workload Identity":  "pass",
			},
		},
		{
			desc:      "same network name in different projects is not the same VPC",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PrivateIPEnabled: true, VPCNetwork: net}},
			gkeInfo:   &clusterInfo{WorkloadIdentity: true, VPCNative: true, NetworkURI: "projects/other-project/global/networks/default"},
			wantValid: false,
			wantChecks: map[string]string{
				"VPC Network Alignment": "warn",
				"Network Path":          "fail",
			},
		},
		{
			desc:      "private nodes on public path warn about Cloud NAT",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PublicIPEnabled: true}},
			gkeInfo:   &clusterInfo{WorkloadIdentity: true, PrivateCluster: true, VPCNative: true},
			wantValid: true,
			wantChecks: map[string]string{
				"Network Path": "warn",
			},
		},
		{
			desc:      "PSC path flags missing allowed project",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{}, PSCEnabled: true, PSCAllowedConsumers: []string{"sql-project"}},
			gkeInfo:   &clusterInfo{Project: "app-project", WorkloadIdentity: true, VPCNative: true},
			wantValid: true,
			wantChecks: map[string]string{
				"Network Path":         "warn",
				"PSC Allowed Projects": "warn",
			},
		},
		{
			desc:      "stopped cluster is invalid",
			sqlInfo:   &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PublicIPEnabled: true}},
			gkeInfo:   &clusterInfo{Name: "c", WorkloadIdentity: true, Status: "STOPPING"},
			wantValid: false,
			wantChecks: map[string]string{
				"Cluster Status": "fail",
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			result := validateGKEConnection(tc.sqlInfo, tc.gkeInfo)
			if result.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v (checks: %+v)", result.Valid, tc.wantValid, result.Checks)
			}
			got := map[string]string{}
			for _, c := range result.Checks {
				got[c.Name] = c.Status
			}
			for name, status := range tc.wantChecks {
				if got[name] != status {
					t.Errorf("check %q status = %q, want %q", name, got[name], status)
				}
			}
			if !result.Valid && len(result.Issues) == 0 {
				t.Error("invalid result should explain itself in Issues")
			}
		})
	}
}
