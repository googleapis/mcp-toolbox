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
	"regexp"
)

// Caller-supplied GKE identifiers end up in gcloud/kubectl commands,
// Kubernetes manifests and IAM member strings, so they are checked against
// the upstream naming rules before any API call.
//
// References:
//   - GCP project IDs:         https://cloud.google.com/resource-manager/docs/creating-managing-projects
//   - GKE cluster names:       https://cloud.google.com/kubernetes-engine/docs/reference/rest/v1/projects.locations.clusters
//   - Kubernetes object names: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
var (
	projectIDRe      = regexp.MustCompile(`^[a-z][-a-z0-9]{4,28}[a-z0-9]$`)
	gkeClusterNameRe = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$`)
	gcpLocationRe    = regexp.MustCompile(`^[a-z]+(-[a-z]+)*[0-9]+(-[a-z])?$`)
	kubernetesNameRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
)

// validateProjectID checks a GCP project ID supplied directly by the caller.
func validateProjectID(project, kind string) error {
	if !projectIDRe.MatchString(project) {
		return fmt.Errorf("invalid %s %q: must match %s", kind, project, projectIDRe)
	}
	return nil
}

// validateClusterName checks a GKE cluster name (lowercase letters,
// digits and hyphens, starting with a letter, at most 40 characters).
func validateClusterName(name string) error {
	if !gkeClusterNameRe.MatchString(name) {
		return fmt.Errorf("invalid cluster_name %q: must match %s", name, gkeClusterNameRe)
	}
	return nil
}

// validateLocation accepts a region ("us-central1") or zone
// ("us-central1-a"), as GKE clusters can be regional or zonal.
func validateLocation(location, kind string) error {
	if !gcpLocationRe.MatchString(location) {
		return fmt.Errorf("invalid %s %q: expected a region like us-central1 or a zone like us-central1-a", kind, location)
	}
	return nil
}

// validateKubernetesName checks an RFC 1123 DNS label, the rule for
// namespaces and the subset of service account names that is safe in
// generated kubectl commands and IAM member strings.
func validateKubernetesName(name, kind string) error {
	if !kubernetesNameRe.MatchString(name) {
		return fmt.Errorf("invalid %s %q: must be a lowercase RFC 1123 label matching %s", kind, name, kubernetesNameRe)
	}
	return nil
}
