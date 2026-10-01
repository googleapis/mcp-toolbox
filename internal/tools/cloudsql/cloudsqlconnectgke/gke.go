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
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
	"golang.org/x/oauth2"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

// containerService is the ADC-backed GKE client, built once and shared.
// It follows the same rules as computeService in gce.go: initialized with
// context.Background() so a cancelled first caller cannot poison the
// cached error, and bypassed entirely when a caller token is supplied.
var (
	containerOnce    sync.Once
	containerService *container.Service
	containerErr     error
)

// getContainerService returns a GKE (Kubernetes Engine) API client.
//
// When accessToken is non-empty the client is scoped to that caller
// token, so cluster reads are authorized as the caller (matching the
// cloud-sql-admin source's useClientOAuth behavior). When empty, the
// process-wide client backed by Application Default Credentials is
// returned. The tools only issue read calls (clusters.get / list); the
// GKE API has no read-only OAuth scope, so cloud-platform is requested.
func getContainerService(ctx context.Context, accessToken string) (*container.Service, error) {
	if accessToken != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
		svc, err := container.NewService(ctx, option.WithTokenSource(ts))
		if err != nil {
			return nil, fmt.Errorf("failed to build token-scoped GKE client: %w", err)
		}
		return svc, nil
	}
	containerOnce.Do(func() {
		containerService, containerErr = container.NewService(context.Background(), option.WithScopes(container.CloudPlatformScope))
	})
	return containerService, containerErr
}

// errClusterLookup marks cluster-resolution failures the caller can fix
// by changing its input (not found, ambiguous name). Tools surface these
// as agent errors rather than server errors.
var errClusterLookup = errors.New("cluster lookup failed")

// clusterResourceName builds the GKE API resource name for a cluster.
func clusterResourceName(project, location, name string) string {
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, name)
}

// findCluster resolves a cluster by name across every location in the
// project using the "-" location wildcard. GKE does not support a
// server-side name filter on clusters.list, so the match is client-side;
// the response is a single page.
func findCluster(ctx context.Context, svc *container.Service, project, name string) (*container.Cluster, error) {
	resp, err := svc.Projects.Locations.Clusters.List(fmt.Sprintf("projects/%s/locations/-", project)).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list GKE clusters in project %q: %w", project, err)
	}

	var matches []*container.Cluster
	for _, c := range resp.Clusters {
		if c.Name == name {
			matches = append(matches, c)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		msg := fmt.Sprintf("GKE cluster %q not found in project %q", name, project)
		if len(resp.MissingZones) > 0 {
			msg += fmt.Sprintf(" (some locations could not be queried: %v; pass cluster_location to look it up directly)", resp.MissingZones)
		}
		if available := clusterNames(resp.Clusters); len(available) > 0 {
			msg += fmt.Sprintf("; clusters in this project: %v", available)
		} else {
			msg += "; if the cluster lives in a different project than the Cloud SQL instance, pass cluster_project"
		}
		return nil, fmt.Errorf("%w: %s", errClusterLookup, msg)
	default:
		locations := make([]string, 0, len(matches))
		for _, c := range matches {
			locations = append(locations, c.Location)
		}
		return nil, fmt.Errorf("%w: multiple GKE clusters named %q found in locations %v - please specify cluster_location", errClusterLookup, name, locations)
	}
}

func clusterNames(clusters []*container.Cluster) []string {
	names := make([]string, 0, len(clusters))
	for _, c := range clusters {
		names = append(names, fmt.Sprintf("%s (%s)", c.Name, c.Location))
	}
	sort.Strings(names)
	return names
}

