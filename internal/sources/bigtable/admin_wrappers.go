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

package bigtable

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/bigtable"
)

func (s *Source) GetInstance(ctx context.Context, instanceId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	instance, err := cs.instanceAdmin.InstanceInfo(ctx, instanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}
	return instance, nil
}

func (s *Source) CreateInstance(ctx context.Context, instanceId, displayName, clusterId, zone string, numNodes int32) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := &bigtable.InstanceConf{
		InstanceId:  instanceId,
		DisplayName: displayName,
		ClusterId:   clusterId,
		Zone:        zone,
		NumNodes:    numNodes,
	}
	err = cs.instanceAdmin.CreateInstance(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}
	return map[string]string{"status": "instance created successfully"}, nil
}

func (s *Source) UpdateInstance(ctx context.Context, instanceId, displayName string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := &bigtable.InstanceWithClustersConfig{
		InstanceID:  instanceId,
		DisplayName: displayName,
	}
	err = cs.instanceAdmin.UpdateInstanceWithClusters(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to update instance: %w", err)
	}
	return map[string]string{"status": "instance updated successfully"}, nil
}

func (s *Source) DeleteInstance(ctx context.Context, instanceId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.instanceAdmin.DeleteInstance(ctx, instanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to delete instance: %w", err)
	}
	return map[string]string{"status": "instance deleted successfully"}, nil
}

func (s *Source) ListInstances(ctx context.Context) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	instances, err := cs.instanceAdmin.Instances(ctx)
	if err != nil {
		var partialErr bigtable.ErrPartiallyUnavailable
		if errors.As(err, &partialErr) {
			return instances, nil
		}
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	return instances, nil
}

func (s *Source) GetCluster(ctx context.Context, instanceId, clusterId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	cluster, err := cs.instanceAdmin.GetCluster(ctx, instanceId, clusterId)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}
	return cluster, nil
}

func (s *Source) ListClusters(ctx context.Context, instanceId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	clusters, err := cs.instanceAdmin.Clusters(ctx, instanceId)
	if err != nil {
		var partialErr bigtable.ErrPartiallyUnavailable
		if errors.As(err, &partialErr) {
			return clusters, nil
		}
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	return clusters, nil
}

func (s *Source) CreateCluster(ctx context.Context, instanceId, clusterId, zone string, numNodes int32) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := &bigtable.ClusterConfig{
		InstanceID: instanceId,
		ClusterID:  clusterId,
		Zone:       zone,
		NumNodes:   numNodes,
	}
	err = cs.instanceAdmin.CreateCluster(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster: %w", err)
	}
	return map[string]string{"status": "cluster created successfully"}, nil
}

func (s *Source) UpdateCluster(ctx context.Context, instanceId, clusterId string, serveNodes int32) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.instanceAdmin.UpdateCluster(ctx, instanceId, clusterId, serveNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to update cluster: %w", err)
	}
	return map[string]string{"status": "cluster updated successfully"}, nil
}

func (s *Source) DeleteCluster(ctx context.Context, instanceId, clusterId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.instanceAdmin.DeleteCluster(ctx, instanceId, clusterId)
	if err != nil {
		return nil, fmt.Errorf("failed to delete cluster: %w", err)
	}
	return map[string]string{"status": "cluster deleted successfully"}, nil
}

func (s *Source) GetTable(ctx context.Context, tableId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	table, err := cs.admin.TableInfo(ctx, tableId)
	if err != nil {
		return nil, fmt.Errorf("failed to get table: %w", err)
	}
	return table, nil
}

func (s *Source) CreateTable(ctx context.Context, tableId, columnFamily string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.admin.CreateTable(ctx, tableId)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	if columnFamily != "" {
		if err := cs.admin.CreateColumnFamily(ctx, tableId, columnFamily); err != nil {
			return nil, fmt.Errorf("failed to create column family: %w", err)
		}
	}
	return map[string]string{"status": "table created successfully"}, nil
}

func (s *Source) DeleteTable(ctx context.Context, tableId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.admin.DeleteTable(ctx, tableId)
	if err != nil {
		return nil, fmt.Errorf("failed to delete table: %w", err)
	}
	return map[string]string{"status": "table deleted successfully"}, nil
}

