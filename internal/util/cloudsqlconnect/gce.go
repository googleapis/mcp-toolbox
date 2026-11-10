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
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

// ParseConnectionName splits a Cloud SQL instance connection name
// ("project:region:instance") into its three components, rejecting any input
// that doesn't have exactly three non-empty parts.
func ParseConnectionName(connName string) (project, region, instance string, err error) {
	parts := strings.Split(connName, ":")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid connection name format %q: expected project:region:instance", connName)
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid connection name %q: project, region, and instance must all be non-empty", connName)
	}
	return parts[0], parts[1], parts[2], nil
}

// ExtractNetworkName pulls the trailing element off a fully-qualified network
// or subnetwork resource path. Idempotent for inputs that are already a name.
func ExtractNetworkName(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return path
	}
	return path[idx+1:]
}

// IsSameVPC reports whether the Cloud SQL VPC and the GCE VM VPC resolve to
// the same network name.
func IsSameVPC(sqlVPC, vmVPC string) bool {
	sqlNet := ExtractNetworkName(sqlVPC)
	return sqlNet != "" && sqlNet == vmVPC
}

// computeService is lazily initialized on first use and shared across
// invocations that rely on Application Default Credentials:
// *compute.Service is goroutine-safe and rebuilding it per call would
// re-pay token-source + service-discovery costs.
var (
	computeOnce    sync.Once
	computeService *compute.Service
	computeErr     error
)

// GetComputeService returns a Compute Engine read-only client.
//
// When accessToken is non-empty the returned client is scoped to that
// caller-supplied OAuth token, so IAM decisions on the Compute API are
// evaluated as the caller (matching how cloudsqladmin.Source.GetService
// treats its accessToken). When accessToken is empty the function
// returns the process-wide client backed by Application Default
// Credentials, built once on first call.
//
// The ADC-backed initializer runs with context.Background() on purpose:
// a request-scoped ctx cached inside sync.Once would poison every
// subsequent invocation if the first caller cancelled. Callers still
// propagate their request ctx to individual API calls via
// Instances.Get(...).Context(ctx).Do().
func GetComputeService(ctx context.Context, accessToken string) (*compute.Service, error) {
	if accessToken != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
		svc, err := compute.NewService(ctx,
			option.WithTokenSource(ts),
			option.WithScopes(compute.ComputeReadonlyScope),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build token-scoped Compute Engine client: %w", err)
		}
		return svc, nil
	}
	computeOnce.Do(func() {
		computeService, computeErr = compute.NewService(context.Background(), option.WithScopes(compute.ComputeReadonlyScope))
	})
	return computeService, computeErr
}

// ExtractSQLInfo lifts the fields the connect tools need out of a
// Cloud SQL Admin DatabaseInstance.
func ExtractSQLInfo(inst *sqladmin.DatabaseInstance) *CloudSQLInstanceInfo {
	info := &CloudSQLInstanceInfo{
		Name:            inst.Name,
		Project:         inst.Project,
		Region:          inst.Region,
		ConnectionName:  inst.ConnectionName,
		DatabaseVersion: inst.DatabaseVersion,
		DatabaseType:    ParseDatabaseType(inst.DatabaseVersion),
	}
	for _, ip := range inst.IpAddresses {
		switch ip.Type {
		case "PRIMARY":
			info.PublicIPAddress = ip.IpAddress
			info.PublicIPEnabled = true
		case "PRIVATE":
			info.PrivateIPAddress = ip.IpAddress
			info.PrivateIPEnabled = true
		}
	}
	if inst.Settings != nil && inst.Settings.IpConfiguration != nil {
		info.VPCNetwork = inst.Settings.IpConfiguration.PrivateNetwork
		info.RequireSSL = inst.Settings.IpConfiguration.RequireSsl
	}
	return info
}

// ExtractVMInfo lifts the fields the connect tools need out of a
// Compute Engine instance.
func ExtractVMInfo(inst *compute.Instance, zone string) *GCEInstanceInfo {
	info := &GCEInstanceInfo{Name: inst.Name, Zone: zone}
	if len(inst.NetworkInterfaces) > 0 {
		ni := inst.NetworkInterfaces[0]
		info.InternalIP = ni.NetworkIP
		info.VPCNetwork = ExtractNetworkName(ni.Network)
		info.Subnetwork = ExtractNetworkName(ni.Subnetwork)
		for _, ac := range ni.AccessConfigs {
			if ac.NatIP != "" {
				info.ExternalIP = ac.NatIP
				info.HasExternalIP = true
				break
			}
		}
	}
	if len(inst.ServiceAccounts) > 0 {
		info.ServiceAccount = inst.ServiceAccounts[0].Email
	}
	return info
}

// FindVM resolves a VM by name across all zones in a project. It uses a
// server-side name filter so the API short-circuits per-zone scans, and
// stops paging once a second match is seen (so we don't read pages we
// don't need just to error out).
func FindVM(ctx context.Context, service *compute.Service, project, vmName string) (*compute.Instance, string, error) {
	var foundInstances []*compute.Instance
	var foundZones []string

	const stopPaging stringErr = "stop-paging"

	// vmName is whitelist-validated by ValidateGCEResourceName upstream of this
	// call, so it cannot contain quote/space/special chars; %q wraps it in
	// quotes for GCE's filter language regardless.
	req := service.Instances.AggregatedList(project).Filter(fmt.Sprintf("name eq %q", vmName))
	err := req.Pages(ctx, func(page *compute.InstanceAggregatedList) error {
		for zone, list := range page.Items {
			for _, instance := range list.Instances {
				if instance.Name != vmName {
					continue
				}
				foundInstances = append(foundInstances, instance)
				foundZones = append(foundZones, ExtractNetworkName(zone))
				if len(foundInstances) > 1 {
					return stopPaging
				}
			}
		}
		return nil
	})
	if err != nil && err != stopPaging {
		return nil, "", fmt.Errorf("failed to search for VM: %w", err)
	}

	switch len(foundInstances) {
	case 0:
		return nil, "", fmt.Errorf("VM %q not found in project %q", vmName, project)
	case 1:
		return foundInstances[0], foundZones[0], nil
	default:
		return nil, "", fmt.Errorf("multiple VMs named %q found in zones: %v - please specify vm_zone parameter", vmName, foundZones)
	}
}

// stringErr signals early termination from Pages without conflating with
// real API errors. Unexported: only FindVM's Pages callback uses it.
type stringErr string

func (e stringErr) Error() string { return string(e) }