// extractClusterInfo lifts the fields the connect tool reasons over out of a
// GKE Cluster. project is the project the cluster was read from (the
// cluster resource itself does not carry it).
func extractClusterInfo(c *container.Cluster, project string) *clusterInfo {
	info := &clusterInfo{
		Name:          c.Name,
		Location:      c.Location,
		Project:       project,
		VPCNetwork:    c.Network,
		Subnetwork:    c.Subnetwork,
		Status:        c.Status,
		MasterVersion: c.CurrentMasterVersion,
	}
	if c.NetworkConfig != nil {
		info.NetworkURI = c.NetworkConfig.Network
		if info.VPCNetwork == "" {
			info.VPCNetwork = cloudsqlconnect.ExtractNetworkName(c.NetworkConfig.Network)
		}
		// Newer clusters express private nodes here instead of (or in
		// addition to) PrivateClusterConfig.
		info.PrivateCluster = c.NetworkConfig.DefaultEnablePrivateNodes
	}
	if c.PrivateClusterConfig != nil && c.PrivateClusterConfig.EnablePrivateNodes {
		info.PrivateCluster = true
	}
	if c.IpAllocationPolicy != nil {
		info.VPCNative = c.IpAllocationPolicy.UseIpAliases
	}
	if c.Autopilot != nil {
		info.Autopilot = c.Autopilot.Enabled
	}
	if c.WorkloadIdentityConfig != nil && c.WorkloadIdentityConfig.WorkloadPool != "" {
		info.WorkloadIdentity = true
		info.WorkloadPool = c.WorkloadIdentityConfig.WorkloadPool
	}
	// Autopilot always runs Workload Identity and manages node pools, so
	// the per-pool metadata check only applies to Standard clusters. It
	// runs even when Workload Identity is off, because every pool will
	// then need updating after the cluster-level switch is flipped.
	if !info.Autopilot {
		for _, np := range c.NodePools {
			if np.Config == nil || np.Config.WorkloadMetadataConfig == nil || np.Config.WorkloadMetadataConfig.Mode != "GKE_METADATA" {
				info.NodePoolsWithoutGKEMetadata = append(info.NodePoolsWithoutGKEMetadata, np.Name)
			}
		}
	}
	return info
}

// sameNetwork reports whether a Cloud SQL private network and a GKE
// cluster network are the same VPC. Both are compared as
// project + network when both paths carry a project, which avoids the
// false positive of two unrelated projects that each have a VPC named
// "default". If either side lacks a project, or one side uses a project
// number while the other uses an ID, it falls back to the network name.
func sameNetwork(sqlNetwork, clusterNetwork string) bool {
	sqlProject, sqlName := splitNetworkPath(sqlNetwork)
	clusterProject, clusterName := splitNetworkPath(clusterNetwork)
	if sqlName == "" || sqlName != clusterName {
		return false
	}
	if sqlProject == "" || clusterProject == "" || sqlProject == clusterProject {
		return true
	}
	// Project numbers and IDs can't be compared without a lookup; trust
	// the name match rather than reporting a misleading mismatch.
	return isAllDigits(sqlProject) != isAllDigits(clusterProject)
}

// splitNetworkPath accepts either a bare network name or a resource path
// containing "projects/<p>/.../networks/<n>" (optionally as a full URL).
func splitNetworkPath(path string) (project, name string) {
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "projects":
			project = parts[i+1]
		case "networks":
			name = parts[i+1]
		}
	}
	if name == "" && !strings.Contains(path, "/") {
		name = path
	}
	return project, name
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// supportsNativeSidecar reports whether the control plane version can run
// Kubernetes native sidecars (init containers with restartPolicy: Always),
// which GKE enables from 1.29. Native sidecars start before and stop
// after the app container, so the app never races the proxy at startup
// and Jobs complete instead of hanging on a still-running proxy.
// Unparseable versions return false so callers fall back to the
// universally supported classic sidecar.
func supportsNativeSidecar(masterVersion string) bool {
	major, minor, ok := parseKubeMinor(masterVersion)
	if !ok {
		return false
	}
	return major > 1 || (major == 1 && minor >= 29)
}

// parseKubeMinor extracts major.minor from GKE versions such as
// "1.30.5-gke.1014001".
func parseKubeMinor(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}
