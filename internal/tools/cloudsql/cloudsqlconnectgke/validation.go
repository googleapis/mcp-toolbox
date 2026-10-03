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
	"fmt"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

// validateGKEConnection checks whether pods in a GKE cluster can reach
// and authenticate to a Cloud SQL instance. It is Valid unless no network
// path exists at all; fixable gaps (Workload Identity off, node pools on
// the wrong metadata mode, missing Cloud NAT) are reported as warnings
// with matching setup steps rather than as failures.
func validateGKEConnection(sqlInfo *instanceInfo, gkeInfo *clusterInfo) *cloudsqlconnect.ValidationResult {
	result := &cloudsqlconnect.ValidationResult{
		Valid:  true,
		Checks: []cloudsqlconnect.ValidationCheck{},
	}
	add := func(name, status, msg string) {
		result.Checks = append(result.Checks, cloudsqlconnect.ValidationCheck{Name: name, Status: status, Message: msg})
	}

	// Check 1: Cluster health. A cluster mid-upgrade still works but
	// may reject the update commands in the setup steps.
	switch gkeInfo.Status {
	case "", "RUNNING":
	case "RECONCILING", "PROVISIONING":
		add("Cluster Status", "warn", fmt.Sprintf("Cluster is %s; cluster update commands may fail until it is RUNNING", gkeInfo.Status))
	default:
		add("Cluster Status", "fail", fmt.Sprintf("Cluster status is %s", gkeInfo.Status))
		result.Issues = append(result.Issues, fmt.Sprintf("Cluster %s is %s, not RUNNING.", gkeInfo.Name, gkeInfo.Status))
		result.Valid = false
	}

	// Check 2: Instance IP posture.
	var exposure []string
	if sqlInfo.PrivateIPEnabled {
		exposure = append(exposure, "private IP "+sqlInfo.PrivateIPAddress)
	}
	if sqlInfo.PublicIPEnabled {
		exposure = append(exposure, "public IP "+sqlInfo.PublicIPAddress)
	}
	if sqlInfo.PSCEnabled {
		exposure = append(exposure, "Private Service Connect")
	}
	if len(exposure) == 0 {
		add("Cloud SQL Connectivity", "fail", "Instance has no private IP, public IP, or Private Service Connect enabled")
	} else {
		add("Cloud SQL Connectivity", "info", "Instance exposes: "+strings.Join(exposure, ", "))
	}

	// Check 3: Private IP reachability, which needs the same VPC (Shared
	// VPC counts) AND a VPC-native cluster.
	if sqlInfo.PrivateIPEnabled {
		sameNet := sameNetwork(sqlInfo.VPCNetwork, gkeNetwork(gkeInfo))
		switch {
		case !sameNet:
			add("VPC Network Alignment", "warn", fmt.Sprintf("Different VPCs - cluster: %s, Cloud SQL: %s. Private services access peering is not transitive, so pods cannot use the private IP", gkeNetwork(gkeInfo), sqlInfo.VPCNetwork))
		case !gkeInfo.VPCNative:
			add("VPC-Native Cluster", "warn", "Same VPC, but the cluster is routes-based; pod IPs are not exported over the Cloud SQL peering, so pods cannot use the private IP")
		default:
			add("Private IP Reachability", "pass", fmt.Sprintf("Cluster is VPC-native and shares VPC %s with the instance", cloudsqlconnect.ExtractNetworkName(sqlInfo.VPCNetwork)))
		}
	}

	// Check 4: The resolved path, and what it still needs.
	path := resolveNetworkPath(sqlInfo, gkeInfo)
	switch path {
	case pathPrivateIP:
		add("Network Path", "pass", "Pods will connect over the private IP")
	case pathPublicIP:
		if gkeInfo.PrivateCluster {
			add("Network Path", "warn", "Pods will connect over the public IP, but nodes are private: Cloud NAT is required for egress")
			result.Recommendations = append(result.Recommendations, "Make sure Cloud NAT is configured for the cluster's network and region")
		} else {
			add("Network Path", "pass", "Pods will connect over the public IP; the Auth Proxy encrypts traffic and authorizes via IAM, so no authorized networks are needed")
		}
		if sqlInfo.PrivateIPEnabled {
			result.Recommendations = append(result.Recommendations, "For private connectivity, run the cluster in the instance's VPC (VPC-native) or enable Private Service Connect")
		}
	case pathPSC:
		add("Network Path", "warn", "Pods will connect over Private Service Connect; an endpoint and DNS record are required in the cluster's VPC")
		if !containsString(sqlInfo.PSCAllowedConsumers, gkeInfo.Project) {
			add("PSC Allowed Projects", "warn", fmt.Sprintf("Project %s is not in the instance's allowed PSC consumer projects", gkeInfo.Project))
		}
	case pathNone:
		add("Network Path", "fail", "No network path from the cluster to the instance")
		result.Issues = append(result.Issues, noPathReason(sqlInfo, gkeInfo))
		result.Valid = false
	}

	// Check 5: Workload Identity.
	switch {
	case gkeInfo.Autopilot:
		add("Workload Identity", "pass", "Autopilot cluster - Workload Identity is always enabled")
	case gkeInfo.WorkloadIdentity:
		add("Workload Identity", "pass", fmt.Sprintf("Workload Identity enabled (pool %s)", gkeInfo.WorkloadPool))
	default:
		add("Workload Identity", "warn", "Workload Identity is not enabled; pods would authenticate as the node's service account")
		result.Recommendations = append(result.Recommendations, "Enable Workload Identity for keyless, per-workload authentication to Cloud SQL")
	}

	// Check 6: Node pools that bypass Workload Identity.
	if len(gkeInfo.NodePoolsWithoutGKEMetadata) > 0 {
		add("Node Pool Metadata", "warn", fmt.Sprintf("Node pools %v are not using GKE_METADATA; pods there will not get Workload Identity credentials", gkeInfo.NodePoolsWithoutGKEMetadata))
	}

	// Check 7: Database authentication mode.
	if sqlInfo.IAMDatabaseAuthEnabled {
		add("IAM Database Authentication", "info", "IAM database authentication is enabled; you can optionally log in as the Google service account (proxy flag --auto-iam-authn) instead of a password")
	}
	if sqlInfo.RequireSSL || (sqlInfo.SSLMode != "" && sqlInfo.SSLMode != "ALLOW_UNENCRYPTED_AND_ENCRYPTED") {
		add("SSL Enforcement", "info", "Instance enforces SSL; the Auth Proxy and connectors handle this automatically, direct connections must configure TLS")
	}

	return result
}

// noPathReason explains, in one sentence, why no network path exists.
func noPathReason(sqlInfo *instanceInfo, gkeInfo *clusterInfo) string {
	if !sqlInfo.PrivateIPEnabled && !sqlInfo.PublicIPEnabled && !sqlInfo.PSCEnabled {
		return "The instance has no IP connectivity configured."
	}
	if !sameNetwork(sqlInfo.VPCNetwork, gkeNetwork(gkeInfo)) {
		return fmt.Sprintf("The instance only has a private IP in %s, and the cluster runs in a different VPC (%s).", cloudsqlconnect.ExtractNetworkName(sqlInfo.VPCNetwork), gkeInfo.VPCNetwork)
	}
	return "The instance only has a private IP, and the cluster is routes-based (not VPC-native), so pods cannot reach it."
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ValidateCloudRunConnection validates network connectivity between Cloud SQL and Cloud Run.
