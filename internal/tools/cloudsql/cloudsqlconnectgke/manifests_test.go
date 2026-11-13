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

func TestResolveNetworkPath(t *testing.T) {
	const net = "projects/p/global/networks/default"
	nativeSameVPC := &clusterInfo{VPCNative: true, NetworkURI: net}
	tcs := []struct {
		desc string
		sql  *instanceInfo
		gke  *clusterInfo
		want networkPath
	}{
		{"private preferred over public", &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PrivateIPEnabled: true, PublicIPEnabled: true, VPCNetwork: net}}, nativeSameVPC, pathPrivateIP},
		{"routes-based falls back to public", &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PrivateIPEnabled: true, PublicIPEnabled: true, VPCNetwork: net}}, &clusterInfo{NetworkURI: net}, pathPublicIP},
		{"public preferred over PSC", &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PublicIPEnabled: true}, PSCEnabled: true}, nativeSameVPC, pathPublicIP},
		{"PSC when it is the only option", &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{}, PSCEnabled: true}, nativeSameVPC, pathPSC},
		{"private in other VPC is no path", &instanceInfo{CloudSQLInstanceInfo: &cloudsqlconnect.CloudSQLInstanceInfo{PrivateIPEnabled: true, VPCNetwork: "projects/x/global/networks/default"}}, nativeSameVPC, pathNone},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := resolveNetworkPath(tc.sql, tc.gke); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSidecarYAML(t *testing.T) {
	base := manifestOptions{Method: cloudsqlconnect.MethodAuthProxy, ConnectionName: "p:r:i", Port: 5432}

	t.Run("private path native sidecar", func(t *testing.T) {
		opts := base
		opts.Path = pathPrivateIP
		opts.NativeSidecar = true
		y := sidecarYAML(opts)
		for _, want := range []string{authProxyImage, `"--private-ip"`, `"--port=5432"`, "restartPolicy: Always", "startupProbe", `"p:r:i"`, `"--exit-zero-on-sigterm"`} {
			if !strings.Contains(y, want) {
				t.Errorf("sidecar missing %q:\n%s", want, y)
			}
		}
		if strings.Contains(y, "auto-iam-authn") {
			t.Error("sidecar must not enable --auto-iam-authn by default")
		}
	})

	t.Run("public path classic sidecar", func(t *testing.T) {
		opts := base
		opts.Path = pathPublicIP
		y := sidecarYAML(opts)
		if strings.Contains(y, "--private-ip") || strings.Contains(y, "--psc") || strings.Contains(y, "restartPolicy") {
			t.Errorf("unexpected flags for public classic sidecar:\n%s", y)
		}
	})

	t.Run("psc path", func(t *testing.T) {
		opts := base
		opts.Path = pathPSC
		if y := sidecarYAML(opts); !strings.Contains(y, `"--psc"`) {
			t.Errorf("psc sidecar missing --psc:\n%s", y)
		}
	})
}

func TestDeploymentYAML(t *testing.T) {
	id := newGKEIdentity("app", "shop", "orders")
	opts := manifestOptions{Method: cloudsqlconnect.MethodAuthProxy, Path: pathPrivateIP, ConnectionName: "p:r:i", Port: 3306, DBName: "orders", Identity: id}

	t.Run("native sidecar goes in initContainers", func(t *testing.T) {
		opts := opts
		opts.NativeSidecar = true
		y := deploymentYAML(opts)
		if strings.Index(y, "initContainers:") > strings.Index(y, "containers:\n      - name: app") || !strings.Contains(y, "initContainers:") {
			t.Errorf("native sidecar should precede app containers:\n%s", y)
		}
		for _, want := range []string{"namespace: shop", "serviceAccountName: orders", "name: " + secretName, `value: "127.0.0.1"`, `value: "3306"`} {
			if !strings.Contains(y, want) {
				t.Errorf("deployment missing %q:\n%s", want, y)
			}
		}
	})

	t.Run("classic sidecar is a second container", func(t *testing.T) {
		y := deploymentYAML(opts)
		if strings.Contains(y, "initContainers") {
			t.Errorf("classic sidecar should not use initContainers:\n%s", y)
		}
		if strings.Index(y, "name: cloud-sql-proxy") < strings.Index(y, "name: app") {
			t.Errorf("classic sidecar should follow the app container:\n%s", y)
		}
	})

	t.Run("connector has no sidecar", func(t *testing.T) {
		opts := opts
		opts.Method = cloudsqlconnect.MethodConnector
		y := deploymentYAML(opts)
		if strings.Contains(y, "cloud-sql-proxy") || !strings.Contains(y, "INSTANCE_CONNECTION_NAME") {
			t.Errorf("unexpected connector deployment:\n%s", y)
		}
	})
}

func TestNewGKEIdentity(t *testing.T) {
	id := newGKEIdentity("app-proj", "", "")
	want := gkeIdentity{
		ClusterProject: "app-proj",
		Namespace:      defaultNamespace,
		KSA:            defaultKSA,
		GSAEmail:       "cloudsql-client@app-proj.iam.gserviceaccount.com",
	}
	if diff := cmp.Diff(want, id); diff != "" {
		t.Errorf("identity mismatch (-want +got):\n%s", diff)
	}
	if got := id.WorkloadIdentityMember(); got != "serviceAccount:app-proj.svc.id.goog[default/cloudsql-ksa]" {
		t.Errorf("member = %q", got)
	}
}