func (s *Source) ListTables(ctx context.Context) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	tables, err := cs.admin.Tables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	return tables, nil
}

func (s *Source) UpdateTable(ctx context.Context, tableId string, disableChangeStream bool) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	if disableChangeStream {
		err = cs.admin.UpdateTableDisableChangeStream(ctx, tableId)
	} else {
		err = cs.admin.UpdateTableWithChangeStream(ctx, tableId, 24*time.Hour)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update table: %w", err)
	}
	return map[string]string{"status": "table updated successfully"}, nil
}

func (s *Source) GetLogicalView(ctx context.Context, instanceId, logicalViewId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	view, err := cs.instanceAdmin.LogicalViewInfo(ctx, instanceId, logicalViewId)
	if err != nil {
		return nil, fmt.Errorf("failed to get logical view: %w", err)
	}
	return view, nil
}

func (s *Source) ListLogicalViews(ctx context.Context, instanceId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	views, err := cs.instanceAdmin.LogicalViews(ctx, instanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to list logical views: %w", err)
	}
	return views, nil
}

func (s *Source) ListMaterializedViews(ctx context.Context, instanceId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	views, err := cs.instanceAdmin.MaterializedViews(ctx, instanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to list materialized views: %w", err)
	}
	return views, nil
}

func (s *Source) CreateLogicalView(ctx context.Context, instanceId, logicalViewId, query string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := &bigtable.LogicalViewInfo{
		LogicalViewID: logicalViewId,
		Query:         query,
	}
	err = cs.instanceAdmin.CreateLogicalView(ctx, instanceId, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create logical view: %w", err)
	}
	return map[string]string{"status": "logical view created successfully"}, nil
}

func (s *Source) UpdateLogicalView(ctx context.Context, instanceId, logicalViewId, query string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := bigtable.LogicalViewInfo{ // MUST be value per bigtable SDK
		LogicalViewID: logicalViewId,
		Query:         query,
	}
	err = cs.instanceAdmin.UpdateLogicalView(ctx, instanceId, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to update logical view: %w", err)
	}
	return map[string]string{"status": "logical view updated successfully"}, nil
}

func (s *Source) DeleteLogicalView(ctx context.Context, instanceId, logicalViewId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.instanceAdmin.DeleteLogicalView(ctx, instanceId, logicalViewId)
	if err != nil {
		return nil, fmt.Errorf("failed to delete logical view: %w", err)
	}
	return map[string]string{"status": "logical view deleted successfully"}, nil
}

func (s *Source) GetMaterializedView(ctx context.Context, instanceId, materializedViewId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	view, err := cs.instanceAdmin.MaterializedViewInfo(ctx, instanceId, materializedViewId)
	if err != nil {
		return nil, fmt.Errorf("failed to get materialized view: %w", err)
	}
	return view, nil
}

func (s *Source) CreateMaterializedView(ctx context.Context, instanceId, materializedViewId, query string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := &bigtable.MaterializedViewInfo{
		MaterializedViewID: materializedViewId,
		Query:              query,
	}
	err = cs.instanceAdmin.CreateMaterializedView(ctx, instanceId, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create materialized view: %w", err)
	}
	return map[string]string{"status": "materialized view created successfully"}, nil
}

func (s *Source) UpdateMaterializedView(ctx context.Context, instanceId, materializedViewId, query string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	conf := bigtable.MaterializedViewInfo{ // MUST be value per bigtable SDK
		MaterializedViewID: materializedViewId,
		Query:              query,
	}
	err = cs.instanceAdmin.UpdateMaterializedView(ctx, instanceId, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to update materialized view: %w", err)
	}
	return map[string]string{"status": "materialized view updated successfully"}, nil
}

func (s *Source) DeleteMaterializedView(ctx context.Context, instanceId, materializedViewId string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	err = cs.instanceAdmin.DeleteMaterializedView(ctx, instanceId, materializedViewId)
	if err != nil {
		return nil, fmt.Errorf("failed to delete materialized view: %w", err)
	}
	return map[string]string{"status": "materialized view deleted successfully"}, nil
}
