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

package cloudsqlconnect

import (
	"fmt"
	"strings"
)

// ValidateGCEConnection validates network connectivity between Cloud SQL and GCE VM.
func ValidateGCEConnection(sqlInfo *CloudSQLInstanceInfo, vmInfo *GCEInstanceInfo) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Checks: []ValidationCheck{},
	}

	// Check 1: Private IP availability
	if sqlInfo.PrivateIPEnabled {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Cloud SQL Private IP",
			Status:  "pass",
			Message: fmt.Sprintf("Private IP enabled: %s", sqlInfo.PrivateIPAddress),
		})
	} else {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Cloud SQL Private IP",
			Status:  "warn",
			Message: "Private IP not enabled - direct VPC connection not possible",
		})
		result.Recommendations = append(result.Recommendations,
			"Consider enabling private IP for secure, low-latency connections")
	}

	// Check 2: Public IP availability
	if sqlInfo.PublicIPEnabled {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Cloud SQL Public IP",
			Status:  "info",
			Message: fmt.Sprintf("Public IP enabled: %s", sqlInfo.PublicIPAddress),
		})
	} else if !sqlInfo.PrivateIPEnabled {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Cloud SQL Public IP",
			Status:  "fail",
			Message: "No IP connectivity available - enable private or public IP",
		})
		result.Issues = append(result.Issues, "Cloud SQL instance has no IP addresses configured")
		result.Valid = false
	}

	// Check 3: VPC Network alignment
	if sqlInfo.PrivateIPEnabled && sqlInfo.VPCNetwork != "" && vmInfo.VPCNetwork != "" {
		sqlVPC := ExtractNetworkName(sqlInfo.VPCNetwork)
		vmVPC := vmInfo.VPCNetwork

		if sqlVPC == vmVPC {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "VPC Network Alignment",
				Status:  "pass",
				Message: fmt.Sprintf("Both resources in same VPC: %s", vmVPC),
			})
		} else {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "VPC Network Alignment",
				Status:  "warn",
				Message: fmt.Sprintf("Different VPCs - VM: %s, Cloud SQL: %s", vmVPC, sqlVPC),
			})
			result.Recommendations = append(result.Recommendations,
				"Use Cloud SQL Auth Proxy or set up VPC peering for private IP connectivity")
		}
	}

	// Check 4: VM service account
	if vmInfo.ServiceAccount != "" {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "VM Service Account",
			Status:  "info",
			Message: fmt.Sprintf("Service account: %s", vmInfo.ServiceAccount),
		})
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Ensure %s has roles/cloudsql.client", vmInfo.ServiceAccount))
	} else {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "VM Service Account",
			Status:  "warn",
			Message: "No service account detected on VM",
		})
	}

	return result
}

// ValidateGKEConnection validates network connectivity between Cloud SQL and GKE cluster.
func ValidateGKEConnection(sqlInfo *CloudSQLInstanceInfo, gkeInfo *GKEClusterInfo) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Checks: []ValidationCheck{},
	}

	// Check 1: Private IP availability
	if sqlInfo.PrivateIPEnabled {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Cloud SQL Private IP",
			Status:  "pass",
			Message: fmt.Sprintf("Private IP enabled: %s", sqlInfo.PrivateIPAddress),
		})
	} else {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Cloud SQL Private IP",
			Status:  "info",
			Message: "Private IP not enabled - will use Auth Proxy with public IP",
		})
	}

	// Check 2: Workload Identity
	if gkeInfo.WorkloadIdentity {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Workload Identity",
			Status:  "pass",
			Message: "Workload Identity enabled - recommended for secure authentication",
		})
	} else {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Workload Identity",
			Status:  "warn",
			Message: "Workload Identity not enabled",
		})
		result.Recommendations = append(result.Recommendations,
			"Enable Workload Identity for secure, keyless authentication to Cloud SQL")
	}

	// Check 3: VPC Network alignment
	if sqlInfo.PrivateIPEnabled && sqlInfo.VPCNetwork != "" && gkeInfo.VPCNetwork != "" {
		sqlVPC := ExtractNetworkName(sqlInfo.VPCNetwork)

		if sqlVPC == gkeInfo.VPCNetwork {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "VPC Network Alignment",
				Status:  "pass",
				Message: fmt.Sprintf("Both resources in same VPC: %s", gkeInfo.VPCNetwork),
			})
		} else {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "VPC Network Alignment",
				Status:  "warn",
				Message: fmt.Sprintf("Different VPCs - GKE: %s, Cloud SQL: %s", gkeInfo.VPCNetwork, sqlVPC),
			})
		}
	}

	// Check 4: VPC-native cluster
	if gkeInfo.VPCNative {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "VPC-Native Cluster",
			Status:  "pass",
			Message: "Cluster is VPC-native (IP aliasing enabled)",
		})
	} else {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "VPC-Native Cluster",
			Status:  "info",
			Message: "Cluster is not VPC-native - private IP access may be limited",
		})
	}

	// Check 5: Private cluster
	if gkeInfo.PrivateCluster {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Private Cluster",
			Status:  "info",
			Message: "Private cluster - nodes have no public IPs",
		})
	}

	return result
}

