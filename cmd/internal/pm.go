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

package internal

import (
	"context"

	parametermanager "cloud.google.com/go/parametermanager/apiv1"
	parametermanagerpb "cloud.google.com/go/parametermanager/apiv1/parametermanagerpb"
)

// FetchToolsFromParameterManager retrieves the parameter version payload from Google Cloud Parameter Manager.
func FetchToolsFromParameterManager(ctx context.Context, versionName string) ([]byte, error) {
	client, err := parametermanager.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	req := &parametermanagerpb.RenderParameterVersionRequest{
		Name: versionName,
	}
	resp, err := client.RenderParameterVersion(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetPayload().GetData(), nil
}
