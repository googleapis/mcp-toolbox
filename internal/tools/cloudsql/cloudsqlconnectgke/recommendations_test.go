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

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

func TestGetGKERecommendations(t *testing.T) {
	const net = "projects/my-project/global/networks/default"
	tcs := []struct {
		desc              string
		sqlInfo           *instanceInfo
		gkeInfo           *clusterInfo
		wantName          string
		wantAlternatives  []cloudsqlconnect.ConnectionMethod
		wantPrimaryReqHas string
	}{
		{
			desc:              "same VPC, VPC-native: private IP proxy plus connector and direct IP",
			sqlInfo:           &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{DatabaseType: cloudsqlconnect.PostgreSQL, PrivateIPEnabled: true, VPCNetwork: net}},
			gkeInfo:           &clusterInfo{WorkloadIdentity: true, VPCNative: true, NetworkURI: net},
			wantName:          "Cloud SQL Auth Proxy sidecar (Private IP)",
			wantAlternatives:  []cloudsqlconnect.ConnectionMethod{cloudsqlconnect.MethodConnector, cloudsqlconnect.MethodDirectPrivateIP},
			wantPrimaryReqHas: "--private-ip",
		},
		{
			desc:             "routes-based cluster cannot use private IP, falls back to public",
			sqlInfo:          &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{DatabaseType: cloudsqlconnect.MySQL, PrivateIPEnabled: true, PublicIPEnabled: true, VPCNetwork: net}},
			gkeInfo:          &clusterInfo{WorkloadIdentity: true, VPCNative: false, NetworkURI: net},
			wantName:         "Cloud SQL Auth Proxy sidecar (Public IP)",
			wantAlternatives: []cloudsqlconnect.ConnectionMethod{cloudsqlconnect.MethodConnector},
		},
		{
			desc:              "private nodes on public path need Cloud NAT",
			sqlInfo:           &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{DatabaseType: cloudsqlconnect.PostgreSQL, PublicIPEnabled: true}},
			gkeInfo:           &clusterInfo{WorkloadIdentity: true, VPCNative: true, PrivateCluster: true, NetworkURI: net},
			wantName:          "Cloud SQL Auth Proxy sidecar (Public IP)",
			wantAlternatives:  []cloudsqlconnect.ConnectionMethod{cloudsqlconnect.MethodConnector},
			wantPrimaryReqHas: "Cloud NAT",
		},
		{
			desc:              "PSC-only instance",
			sqlInfo:           &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{DatabaseType: cloudsqlconnect.PostgreSQL}, PSCEnabled: true},
			gkeInfo:           &clusterInfo{Autopilot: true, VPCNative: true, NetworkURI: net},
			wantName:          "Cloud SQL Auth Proxy sidecar (Private Service Connect)",
			wantAlternatives:  []cloudsqlconnect.ConnectionMethod{cloudsqlconnect.MethodConnector},
			wantPrimaryReqHas: "--psc",
		},
		{
			desc:             "SQL Server gets no connector alternative",
			sqlInfo:          &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{DatabaseType: cloudsqlconnect.SQLServer, PrivateIPEnabled: true, VPCNetwork: net}},
			gkeInfo:          &clusterInfo{WorkloadIdentity: true, VPCNative: true, NetworkURI: net},
			wantName:         "Cloud SQL Auth Proxy sidecar (Private IP)",
			wantAlternatives: []cloudsqlconnect.ConnectionMethod{cloudsqlconnect.MethodDirectPrivateIP},
		},
		{
			desc:             "different VPC, private only: blocked",
			sqlInfo:          &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{DatabaseType: cloudsqlconnect.PostgreSQL, PrivateIPEnabled: true, VPCNetwork: net}},
			gkeInfo:          &clusterInfo{WorkloadIdentity: true, VPCNative: true, NetworkURI: "projects/other/global/networks/default"},
			wantName:         "Cloud SQL Auth Proxy sidecar (no network path)",
			wantAlternatives: []cloudsqlconnect.ConnectionMethod{cloudsqlconnect.MethodConnector},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			primary, alternatives := getGKERecommendations(tc.sqlInfo, tc.gkeInfo)
			if primary.Method != cloudsqlconnect.MethodAuthProxy {
				t.Errorf("primary method = %v, want %v", primary.Method, cloudsqlconnect.MethodAuthProxy)
			}
			if primary.Name != tc.wantName {
				t.Errorf("primary name = %q, want %q", primary.Name, tc.wantName)
			}
			var got []cloudsqlconnect.ConnectionMethod
			for _, a := range alternatives {
				got = append(got, a.Method)
			}
			if diff := cmp.Diff(tc.wantAlternatives, got); diff != "" {
				t.Errorf("alternatives mismatch (-want +got):\n%s", diff)
			}
			if tc.wantPrimaryReqHas != "" && !strings.Contains(strings.Join(primary.Requirements, "|"), tc.wantPrimaryReqHas) {
				t.Errorf("primary requirements %v missing %q", primary.Requirements, tc.wantPrimaryReqHas)
			}
		})
	}
}
