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
	"regexp"
)

// Caller-supplied parameters end up interpolated into shell commands, JDBC
// URLs, Python/JS string literals, and the GCE AggregatedList "Filter"
// expression. We validate them against the upstream GCP naming rules so
// metacharacters can't break out of those contexts.
//
// References:
//   - GCE instance + zone names:  https://cloud.google.com/compute/docs/naming-resources
//   - GCP project IDs:            https://cloud.google.com/resource-manager/docs/creating-managing-projects
//   - Cloud SQL instance IDs:     https://cloud.google.com/sql/docs/postgres/instance-settings
var (
	projectIDRe        = regexp.MustCompile(`^[a-z][-a-z0-9]{4,28}[a-z0-9]$`)
	gcpRegionRe        = regexp.MustCompile(`^[a-z]+-[a-z0-9-]+$`)
	cloudSQLInstanceRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,97}[a-z0-9]$`)
	gceResourceRe      = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`)
	databaseNameRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,62}$`)
)

// ValidateInstanceConnectionName splits and validates project, region, and
// instance ID per the GCP naming rules. Use this in place of plain
// ParseConnectionName when the parts will flow into generated code or shell.
func ValidateInstanceConnectionName(connName string) (project, region, instance string, err error) {
	project, region, instance, err = ParseConnectionName(connName)
	if err != nil {
		return "", "", "", err
	}
	if !projectIDRe.MatchString(project) {
		return "", "", "", fmt.Errorf("invalid project ID %q: must match %s", project, projectIDRe)
	}
	if !gcpRegionRe.MatchString(region) {
		return "", "", "", fmt.Errorf("invalid region %q: must match %s", region, gcpRegionRe)
	}
	if !cloudSQLInstanceRe.MatchString(instance) {
		return "", "", "", fmt.Errorf("invalid Cloud SQL instance ID %q: must match %s", instance, cloudSQLInstanceRe)
	}
	return project, region, instance, nil
}

// ValidateGCEResourceName checks a VM name or zone name against the standard
// GCE resource-name rule (lowercase, digits, hyphen; must start with a letter
// and not end with a hyphen, max 63 chars).
func ValidateGCEResourceName(name, kind string) error {
	if !gceResourceRe.MatchString(name) {
		return fmt.Errorf("invalid %s %q: must match %s", kind, name, gceResourceRe)
	}
	return nil
}

// ValidateDatabaseName accepts the conservative subset of database identifier
// characters that's safe in DSNs and code-snippet templates across Postgres,
// MySQL and SQL Server. It deliberately rejects quotes, semicolons, and
// whitespace even when the engine itself would accept them.
func ValidateDatabaseName(name string) error {
	if !databaseNameRe.MatchString(name) {
		return fmt.Errorf("invalid database_name %q: must match %s", name, databaseNameRe)
	}
	return nil
}
