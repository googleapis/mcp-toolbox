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

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/container/v1"
)

func TestExtractClusterInfo(t *testing.T) {
	tcs := []struct {
		desc    string
		cluster *container.Cluster
		want    *clusterInfo
	}{
		{
			desc: "standard cluster with WI and one legacy node pool",
			cluster: &container.Cluster{
				Name:                 "prod",
				Location:             "us-central1",
				Network:              "shared",
				Subnetwork:           "gke-subnet",
				Status:               "RUNNING",
				CurrentMasterVersion: "1.30.5-gke.1014001",
				NetworkConfig:        &container.NetworkConfig{Network: "projects/host/global/networks/shared"},
				PrivateClusterConfig: &container.PrivateClusterConfig{EnablePrivateNodes: true},
				IpAllocationPolicy:   &container.IPAllocationPolicy{UseIpAliases: true},
				WorkloadIdentityConfig: &container.WorkloadIdentityConfig{
					WorkloadPool: "app.svc.id.goog",
				},
				NodePools: []*container.NodePool{
					{Name: "good", Config: &container.NodeConfig{WorkloadMetadataConfig: &container.WorkloadMetadataConfig{Mode: "GKE_METADATA"}}},
					{Name: "legacy", Config: &container.NodeConfig{WorkloadMetadataConfig: &container.WorkloadMetadataConfig{Mode: "GCE_METADATA"}}},
					{Name: "unset", Config: &container.NodeConfig{}},
				},
			},
			want: &clusterInfo{
				Name:                        "prod",
				Location:                    "us-central1",
				Project:                     "app",
				VPCNetwork:                  "shared",
				NetworkURI:                  "projects/host/global/networks/shared",
				Subnetwork:                  "gke-subnet",
				WorkloadIdentity:            true,
				WorkloadPool:                "app.svc.id.goog",
				PrivateCluster:              true,
				VPCNative:                   true,
				Status:                      "RUNNING",
				MasterVersion:               "1.30.5-gke.1014001",
				NodePoolsWithoutGKEMetadata: []string{"legacy", "unset"},
			},
		},
		{
			desc: "autopilot skips node pool checks; private nodes from network config",
			cluster: &container.Cluster{
				Name:                   "ap",
				Location:               "us-east1",
				Autopilot:              &container.Autopilot{Enabled: true},
				NetworkConfig:          &container.NetworkConfig{Network: "projects/app/global/networks/default", DefaultEnablePrivateNodes: true},
				WorkloadIdentityConfig: &container.WorkloadIdentityConfig{WorkloadPool: "app.svc.id.goog"},
				NodePools:              []*container.NodePool{{Name: "managed"}},
			},
			want: &clusterInfo{
				Name:             "ap",
				Location:         "us-east1",
				Project:          "app",
				VPCNetwork:       "default",
				NetworkURI:       "projects/app/global/networks/default",
				WorkloadIdentity: true,
				WorkloadPool:     "app.svc.id.goog",
				PrivateCluster:   true,
				Autopilot:        true,
			},
		},
		{
			desc: "WI off lists every standard pool",
			cluster: &container.Cluster{
				Name:      "old",
				NodePools: []*container.NodePool{{Name: "default-pool"}},
			},
			want: &clusterInfo{
				Name:                        "old",
				Project:                     "app",
				NodePoolsWithoutGKEMetadata: []string{"default-pool"},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := extractClusterInfo(tc.cluster, "app")
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("extractClusterInfo mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSameNetwork(t *testing.T) {
	tcs := []struct {
		sql, cluster string
		want         bool
	}{
		{"projects/p/global/networks/default", "projects/p/global/networks/default", true},
		{"projects/p/global/networks/default", "default", true},
		{"projects/p/global/networks/default", "projects/other/global/networks/default", false},
		{"projects/p/global/networks/default", "projects/p/global/networks/prod", false},
		{"projects/123456/global/networks/default", "projects/p/global/networks/default", true},
		{"projects/123/global/networks/default", "projects/456/global/networks/default", false},
		{"https://www.googleapis.com/compute/v1/projects/p/global/networks/n", "projects/p/global/networks/n", true},
		{"", "default", false},
	}
	for _, tc := range tcs {
		if got := sameNetwork(tc.sql, tc.cluster); got != tc.want {
			t.Errorf("sameNetwork(%q, %q) = %v, want %v", tc.sql, tc.cluster, got, tc.want)
		}
	}
}

func TestSupportsNativeSidecar(t *testing.T) {
	tcs := map[string]bool{
		"1.30.5-gke.1014001": true,
		"1.29.0-gke.1":       true,
		"1.28.15-gke.100":    false,
		"v1.31.1":            true,
		"2.0.0":              true,
		"":                   false,
		"garbage":            false,
	}
	for v, want := range tcs {
		if got := supportsNativeSidecar(v); got != want {
			t.Errorf("supportsNativeSidecar(%q) = %v, want %v", v, got, want)
		}
	}
}