// ValidateCloudRunConnection validates network connectivity between Cloud SQL and Cloud Run.
func ValidateCloudRunConnection(sqlInfo *CloudSQLInstanceInfo, runInfo *CloudRunServiceInfo) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Checks: []ValidationCheck{},
	}

	// Check 1: Built-in Cloud SQL support
	result.Checks = append(result.Checks, ValidationCheck{
		Name:    "Cloud Run Cloud SQL Integration",
		Status:  "pass",
		Message: "Cloud Run has built-in Cloud SQL connection support via Unix socket",
	})

	// Check 2: Check if already connected
	for _, inst := range runInfo.CloudSQLInstances {
		if strings.Contains(inst, sqlInfo.ConnectionName) {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "Existing Connection",
				Status:  "info",
				Message: "Cloud SQL instance already configured on this service",
			})
		}
	}

	// Check 3: Service account
	if runInfo.ServiceAccount != "" {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Service Account",
			Status:  "info",
			Message: fmt.Sprintf("Service account: %s", runInfo.ServiceAccount),
		})
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Ensure %s has roles/cloudsql.client", runInfo.ServiceAccount))
	}

	// Check 4: VPC Connector (for private IP)
	if sqlInfo.PrivateIPEnabled {
		if runInfo.VPCConnector != "" {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "VPC Connector",
				Status:  "pass",
				Message: fmt.Sprintf("VPC Connector configured: %s", runInfo.VPCConnector),
			})
		} else if runInfo.DirectVPCEgress {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "Direct VPC Egress",
				Status:  "pass",
				Message: "Direct VPC Egress enabled",
			})
		} else {
			result.Checks = append(result.Checks, ValidationCheck{
				Name:    "VPC Access",
				Status:  "info",
				Message: "No VPC connector - will use Unix socket (recommended)",
			})
		}
	}

	return result
}

// ValidateLocalConnection validates requirements for local IDE connection.
func ValidateLocalConnection(sqlInfo *CloudSQLInstanceInfo) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Checks: []ValidationCheck{},
	}

	// Check 1: Public IP for Auth Proxy
	if sqlInfo.PublicIPEnabled {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Public IP Access",
			Status:  "pass",
			Message: fmt.Sprintf("Cloud SQL Auth Proxy can connect via public IP: %s", sqlInfo.PublicIPAddress),
		})
	} else if sqlInfo.PrivateIPEnabled {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "Public IP Access",
			Status:  "warn",
			Message: "No public IP - Auth Proxy requires VPN or Private Google Access",
		})
		result.Recommendations = append(result.Recommendations,
			"Enable public IP for easier local development, or configure VPN/Private Google Access")
	} else {
		result.Checks = append(result.Checks, ValidationCheck{
			Name:    "IP Access",
			Status:  "fail",
			Message: "No IP addresses configured on Cloud SQL instance",
		})
		result.Issues = append(result.Issues, "Enable public or private IP on Cloud SQL instance")
		result.Valid = false
	}

	// Check 2: Auth Proxy requirements
	result.Checks = append(result.Checks, ValidationCheck{
		Name:    "Auth Proxy Requirements",
		Status:  "info",
		Message: "Cloud SQL Auth Proxy or Connector library required for local development",
	})

	result.Recommendations = append(result.Recommendations,
		"Ensure Cloud SQL Admin API is enabled",
		"Configure Application Default Credentials: gcloud auth application-default login",
		"Ensure your user has roles/cloudsql.client",
	)

	return result
}
