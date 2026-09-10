// Copyright 2025 Google LLC
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

package dataplex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	dataplex "cloud.google.com/go/dataplex/apiv1"
	dataplexpb "cloud.google.com/go/dataplex/apiv1/dataplexpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	storageapi "cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

var (
	DataplexSourceType                       = "dataplex"
	DataplexLookupContextToolType            = "dataplex-lookup-context"
	DataplexSearchEntriesToolType            = "dataplex-search-entries"
	DataplexLookupEntryToolType              = "dataplex-lookup-entry"
	DataplexSearchAspectTypesToolType        = "dataplex-search-aspect-types"
	DataplexSearchDataQualityScansToolType   = "dataplex-search-dq-scans"
	DataplexListDataProductsToolType         = "dataplex-list-data-products"
	DataplexGetDataProductToolType           = "dataplex-get-data-product"
	DataplexListDataAssetsToolType           = "dataplex-list-data-assets"
	DataplexGetDataAssetToolType             = "dataplex-get-data-asset"
	DataplexCreateDataProductToolType        = "dataplex-create-data-product"
	DataplexUpdateDataProductToolType        = "dataplex-update-data-product"
	DataplexCreateDataAssetToolType          = "dataplex-create-data-asset"
	DataplexUpdateDataAssetToolType          = "dataplex-update-data-asset"
	DataplexUpdateDataProductAspectsToolType = "dataplex-update-data-product-aspects"
	DataplexGenerateDataProfileToolType      = "dataplex-generate-data-profile"
	DataplexGetDataProfileToolType           = "dataplex-get-data-profile"
	DataplexGetOperationToolType             = "dataplex-get-operation"
	DataplexGetRunStatusToolType             = "dataplex-get-run-status"
	DataplexGenerateDataInsightsToolType     = "dataplex-generate-data-insights"
	DataplexGetDataInsightsToolType          = "dataplex-get-data-insights"
	DataplexDiscoverMetadataToolType         = "dataplex-discover-metadata"
	DataplexGetDiscoveryResultsToolType      = "dataplex-get-discovery-results"
	DataplexCheckDataQualityToolType         = "dataplex-check-data-quality"
	DataplexGetDataQualityResultsToolType    = "dataplex-get-data-quality-results"
	DataplexProject                          = os.Getenv("DATAPLEX_PROJECT")
)

func getDataplexVars(t *testing.T) map[string]any {
	switch "" {
	case DataplexProject:
		t.Fatal("'DATAPLEX_PROJECT' not set")
	}
	return map[string]any{
		"type":    DataplexSourceType,
		"project": DataplexProject,
	}
}

// Copied over from bigquery.go
func initBigQueryConnection(ctx context.Context, project string) (*bigqueryapi.Client, error) {
	cred, err := google.FindDefaultCredentials(ctx, bigqueryapi.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", bigqueryapi.Scope, err)
	}

	client, err := bigqueryapi.NewClient(ctx, project, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client for project %q: %w", project, err)
	}
	return client, nil
}

func initDataplexConnection(ctx context.Context) (*dataplex.CatalogClient, error) {
	cred, err := google.FindDefaultCredentials(ctx, sources.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials: %w", err)
	}

	client, err := dataplex.NewCatalogClient(ctx, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Dataplex client %w", err)
	}
	return client, nil
}

type pollableDeleteOperation interface {
	Poll(ctx context.Context, opts ...gax.CallOption) error
	Done() bool
}

// waitSlowlyForDeletion blocks and polls the deletion operation status using
// an exponential backoff, polling for the first time after 3s, doubling up to
// a maximum of 60s.
// We use our own function for polling LROs during the aspect type and data
// product cleanup since the default op.Wait(ctx) method polls too quickly,
// hitting rate limits (sending ~3 poll requests per call).
func waitSlowlyForDeletion(ctx context.Context, op pollableDeleteOperation) error {
	backoff := 3 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		if err := op.Poll(ctx); err != nil {
			return err
		}
		if op.Done() {
			return nil
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// cleanupOldAspectTypes Deletes AspectTypes older than the specified duration.
func cleanupOldAspectTypes(t *testing.T, ctx context.Context, client *dataplex.CatalogClient, oldThreshold time.Duration) {
	parent := fmt.Sprintf("projects/%s/locations/us", DataplexProject)
	olderThanTime := time.Now().Add(-oldThreshold)

	listReq := &dataplexpb.ListAspectTypesRequest{
		Parent:   parent,
		PageSize: 100,               // Fetch up to 100 items
		OrderBy:  "create_time asc", // Order by creation time
	}

	const maxDeletes = 8 // Explicitly limit the number of deletions
	it := client.ListAspectTypes(ctx, listReq)
	var aspectTypesToDelete []string
	for len(aspectTypesToDelete) < maxDeletes {
		aspectType, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Warning: Failed to list aspect types during cleanup: %v", err)
			return
		}
		// Perform time-based filtering in memory and restrict to test suite created resources
		if aspectType.CreateTime != nil {
			createTime := aspectType.CreateTime.AsTime()
			if createTime.Before(olderThanTime) && strings.HasPrefix(path.Base(aspectType.GetName()), "param-aspect-type-") {
				aspectTypesToDelete = append(aspectTypesToDelete, aspectType.GetName())
			}
		} else {
			t.Logf("Warning: AspectType %s has no CreateTime", aspectType.GetName())
		}
	}
	if len(aspectTypesToDelete) == 0 {
		t.Logf("cleanupOldAspectTypes: No aspect types found older than %s to delete.", oldThreshold.String())
		return
	}

	for _, aspectTypeName := range aspectTypesToDelete {
		deleteReq := &dataplexpb.DeleteAspectTypeRequest{Name: aspectTypeName}
		op, err := client.DeleteAspectType(ctx, deleteReq)
		if err != nil {
			t.Logf("Warning: Failed to delete aspect type %s: %v", aspectTypeName, err)
			continue // Skip to the next item if initiation fails
		}

		if err := waitSlowlyForDeletion(ctx, op); err != nil {
			t.Logf("Warning: Failed to delete aspect type %s, operation error: %v", aspectTypeName, err)
		} else {
			t.Logf("cleanupOldAspectTypes: Successfully deleted %s", aspectTypeName)
		}
	}
}

// cleanupOldDataProducts Deletes DataProducts and their DataAssets older than the specified duration.
func cleanupOldDataProducts(t *testing.T, ctx context.Context, client *dataplex.DataProductClient, oldThreshold time.Duration) {
	parent := fmt.Sprintf("projects/%s/locations/us", DataplexProject)
	olderThanTime := time.Now().Add(-oldThreshold)

	listReq := &dataplexpb.ListDataProductsRequest{
		Parent:   parent,
		PageSize: 200, // Max Data Products per project per region.
		Filter:   fmt.Sprintf("name:projects/%s/locations/us/dataProducts/param-data-product", DataplexProject),
	}

	const maxDeletes = 10
	it := client.ListDataProducts(ctx, listReq)
	var dataProductsToDelete []string
	for len(dataProductsToDelete) < maxDeletes {
		dp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Warning: Failed to list data products during cleanup: %v", err)
			return
		}
		if dp.CreateTime != nil && strings.HasPrefix(path.Base(dp.GetName()), "param-data-product-") {
			createTime := dp.CreateTime.AsTime()
			if createTime.Before(olderThanTime) {
				dataProductsToDelete = append(dataProductsToDelete, dp.GetName())
			}
		} else if dp.CreateTime == nil {
			t.Logf("Warning: DataProduct %s has no CreateTime", dp.GetName())
		}
	}
	if len(dataProductsToDelete) == 0 {
		t.Logf("cleanupOldDataProducts: No data products found older than %s to delete.", oldThreshold.String())
		return
	}

	for _, dpName := range dataProductsToDelete {
		// Must delete child data assets before we can delete the data product.
		assetIt := client.ListDataAssets(ctx, &dataplexpb.ListDataAssetsRequest{
			Parent:   dpName,
			PageSize: 50, // Max Data Assets per Data Product.
		})
		for {
			asset, err := assetIt.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Logf("Warning: Failed to list data assets for %s: %v", dpName, err)
				break
			}
			op, err := client.DeleteDataAsset(ctx, &dataplexpb.DeleteDataAssetRequest{Name: asset.GetName()})
			if err != nil {
				t.Logf("Warning: Failed to initiate DeleteDataAsset for %s: %v", asset.GetName(), err)
				continue
			}
			if err := waitSlowlyForDeletion(ctx, op); err != nil {
				t.Logf("Warning: Failed to wait for DeleteDataAsset %s: %v", asset.GetName(), err)
			} else {
				t.Logf("cleanupOldDataProducts: Successfully deleted asset %s", asset.GetName())
			}
		}

		op, err := client.DeleteDataProduct(ctx, &dataplexpb.DeleteDataProductRequest{Name: dpName})
		if err != nil {
			t.Logf("Warning: Failed to initiate DeleteDataProduct for %s: %v", dpName, err)
			continue
		}
		if err := waitSlowlyForDeletion(ctx, op); err != nil {
			t.Logf("Warning: Failed to wait for DeleteDataProduct %s: %v", dpName, err)
		} else {
			t.Logf("cleanupOldDataProducts: Successfully deleted data product %s", dpName)
		}
	}
}

// cleanupOldDataScans Deletes DataScans older than the specified duration.
func cleanupOldDataScans(t *testing.T, ctx context.Context, client *dataplex.DataScanClient, oldThreshold time.Duration) {
	parent := fmt.Sprintf("projects/%s/locations/us-central1", DataplexProject)
	olderThanTime := time.Now().Add(-oldThreshold)

	listReq := &dataplexpb.ListDataScansRequest{
		Parent: parent,
	}

	const maxDeletes = 50
	it := client.ListDataScans(ctx, listReq)
	var dataScansToDelete []string
	for len(dataScansToDelete) < maxDeletes {
		scan, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("Warning: Failed to list data scans during cleanup: %v", err)
			return
		}
		if scan.CreateTime != nil && strings.HasPrefix(path.Base(scan.GetName()), "param-data-scan-") {
			createTime := scan.CreateTime.AsTime()
			if createTime.Before(olderThanTime) {
				dataScansToDelete = append(dataScansToDelete, scan.GetName())
			}
		}
	}
	if len(dataScansToDelete) == 0 {
		t.Logf("cleanupOldDataScans: No data scans found older than %s to delete.", oldThreshold.String())
		return
	}

	for _, scanName := range dataScansToDelete {
		deleteReq := &dataplexpb.DeleteDataScanRequest{Name: scanName}
		op, err := client.DeleteDataScan(ctx, deleteReq)
		if err != nil {
			t.Logf("Warning: Failed to delete data scan %s: %v", scanName, err)
			continue
		}

		err = op.Wait(ctx)
		if err != nil {
			t.Logf("Warning: Failed to wait for DeleteDataScan %s: %v", scanName, err)
		} else {
			t.Logf("cleanupOldDataScans: Successfully deleted data scan %s", scanName)
		}
	}
}

func setupDataplexSearchDataQualityScan(t *testing.T, ctx context.Context, client *dataplex.DataScanClient, dataScanId string, datasetName string, tableName string) func(*testing.T) {
	parent := fmt.Sprintf("projects/%s/locations/us-central1", DataplexProject)
	tableResource := fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s", DataplexProject, datasetName, tableName)

	createDataScanReq := &dataplexpb.CreateDataScanRequest{
		Parent:     parent,
		DataScanId: dataScanId,
		DataScan: &dataplexpb.DataScan{
			Data: &dataplexpb.DataSource{
				Source: &dataplexpb.DataSource_Resource{
					Resource: tableResource,
				},
			},
			Spec: &dataplexpb.DataScan_DataQualitySpec{
				DataQualitySpec: &dataplexpb.DataQualitySpec{
					Rules: []*dataplexpb.DataQualityRule{
						{
							RuleType: &dataplexpb.DataQualityRule_NonNullExpectation_{
								NonNullExpectation: &dataplexpb.DataQualityRule_NonNullExpectation{},
							},
							Dimension: "COMPLETENESS",
							Column:    "col1",
						},
					},
				},
			},
		},
	}

	op, err := client.CreateDataScan(ctx, createDataScanReq)
	if err != nil {
		t.Fatalf("Failed to create data scan %s: %v", dataScanId, err)
	}

	// Wait for creation
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("Failed to wait for create data scan %s: %v", dataScanId, err)
	}

	return func(t *testing.T) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cleanupCancel()
		deleteDataScanReq := &dataplexpb.DeleteDataScanRequest{
			Name: fmt.Sprintf("%s/dataScans/%s", parent, dataScanId),
		}
		op, err := client.DeleteDataScan(cleanupCtx, deleteDataScanReq)
		if err != nil {
			t.Errorf("Failed to delete data scan %s: %v", dataScanId, err)
			return
		}
		if err := op.Wait(cleanupCtx); err != nil {
			t.Logf("Warning: Failed to wait for delete data scan %s: %v", dataScanId, err)
		}
	}
}

func initDataplexDataScanConnection(ctx context.Context) (*dataplex.DataScanClient, error) {
	cred, err := google.FindDefaultCredentials(ctx, sources.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials: %w", err)
	}

	client, err := dataplex.NewDataScanClient(ctx, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Dataplex DataScan client %w", err)
	}
	return client, nil
}

func initDataplexDataProductConnection(ctx context.Context) (*dataplex.DataProductClient, error) {
	cred, err := google.FindDefaultCredentials(ctx, sources.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials: %w", err)
	}

	client, err := dataplex.NewDataProductClient(ctx, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Dataplex DataProduct client %w", err)
	}
	return client, nil
}

func TestDataplexToolEndpoints(t *testing.T) {
	sourceConfig := getDataplexVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	bigqueryClient, err := initBigQueryConnection(ctx, DataplexProject)
	if err != nil {
		t.Fatalf("unable to create Cloud SQL connection pool: %s", err)
	}

	dataplexClient, err := initDataplexConnection(ctx)
	if err != nil {
		t.Fatalf("unable to create Dataplex connection: %s", err)
	}

	dataplexDataScanClient, err := initDataplexDataScanConnection(ctx)
	if err != nil {
		t.Fatalf("unable to create Dataplex DataScan connection: %s", err)
	}

	dataplexDataProductClient, err := initDataplexDataProductConnection(ctx)
	if err != nil {
		t.Fatalf("unable to create Dataplex DataProduct connection: %s", err)
	}

	// Cleanup older aspect types and data products that may have leaked due to aborted tests
	const cleanupThresholdTime = 30 * time.Minute
	cleanupOldAspectTypes(t, ctx, dataplexClient, cleanupThresholdTime)
	cleanupOldDataProducts(t, ctx, dataplexDataProductClient, cleanupThresholdTime)
	cleanupOldDataScans(t, ctx, dataplexDataScanClient, cleanupThresholdTime)

	datasetName1 := fmt.Sprintf("temp_toolbox_test_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	datasetName2 := fmt.Sprintf("temp_toolbox_test_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableName1 := fmt.Sprintf("param_table_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableName2 := fmt.Sprintf("param_table_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	aspectTypeId := fmt.Sprintf("param-aspect-type-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataScanId := fmt.Sprintf("param-data-scan-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataProductId1 := fmt.Sprintf("param-data-product-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataProductId2 := fmt.Sprintf("param-data-product-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataProductId3 := fmt.Sprintf("param-data-product-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataProductId4 := fmt.Sprintf("param-data-product-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataAssetId1 := fmt.Sprintf("param-data-asset-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataAssetId2 := fmt.Sprintf("param-data-asset-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataAssetId3 := fmt.Sprintf("param-data-asset-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	bucketName := fmt.Sprintf("temp-toolbox-test-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	teardownTable1 := setupBigQueryTable(t, ctx, bigqueryClient, datasetName1, tableName1)
	teardownTable2 := setupBigQueryTable(t, ctx, bigqueryClient, datasetName2, tableName2)
	teardownAspectType := setupDataplexThirdPartyAspectType(t, ctx, dataplexClient, aspectTypeId)
	teardownDataScan := setupDataplexSearchDataQualityScan(t, ctx, dataplexDataScanClient, dataScanId, datasetName1, tableName1)
	teardownDataProduct1 := setupDataplexDataProduct(t, ctx, dataplexDataProductClient, dataProductId1)
	teardownDataProduct2 := setupDataplexDataProduct(t, ctx, dataplexDataProductClient, dataProductId2)
	teardownDataAsset1 := setupDataplexDataAsset(t, ctx, dataplexDataProductClient, dataProductId1, dataAssetId1, datasetName1, tableName1)
	teardownDataProduct3 := func(t *testing.T) {
		teardownDataProduct(t, dataplexDataProductClient, dataProductId3)
	}
	teardownDataProduct4 := func(t *testing.T) {
		teardownDataProduct(t, dataplexDataProductClient, dataProductId4)
	}
	teardownBucket := setupGcsBucket(t, ctx, DataplexProject, bucketName)

	teardowns := []func(*testing.T){
		teardownAspectType,
		teardownDataScan,
		teardownDataProduct3,
		teardownDataProduct4,
		// Sequence asset deletion before its parent data product to avoid API precondition failure
		func(t *testing.T) {
			teardownTable1(t)
			teardownDataAsset1(t)
			teardownDataProduct1(t)
			teardownTable2(t)
			teardownDataAsset(t, dataplexDataProductClient, dataProductId2, dataAssetId2)
			teardownDataAsset(t, dataplexDataProductClient, dataProductId2, dataAssetId3)
			teardownDataProduct2(t)
		},
		teardownBucket,
	}

	// Wait for table and aspect type to be ingested.
	// Also clears the Data Products API quota so calls made in cleanupOldDataProducts don't push the actual integration test calls over the quota limit.
	time.Sleep(1 * time.Minute)
	// Execute teardowns concurrently using a WaitGroup to minimize overall test cleanup duration.
	defer func() {
		var wg sync.WaitGroup
		for _, fn := range teardowns {
			wg.Add(1)
			go func(cleanup func(*testing.T)) {
				defer wg.Done()
				cleanup(t)
			}(fn)
		}
		wg.Wait()
	}()

	toolsFile := getDataplexToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runDataplexToolGetTest(t)
	runDataplexSearchEntriesToolInvokeTest(t, tableName1, datasetName1)
	runDataplexLookupEntryToolInvokeTest(t, tableName1, datasetName1)
	runDataplexSearchAspectTypesToolInvokeTest(t, aspectTypeId)
	runDataplexLookupContextToolInvokeTest(t, tableName1, datasetName1)
	runDataplexSearchDataQualityScansToolInvokeTest(t, dataScanId, tableName1, datasetName1)
	runDataplexListDataProductsToolInvokeTest(t, dataProductId1, dataProductId2)
	runDataplexGetDataProductToolInvokeTest(t, dataProductId1)
	runDataplexListDataAssetsToolInvokeTest(t, dataProductId1, dataAssetId1)
	runDataplexGetDataAssetToolInvokeTest(t, dataProductId1, dataAssetId1)
	runDataplexCreateAndUpdateDataProductToolsInvokeTest(t, dataplexDataProductClient, dataProductId3, dataProductId4)
	runDataplexCreateAndUpdateDataAssetToolsInvokeTest(t, dataplexDataProductClient, dataProductId2, dataAssetId2, dataAssetId3, datasetName1, tableName1, datasetName2, tableName2)
	runDataplexUpdateDataProductAspectsToolInvokeTest(t, dataProductId1, aspectTypeId)
	runDataplexEnrichmentToolInvokeTest(t, tableName1, datasetName1, bucketName, dataplexDataScanClient)
}

func setupBigQueryTable(t *testing.T, ctx context.Context, client *bigqueryapi.Client, datasetName string, tableName string) func(*testing.T) {
	// Create dataset
	dataset := client.Dataset(datasetName)
	_, err := dataset.Metadata(ctx)

	if err != nil {
		apiErr, ok := err.(*googleapi.Error)
		if !ok || apiErr.Code != 404 {
			t.Fatalf("Failed to check dataset %q existence: %v", datasetName, err)
		}
		metadataToCreate := &bigqueryapi.DatasetMetadata{Name: datasetName, Location: "us"}
		if err := dataset.Create(ctx, metadataToCreate); err != nil {
			t.Fatalf("Failed to create dataset %q: %v", datasetName, err)
		}
	}

	// Create table
	tab := client.Dataset(datasetName).Table(tableName)
	meta := &bigqueryapi.TableMetadata{
		Schema: bigqueryapi.Schema{
			{Name: "col1", Type: bigqueryapi.StringFieldType},
		},
	}
	if err := tab.Create(ctx, meta); err != nil {
		t.Fatalf("Create table job for %s failed: %v", tableName, err)
	}

	return func(t *testing.T) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cleanupCancel()

		// tear down table
		dropSQL := fmt.Sprintf("drop table %s.%s", datasetName, tableName)
		dropJob, err := client.Query(dropSQL).Run(cleanupCtx)
		if err != nil {
			t.Errorf("Failed to start drop table job for %s: %v", tableName, err)
			return
		}
		dropStatus, err := dropJob.Wait(cleanupCtx)
		if err != nil {
			t.Errorf("Failed to wait for drop table job for %s: %v", tableName, err)
			return
		}
		if err := dropStatus.Err(); err != nil {
			t.Errorf("Error dropping table %s: %v", tableName, err)
		}

		// tear down dataset
		datasetToTeardown := client.Dataset(datasetName)
		tablesIterator := datasetToTeardown.Tables(cleanupCtx)
		_, err = tablesIterator.Next()

		if err == iterator.Done {
			if err := datasetToTeardown.Delete(cleanupCtx); err != nil {
				t.Errorf("Failed to delete dataset %s: %v", datasetName, err)
			}
		} else if err != nil {
			t.Errorf("Failed to list tables in dataset %s to check emptiness: %v.", datasetName, err)
		}
	}
}

func teardownDataProduct(t *testing.T, client *dataplex.DataProductClient, dataProductId string) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cleanupCancel()
	deleteReq := &dataplexpb.DeleteDataProductRequest{
		Name: fmt.Sprintf("projects/%s/locations/us/dataProducts/%s", DataplexProject, dataProductId),
	}
	op, err := client.DeleteDataProduct(cleanupCtx, deleteReq)
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			t.Logf("Data Product %s was not found, skipping deletion", dataProductId)
			return
		}
		t.Errorf("Failed to initiate DeleteDataProduct for %s: %v", dataProductId, err)
		return
	}
	err = op.Wait(cleanupCtx)
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			t.Logf("Data Product %s was not found during wait, skipping deletion", dataProductId)
			return
		}
		t.Logf("Warning: Failed to wait for DeleteDataProduct for %s: %v", dataProductId, err)
	}
}

func setupDataplexDataProduct(t *testing.T, ctx context.Context, client *dataplex.DataProductClient, dataProductId string) func(*testing.T) {
	parent := fmt.Sprintf("projects/%s/locations/us", DataplexProject)
	ownerEmail := tests.ServiceAccountEmail
	if ownerEmail == "" {
		t.Fatalf("Service account email is required, but tests.ServiceAccountEmail is empty.")
	}
	if !strings.HasSuffix(ownerEmail, ".gserviceaccount.com") {
		t.Fatalf("Service account email %q is invalid. Dataplex Data Product integration tests require a service account email ending with '.gserviceaccount.com' to validate access groups.", ownerEmail)
	}
	createReq := &dataplexpb.CreateDataProductRequest{
		Parent:        parent,
		DataProductId: dataProductId,
		DataProduct: &dataplexpb.DataProduct{
			DisplayName: dataProductId,
			Description: "Temporary Data Product for MCP Toolbox integration tests",
			OwnerEmails: []string{ownerEmail},
			AccessGroups: map[string]*dataplexpb.DataProduct_AccessGroup{
				"test-group": {
					Id:          "test-group",
					DisplayName: "Test Group",
					Principal: &dataplexpb.DataProduct_Principal{
						ServiceAccount: &ownerEmail,
					},
				},
			},
		},
	}

	op, err := client.CreateDataProduct(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to initiate CreateDataProduct for %s: %v", dataProductId, err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		teardownDataProduct(t, client, dataProductId)
		t.Fatalf("Failed to wait for CreateDataProduct for %s: %v", dataProductId, err)
	}

	return func(t *testing.T) {
		teardownDataProduct(t, client, dataProductId)
	}
}

func teardownDataAsset(t *testing.T, client *dataplex.DataProductClient, dataProductId string, dataAssetId string) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cleanupCancel()
	deleteReq := &dataplexpb.DeleteDataAssetRequest{
		Name: fmt.Sprintf("projects/%s/locations/us/dataProducts/%s/dataAssets/%s", DataplexProject, dataProductId, dataAssetId),
	}
	op, err := client.DeleteDataAsset(cleanupCtx, deleteReq)
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			t.Logf("Data Asset %s was not found, skipping deletion", dataAssetId)
			return
		}
		t.Errorf("Failed to initiate DeleteDataAsset for %s: %v", dataAssetId, err)
		return
	}
	err = op.Wait(cleanupCtx)
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			t.Logf("Data Asset %s was not found during wait, skipping deletion", dataAssetId)
			return
		}
		t.Logf("Warning: Failed to wait for DeleteDataAsset for %s: %v", dataAssetId, err)
	}
}

func setupDataplexDataAsset(t *testing.T, ctx context.Context, client *dataplex.DataProductClient, dataProductId string, dataAssetId string, datasetName string, tableName string) func(*testing.T) {
	parentProductPath := fmt.Sprintf("projects/%s/locations/us/dataProducts/%s", DataplexProject, dataProductId)
	resource := fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s", DataplexProject, datasetName, tableName)
	createReq := &dataplexpb.CreateDataAssetRequest{
		Parent:      parentProductPath,
		DataAssetId: dataAssetId,
		DataAsset: &dataplexpb.DataAsset{
			Resource: resource,
			Labels: map[string]string{
				"env": "test",
			},
		},
	}

	teardown := func(t *testing.T) {
		teardownDataAsset(t, client, dataProductId, dataAssetId)
	}

	op, err := client.CreateDataAsset(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to initiate CreateDataAsset for %s: %v", dataAssetId, err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		teardown(t)
		t.Fatalf("Failed to wait for CreateDataAsset for %s: %v", dataAssetId, err)
	}

	return teardown
}

func setupGcsBucket(t *testing.T, ctx context.Context, project string, bucketName string) func(*testing.T) {
	cred, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		t.Fatalf("failed to find default credentials: %v", err)
	}
	client, err := storageapi.NewClient(ctx, option.WithCredentials(cred))
	if err != nil {
		t.Fatalf("failed to create storage client: %v", err)
	}

	bucket := client.Bucket(bucketName)
	if err := bucket.Create(ctx, project, &storageapi.BucketAttrs{Location: "us-central1"}); err != nil {
		t.Fatalf("failed to create bucket %s: %v", bucketName, err)
	}

	return func(t *testing.T) {
		if err := bucket.Delete(context.WithoutCancel(ctx)); err != nil {
			t.Logf("cleanup: failed to delete bucket %s: %v", bucketName, err)
		}
	}
}

func setupDataplexThirdPartyAspectType(t *testing.T, ctx context.Context, client *dataplex.CatalogClient, aspectTypeId string) func(*testing.T) {
	parent := fmt.Sprintf("projects/%s/locations/us", DataplexProject)
	createAspectTypeReq := &dataplexpb.CreateAspectTypeRequest{
		Parent:       parent,
		AspectTypeId: aspectTypeId,
		AspectType: &dataplexpb.AspectType{
			Name: fmt.Sprintf("%s/aspectTypes/%s", parent, aspectTypeId),
			MetadataTemplate: &dataplexpb.AspectType_MetadataTemplate{
				Name: "UserSchema",
				Type: "record",
			},
		},
	}
	op, err := client.CreateAspectType(ctx, createAspectTypeReq)
	if err != nil {
		t.Fatalf("Failed to initiate CreateAspectType %s: %v", aspectTypeId, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("Failed to wait for CreateAspectType %s: %v", aspectTypeId, err)
	}

	return func(t *testing.T) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cleanupCancel()

		// tear down aspect type
		deleteAspectTypeReq := &dataplexpb.DeleteAspectTypeRequest{
			Name: fmt.Sprintf("%s/aspectTypes/%s", parent, aspectTypeId),
		}
		if _, err := client.DeleteAspectType(cleanupCtx, deleteAspectTypeReq); err != nil {
			t.Errorf("Failed to delete aspect type %s: %v", aspectTypeId, err)
		}
	}
}

func getDataplexToolsConfig(sourceConfig map[string]any) map[string]any {
	// Write config into a file and pass it to command
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-dataplex-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"my-dataplex-search-entries-tool": map[string]any{
				"type":        DataplexSearchEntriesToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex search entries tool to test end to end functionality.",
			},
			"my-auth-dataplex-search-entries-tool": map[string]any{
				"type":         DataplexSearchEntriesToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex search entries tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-lookup-entry-tool": map[string]any{
				"type":        DataplexLookupEntryToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex lookup entry tool to test end to end functionality.",
			},
			"my-auth-dataplex-lookup-entry-tool": map[string]any{
				"type":         DataplexLookupEntryToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex lookup entry tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-search-aspect-types-tool": map[string]any{
				"type":        DataplexSearchAspectTypesToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex search aspect types tool to test end to end functionality.",
			},
			"my-auth-dataplex-search-aspect-types-tool": map[string]any{
				"type":         DataplexSearchAspectTypesToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex search aspect types tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-lookup-context-tool": map[string]any{
				"type":        DataplexLookupContextToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex lookup context tool to test end to end functionality.",
			},
			"my-auth-dataplex-lookup-context-tool": map[string]any{
				"type":         DataplexLookupContextToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex lookup context tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-search-dq-scans-tool": map[string]any{
				"type":        DataplexSearchDataQualityScansToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex search dq scans tool to test end to end functionality.",
			},
			"my-auth-dataplex-search-dq-scans-tool": map[string]any{
				"type":         DataplexSearchDataQualityScansToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex search dq scans tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-list-data-products-tool": map[string]any{
				"type":        DataplexListDataProductsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex list data products tool to test end to end functionality.",
			},
			"my-auth-dataplex-list-data-products-tool": map[string]any{
				"type":         DataplexListDataProductsToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex list data products tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-get-data-product-tool": map[string]any{
				"type":        DataplexGetDataProductToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get data product tool to test end to end functionality.",
			},
			"my-auth-dataplex-get-data-product-tool": map[string]any{
				"type":         DataplexGetDataProductToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex get data product tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-list-data-assets-tool": map[string]any{
				"type":        DataplexListDataAssetsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex list data assets tool to test end to end functionality.",
			},
			"my-auth-dataplex-list-data-assets-tool": map[string]any{
				"type":         DataplexListDataAssetsToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex list data assets tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-get-data-asset-tool": map[string]any{
				"type":        DataplexGetDataAssetToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get data asset tool to test end to end functionality.",
			},
			"my-auth-dataplex-get-data-asset-tool": map[string]any{
				"type":         DataplexGetDataAssetToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex get data asset tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-create-data-product-tool": map[string]any{
				"type":        DataplexCreateDataProductToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex create data product tool to test end to end functionality.",
			},
			"my-auth-dataplex-create-data-product-tool": map[string]any{
				"type":         DataplexCreateDataProductToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex create data product tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-update-data-product-tool": map[string]any{
				"type":        DataplexUpdateDataProductToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex update data product tool to test end to end functionality.",
			},
			"my-auth-dataplex-update-data-product-tool": map[string]any{
				"type":         DataplexUpdateDataProductToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex update data product tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-create-data-asset-tool": map[string]any{
				"type":        DataplexCreateDataAssetToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex create data asset tool to test end to end functionality.",
			},
			"my-auth-dataplex-create-data-asset-tool": map[string]any{
				"type":         DataplexCreateDataAssetToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex create data asset tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-update-data-asset-tool": map[string]any{
				"type":        DataplexUpdateDataAssetToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex update data asset tool to test end to end functionality.",
			},
			"my-auth-dataplex-update-data-asset-tool": map[string]any{
				"type":         DataplexUpdateDataAssetToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex update data asset tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-update-data-product-aspects-tool": map[string]any{
				"type":        DataplexUpdateDataProductAspectsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex update data product aspects tool to test end to end functionality.",
			},
			"my-auth-dataplex-update-data-product-aspects-tool": map[string]any{
				"type":         DataplexUpdateDataProductAspectsToolType,
				"source":       "my-dataplex-instance",
				"description":  "Simple dataplex update data product aspects tool to test end to end functionality.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-dataplex-generate-data-profile-tool": map[string]any{
				"type":        DataplexGenerateDataProfileToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex generate data profile tool to test end to end functionality.",
			},
			"my-dataplex-get-data-profile-tool": map[string]any{
				"type":        DataplexGetDataProfileToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get data profile tool to test end to end functionality.",
			},
			"my-dataplex-get-operation-tool": map[string]any{
				"type":        DataplexGetOperationToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get operation tool to test end to end functionality.",
			},
			"my-dataplex-get-run-status-tool": map[string]any{
				"type":        DataplexGetRunStatusToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get run status tool to test end to end functionality.",
			},
			"my-dataplex-generate-data-insights-tool": map[string]any{
				"type":        DataplexGenerateDataInsightsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex generate data insights tool to test end to end functionality.",
			},
			"my-dataplex-get-data-insights-tool": map[string]any{
				"type":        DataplexGetDataInsightsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get data insights tool to test end to end functionality.",
			},
			"my-dataplex-discover-metadata-tool": map[string]any{
				"type":        DataplexDiscoverMetadataToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex discover metadata tool to test end to end functionality.",
			},
			"my-dataplex-get-discovery-results-tool": map[string]any{
				"type":        DataplexGetDiscoveryResultsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get discovery results tool to test end to end functionality.",
			},
			"my-dataplex-check-data-quality-tool": map[string]any{
				"type":        DataplexCheckDataQualityToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex check data quality tool to test end to end functionality.",
			},
			"my-dataplex-get-data-quality-results-tool": map[string]any{
				"type":        DataplexGetDataQualityResultsToolType,
				"source":      "my-dataplex-instance",
				"description": "Simple dataplex get data quality results tool to test end to end functionality.",
			},
		},
	}

	return toolsFile
}

func runDataplexToolGetTest(t *testing.T) {
	testCases := []struct {
		name           string
		toolName       string
		expectedParams []string
	}{
		{
			name:           "get my-dataplex-search-entries-tool",
			toolName:       "my-dataplex-search-entries-tool",
			expectedParams: []string{"pageSize", "query", "orderBy", "scope"},
		},
		{
			name:           "get my-dataplex-lookup-entry-tool",
			toolName:       "my-dataplex-lookup-entry-tool",
			expectedParams: []string{"entry", "view", "aspectTypes"},
		},
		{
			name:           "get my-dataplex-search-aspect-types-tool",
			toolName:       "my-dataplex-search-aspect-types-tool",
			expectedParams: []string{"pageSize", "query", "orderBy"},
		},
		{
			name:           "get my-dataplex-search-dq-scans-tool",
			toolName:       "my-dataplex-search-dq-scans-tool",
			expectedParams: []string{"filter", "dataScanId", "resourcePath", "pageSize", "orderBy"},
		},
		{
			name:           "get my-dataplex-list-data-products-tool",
			toolName:       "my-dataplex-list-data-products-tool",
			expectedParams: []string{"filter", "pageSize", "orderBy"},
		},
		{
			name:           "get my-dataplex-get-data-product-tool",
			toolName:       "my-dataplex-get-data-product-tool",
			expectedParams: []string{"locationId", "dataProductId"},
		},
		{
			name:           "get my-dataplex-list-data-assets-tool",
			toolName:       "my-dataplex-list-data-assets-tool",
			expectedParams: []string{"locationId", "dataProductId", "filter", "pageSize", "orderBy"},
		},
		{
			name:           "get my-dataplex-get-data-asset-tool",
			toolName:       "my-dataplex-get-data-asset-tool",
			expectedParams: []string{"locationId", "dataProductId", "dataAssetId"},
		},
		{
			name:           "get my-dataplex-create-data-product-tool",
			toolName:       "my-dataplex-create-data-product-tool",
			expectedParams: []string{"locationId", "dataProductId", "displayName", "description", "ownerEmails", "accessGroups"},
		},
		{
			name:           "get my-dataplex-update-data-product-tool",
			toolName:       "my-dataplex-update-data-product-tool",
			expectedParams: []string{"locationId", "dataProductId", "description", "displayName", "ownerEmails", "accessGroups", "updateMask"},
		},
		{
			name:           "get my-dataplex-create-data-asset-tool",
			toolName:       "my-dataplex-create-data-asset-tool",
			expectedParams: []string{"locationId", "dataProductId", "dataAssetId", "resourceUri", "labels", "accessGroupConfigs"},
		},
		{
			name:           "get my-dataplex-update-data-asset-tool",
			toolName:       "my-dataplex-update-data-asset-tool",
			expectedParams: []string{"locationId", "dataProductId", "dataAssetId", "labels", "accessGroupConfigs", "updateMask"},
		},
		{
			name:           "get my-dataplex-update-data-product-aspects-tool",
			toolName:       "my-dataplex-update-data-product-aspects-tool",
			expectedParams: []string{"locationId", "dataProductId", "aspects"},
		},
		{
			name:           "get my-dataplex-generate-data-profile-tool",
			toolName:       "my-dataplex-generate-data-profile-tool",
			expectedParams: []string{"resourcePath", "location", "publish"},
		},
		{
			name:           "get my-dataplex-get-data-profile-tool",
			toolName:       "my-dataplex-get-data-profile-tool",
			expectedParams: []string{"scanId", "location"},
		},
		{
			name:           "get my-dataplex-get-operation-tool",
			toolName:       "my-dataplex-get-operation-tool",
			expectedParams: []string{"operationName"},
		},
		{
			name:           "get my-dataplex-get-run-status-tool",
			toolName:       "my-dataplex-get-run-status-tool",
			expectedParams: []string{"scanId", "location"},
		},
		{
			name:           "get my-dataplex-generate-data-insights-tool",
			toolName:       "my-dataplex-generate-data-insights-tool",
			expectedParams: []string{"resourcePath", "location", "publish"},
		},
		{
			name:           "get my-dataplex-get-data-insights-tool",
			toolName:       "my-dataplex-get-data-insights-tool",
			expectedParams: []string{"scanId", "location"},
		},
		{
			name:           "get my-dataplex-discover-metadata-tool",
			toolName:       "my-dataplex-discover-metadata-tool",
			expectedParams: []string{"resourcePath", "location"},
		},
		{
			name:           "get my-dataplex-get-discovery-results-tool",
			toolName:       "my-dataplex-get-discovery-results-tool",
			expectedParams: []string{"scanId", "location"},
		},
		{
			name:           "get my-dataplex-check-data-quality-tool",
			toolName:       "my-dataplex-check-data-quality-tool",
			expectedParams: []string{"resourcePath", "location", "specJSON", "publish"},
		},
		{
			name:           "get my-dataplex-get-data-quality-results-tool",
			toolName:       "my-dataplex-get-data-quality-results-tool",
			expectedParams: []string{"scanId", "location"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/", tc.toolName))
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("response status code is not 200")
			}
			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}
			got, ok := body["tools"]
			if !ok {
				t.Fatalf("unable to find tools in response body")
			}

			toolsMap, ok := got.(map[string]interface{})
			if !ok {
				t.Fatalf("expected 'tools' to be a map, got %T", got)
			}
			tool, ok := toolsMap[tc.toolName].(map[string]interface{})
			if !ok {
				t.Fatalf("expected tool %q to be a map, got %T", tc.toolName, toolsMap[tc.toolName])
			}
			params, ok := tool["parameters"].([]interface{})
			if !ok {
				t.Fatalf("expected 'parameters' to be a slice, got %T", tool["parameters"])
			}
			paramSet := make(map[string]struct{})
			for _, param := range params {
				paramMap, ok := param.(map[string]interface{})
				if ok {
					if name, ok := paramMap["name"].(string); ok {
						paramSet[name] = struct{}{}
					}
				}
			}
			var missing []string
			for _, want := range tc.expectedParams {
				if _, found := paramSet[want]; !found {
					missing = append(missing, want)
				}
			}
			if len(missing) > 0 {
				t.Fatalf("missing parameters for tool %q: %v", tc.toolName, missing)
			}
		})
	}
}

func runDataplexSearchEntriesToolInvokeTest(t *testing.T, tableName string, datasetName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name           string
		api            string
		requestHeader  map[string]string
		requestBody    io.Reader
		wantStatusCode int
		expectResult   bool
		wantContentKey string
		wantCount      int
	}{
		{
			name:           "Success - Entry Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"displayname=%s system=bigquery parent:%s\"}", tableName, datasetName))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "dataplex_entry",
		},
		{
			name:           "Success - Entry Found with Scope",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"displayname=%s system=bigquery parent:%s\", \"scope\":\"projects/%s\"}", tableName, datasetName, DataplexProject))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "dataplex_entry",
		},
		{
			name:           "Success - Limit Results by PageSize",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte("{\"query\":\"system=bigquery\", \"pageSize\": 2}")),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "dataplex_entry",
			wantCount:      2,
		},
		{
			name:           "Success with Authorization - Entry Found",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": idToken},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"displayname=%s system=bigquery parent:%s\"}", tableName, datasetName))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "dataplex_entry",
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"displayname=%s system=bigquery parent:%s\"}", tableName, datasetName))),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "dataplex_entry",
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"displayname=%s system=bigquery parent:%s\"}", tableName, datasetName))),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "dataplex_entry",
		},
		{
			name:           "Failure - Entry Not Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-entries-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(`{"query":"displayname=\"\" system=bigquery parent:\"\""}`)),
			wantStatusCode: 200,
			expectResult:   false,
			wantContentKey: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				if result["result"] == nil && !tc.expectResult {
					return
				}
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			if !tc.expectResult && (resultStr == "" || resultStr == "[]") {
				return
			}
			var entries []interface{}
			if err := json.Unmarshal([]byte(resultStr), &entries); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			if tc.expectResult {
				wantCount := tc.wantCount
				if wantCount == 0 {
					wantCount = 1
				}
				if len(entries) != wantCount {
					t.Fatalf("expected exactly %d entries, but got %d", wantCount, len(entries))
				}
				entry, ok := entries[0].(map[string]interface{})
				if !ok {
					t.Fatalf("expected first entry to be a map, got %T", entries[0])
				}
				if _, ok := entry[tc.wantContentKey]; !ok {
					t.Fatalf("expected entry to have key '%s', but it was not found in %v", tc.wantContentKey, entry)
				}
			} else {
				isResultEmpty := resultStr == "" || resultStr == "[]" || resultStr == "null"
				hasError := strings.Contains(resultStr, `"error":`)

				if !isResultEmpty && !hasError {
					t.Fatalf("expected an empty result or error message, but got: %s", resultStr)
				}
			}
		})
	}
}

func runDataplexLookupEntryToolInvokeTest(t *testing.T, tableName string, datasetName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name               string
		wantStatusCode     int
		api                string
		requestHeader      map[string]string
		requestBody        io.Reader
		expectResult       bool
		wantContentKey     string
		dontWantContentKey string
		aspectCheck        bool
		reqBodyMap         map[string]any
	}{
		{
			name:           "Success - Entry Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"name\":\"projects/%s/locations/us\", \"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s\"}", DataplexProject, DataplexProject, DataplexProject, datasetName))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "name",
		},
		{
			name:           "Success - Entry Found with Authorization",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": idToken},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s\"}", DataplexProject, DataplexProject, datasetName))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "name",
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s\"}", DataplexProject, DataplexProject, datasetName))),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "name",
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s\"}", DataplexProject, DataplexProject, datasetName))),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "name",
		},
		{
			name:           "Failure - Invalid Entry Format",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(`{"entry":"invalid/entry/format"}`)),
			wantStatusCode: 200,
			expectResult:   false,
		},
		{
			name:           "Failure - Entry Not Found or Permission Denied",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s\"}", DataplexProject, DataplexProject, "non-existent-dataset"))),
			wantStatusCode: 200,
			expectResult:   false,
		},
		{
			name:               "Success - Entry Found with Basic View",
			api:                "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-entry-tool/invoke",
			requestHeader:      map[string]string{},
			requestBody:        bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s\", \"view\": %d}", DataplexProject, DataplexProject, datasetName, tableName, 1))),
			wantStatusCode:     200,
			expectResult:       true,
			wantContentKey:     "name",
			dontWantContentKey: "aspects",
		},
		{
			name:           "Failure - Entry with Custom View without Aspect Types",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s\", \"view\": %d}", DataplexProject, DataplexProject, datasetName, tableName, 3))),
			wantStatusCode: 200,
			expectResult:   false,
		},
		{
			name:           "Success - Entry Found with only Schema Aspect",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-entry-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"entry\":\"projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s\", \"aspectTypes\":[\"projects/dataplex-types/locations/global/aspectTypes/schema\"], \"view\": %d}", DataplexProject, DataplexProject, datasetName, tableName, 3))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "aspects",
			aspectCheck:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("Response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}

			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("Error parsing response body: %v", err)
			}

			resultStr, hasResult := result["result"].(string)

			if tc.expectResult {
				if !hasResult || resultStr == "" || resultStr == "{}" || resultStr == "null" {
					t.Fatalf("Expected a result, but got: %v", result)
				}

				var entry map[string]interface{}
				if err := json.Unmarshal([]byte(resultStr), &entry); err != nil {
					t.Fatalf("Error unmarshalling result string: %v. Raw result: %s", err, resultStr)
				}

				if _, ok := entry[tc.wantContentKey]; !ok {
					t.Fatalf("Expected entry to have key '%s', but it was not found in %v", tc.wantContentKey, entry)
				}

				if tc.dontWantContentKey != "" {
					if _, ok := entry[tc.dontWantContentKey]; ok {
						t.Fatalf("Expected entry to NOT have key '%s', but it was found", tc.dontWantContentKey)
					}
				}

				if tc.aspectCheck {
					aspects, ok := entry["aspects"].(map[string]interface{})
					if !ok || len(aspects) != 1 {
						t.Fatalf("Expected exactly one aspect, but got %d", len(aspects))
					}
				}
			} else {
				foundError := false
				if _, ok := result["error"]; ok {
					foundError = true
				} else if hasResult && strings.Contains(resultStr, `"error"`) {
					foundError = true
				}

				if !foundError {
					t.Fatalf("Expected an error in response, but none was found. Response: %v", result)
				}
			}
		})
	}
}

func runDataplexSearchAspectTypesToolInvokeTest(t *testing.T, aspectTypeId string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name           string
		api            string
		requestHeader  map[string]string
		requestBody    io.Reader
		wantStatusCode int
		expectResult   bool
		wantContentKey string
	}{
		{
			name:           "Success - Aspect Type Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-aspect-types-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"name:%s\"}", aspectTypeId))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "metadata_template",
		},
		{
			name:           "Success - Aspect Type Found with Authorization",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-aspect-types-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": idToken},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"name:%s\"}", aspectTypeId))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "metadata_template",
		},
		{
			name:           "Failure - Aspect Type Not Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-aspect-types-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(`{"query":"name:non-existent-aspect-type"}`)),
			wantStatusCode: 200,
			expectResult:   false,
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-aspect-types-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"name:%s\"}", aspectTypeId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:           "Failure - No Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-aspect-types-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"query\":\"name:%s\"}", aspectTypeId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				if result["result"] == nil && !tc.expectResult {
					return
				}
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			if !tc.expectResult && (resultStr == "" || resultStr == "[]") {
				return
			}
			var entries []interface{}
			if err := json.Unmarshal([]byte(resultStr), &entries); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			if tc.expectResult {
				if len(entries) != 1 {
					t.Fatalf("expected exactly one entry, but got %d", len(entries))
				}
				entry, ok := entries[0].(map[string]interface{})
				if !ok {
					t.Fatalf("expected entry to be a map, got %T", entries[0])
				}
				if _, ok := entry[tc.wantContentKey]; !ok {
					t.Fatalf("expected entry to have key '%s', but it was not found in %v", tc.wantContentKey, entry)
				}
			} else {
				if len(entries) != 0 {
					t.Fatalf("expected 0 entries, but got %d", len(entries))
				}
			}
		})
	}
}

func runDataplexLookupContextToolInvokeTest(t *testing.T, tableName string, datasetName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	resourceName := fmt.Sprintf("projects/%s/locations/us/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s", DataplexProject, DataplexProject, datasetName, tableName)
	requestBodyFmt := fmt.Sprintf(`{"resources":["%s"]}`, resourceName)

	testCases := []struct {
		name           string
		wantStatusCode int
		api            string
		requestHeader  map[string]string
		requestBody    io.Reader
		expectResult   bool
		wantContentKey string
	}{
		{
			name:           "Success - Context Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-context-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBufferString(requestBodyFmt),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "context",
		},
		{
			name:           "Success with Authorization - Context Found",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-lookup-context-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": idToken},
			requestBody:    bytes.NewBufferString(requestBodyFmt),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "context",
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-lookup-context-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBufferString(requestBodyFmt),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "context",
		},
		{
			name:           "Failure - Invalid Resource Format",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-context-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBufferString(`{"resources":["projects/test-project/invalid-format"]}`),
			wantStatusCode: 200,
			expectResult:   false,
			wantContentKey: "context",
		},
		{
			name:           "Failure - Resources with different locations",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-lookup-context-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBufferString(`{"resources":["projects/test-project/locations/us/entryGroups/g1/entries/e1", "projects/test-project/locations/europe-west1/entryGroups/g2/entries/e2"]}`),
			wantStatusCode: 200,
			expectResult:   false,
			wantContentKey: "context",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("Response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}

			if !tc.expectResult {
				return
			}

			var response map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &response); err != nil {
				t.Fatalf("Error parsing response body: %v\nRaw body: %s", err, string(bodyBytes))
			}

			resultPayload, ok := response["result"]
			if !ok {
				t.Fatalf("Expected to find 'result' key in API response, got: %v", response)
			}

			resultStr, ok := resultPayload.(string)
			if !ok {
				t.Fatalf("Expected 'result' payload to be a JSON string, got: %T", resultPayload)
			}

			var innerResult map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &innerResult); err != nil {
				t.Fatalf("Error parsing inner result string: %v\nRaw string: %s", err, resultStr)
			}

			contextStr, hasContext := innerResult[tc.wantContentKey].(string)

			if !hasContext {
				t.Fatalf("Expected to have key '%s' in response: %v", tc.wantContentKey, innerResult)
			}

			if contextStr == "" || contextStr == "{}" || contextStr == "null" {
				t.Fatalf("Expected non-empty '%s', but got: %s", tc.wantContentKey, contextStr)
			}
		})
	}
}

func runDataplexSearchDataQualityScansToolInvokeTest(t *testing.T, dataScanId string, tableName string, datasetName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	fullDataScanId := fmt.Sprintf("projects/%s/locations/us-central1/dataScans/%s", DataplexProject, dataScanId)
	fullTableName := fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s", DataplexProject, datasetName, tableName)

	testCases := []struct {
		name           string
		api            string
		requestHeader  map[string]string
		requestBody    io.Reader
		wantStatusCode int
		expectResult   bool
		wantContentKey string
	}{
		{
			name:           "Success - Scan Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-dq-scans-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataScanId\":\"%s\"}", fullDataScanId))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "name",
		},
		{
			name:           "Success - Scan Found by Table Name",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-dq-scans-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"resourcePath\":\"%s\"}", fullTableName))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "name",
		},
		{
			name:           "Success with Authorization - Scan Found",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-dq-scans-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": idToken},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataScanId\":\"%s\"}", fullDataScanId))),
			wantStatusCode: 200,
			expectResult:   true,
			wantContentKey: "name",
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-dq-scans-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataScanId\":\"%s\"}", fullDataScanId))),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "name",
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-search-dq-scans-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataScanId\":\"%s\"}", fullDataScanId))),
			wantStatusCode: 401,
			expectResult:   false,
			wantContentKey: "name",
		},
		{
			name:           "Failure - Scan Not Found",
			api:            "http://127.0.0.1:5000/api/tool/my-dataplex-search-dq-scans-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(`{"dataScanId":"projects/pso-dev-ayala/locations/us-central1/dataScans/non-existent-scan"}`)),
			wantStatusCode: 200,
			expectResult:   false,
			wantContentKey: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				if result["result"] == nil && !tc.expectResult {
					return
				}
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			if !tc.expectResult && (resultStr == "" || resultStr == "[]") {
				return
			}
			var entries []interface{}
			if err := json.Unmarshal([]byte(resultStr), &entries); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			if tc.expectResult {
				if len(entries) != 1 {
					t.Fatalf("expected exactly one entry, but got %d", len(entries))
				}
				entry, ok := entries[0].(map[string]interface{})
				if !ok {
					t.Fatalf("expected first entry to be a map, got %T", entries[0])
				}
				if _, ok := entry[tc.wantContentKey]; !ok {
					t.Fatalf("expected entry to have key '%s', but it was not found in %v", tc.wantContentKey, entry)
				}
			} else {
				if len(entries) != 0 {
					t.Fatalf("expected 0 entries, but got %d", len(entries))
				}
			}
		})
	}
}

func runDataplexListDataProductsToolInvokeTest(t *testing.T, dataProductId1 string, dataProductId2 string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name              string
		api               string
		requestHeader     map[string]string
		requestBody       io.Reader
		wantStatusCode    int
		expectResult      bool
		wantLocationID    string
		wantDataProductID string
	}{
		{
			name:              "Success - Filter Extracts One Product (Authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-auth-dataplex-list-data-products-tool/invoke",
			requestHeader:     map[string]string{"my-google-auth_token": idToken},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"filter\":\"display_name:\\\"%s\\\"\"}", dataProductId1))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId1,
		},
		{
			name:              "Success - PageSize Limits to One (Un-authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-dataplex-list-data-products-tool/invoke",
			requestHeader:     map[string]string{},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"pageSize\":1, \"filter\":\"display_name:\\\"%s\\\" OR display_name:\\\"%s\\\"\"}", dataProductId1, dataProductId2))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: "",
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-list-data-products-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"filter\":\"display_name:\\\"%s\\\"\"}", dataProductId1))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-list-data-products-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"filter\":\"display_name:\\\"%s\\\"\"}", dataProductId1))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var entries []interface{}
			if err := json.Unmarshal([]byte(resultStr), &entries); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			if len(entries) != 1 {
				t.Fatalf("expected exactly one entry, but got %d", len(entries))
			}
			entry, ok := entries[0].(map[string]interface{})
			if !ok {
				t.Fatalf("expected entry to be a map, got %T", entries[0])
			}
			locID, ok := entry["locationId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'locationId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantLocationID != "" && locID != tc.wantLocationID {
				t.Fatalf("expected locationId to be %q, got %q", tc.wantLocationID, locID)
			}
			prodID, ok := entry["dataProductId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'dataProductId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantDataProductID != "" && prodID != tc.wantDataProductID {
				t.Fatalf("expected dataProductId to be %q, got %q", tc.wantDataProductID, prodID)
			}
			// Assert raw SDK fields are cleaned/removed
			if _, ok := entry["uid"]; ok {
				t.Errorf("expected entry to NOT have 'uid' field, but it was found")
			}
			if _, ok := entry["etag"]; ok {
				t.Errorf("expected entry to NOT have 'etag' field, but it was found")
			}
			if _, ok := entry["createTime"]; ok {
				t.Errorf("expected entry to NOT have 'createTime' field, but it was found")
			}
		})
	}
}

func runDataplexGetDataProductToolInvokeTest(t *testing.T, dataProductId string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name              string
		api               string
		requestHeader     map[string]string
		requestBody       io.Reader
		wantStatusCode    int
		expectResult      bool
		wantLocationID    string
		wantDataProductID string
	}{
		{
			name:              "Success - Get Product (Authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-auth-dataplex-get-data-product-tool/invoke",
			requestHeader:     map[string]string{"my-google-auth_token": idToken},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\"}", dataProductId))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId,
		},
		{
			name:              "Success - Get Product (Un-authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-dataplex-get-data-product-tool/invoke",
			requestHeader:     map[string]string{},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\"}", dataProductId))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId,
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-get-data-product-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\"}", dataProductId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-get-data-product-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\"}", dataProductId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &entry); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			locID, ok := entry["locationId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'locationId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantLocationID != "" && locID != tc.wantLocationID {
				t.Fatalf("expected locationId to be %q, got %q", tc.wantLocationID, locID)
			}
			prodID, ok := entry["dataProductId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'dataProductId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantDataProductID != "" && prodID != tc.wantDataProductID {
				t.Fatalf("expected dataProductId to be %q, got %q", tc.wantDataProductID, prodID)
			}
			// Additionally assert key fields are populated
			if entry["displayName"] == "" {
				t.Errorf("displayName should not be empty")
			}
			if entry["ownerEmails"] == nil {
				t.Errorf("ownerEmails should not be nil")
			}
		})
	}
}

func runDataplexListDataAssetsToolInvokeTest(t *testing.T, dataProductId string, dataAssetId string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name              string
		api               string
		requestHeader     map[string]string
		requestBody       io.Reader
		wantStatusCode    int
		expectResult      bool
		wantLocationID    string
		wantDataProductID string
		wantDataAssetID   string
	}{
		{
			name:              "Success - List Data Assets (Authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-auth-dataplex-list-data-assets-tool/invoke",
			requestHeader:     map[string]string{"my-google-auth_token": idToken},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId,
			wantDataAssetID:   dataAssetId,
		},
		{
			name:              "Success - List Data Assets (Un-authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-dataplex-list-data-assets-tool/invoke",
			requestHeader:     map[string]string{},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId,
			wantDataAssetID:   dataAssetId,
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-list-data-assets-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-list-data-assets-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var entries []interface{}
			if err := json.Unmarshal([]byte(resultStr), &entries); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			if len(entries) != 1 {
				t.Fatalf("expected exactly one entry, but got %d", len(entries))
			}
			entry, ok := entries[0].(map[string]interface{})
			if !ok {
				t.Fatalf("expected entry to be a map, got %T", entries[0])
			}
			locID, ok := entry["locationId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'locationId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantLocationID != "" && locID != tc.wantLocationID {
				t.Fatalf("expected locationId to be %q, got %q", tc.wantLocationID, locID)
			}
			prodID, ok := entry["dataProductId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'dataProductId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantDataProductID != "" && prodID != tc.wantDataProductID {
				t.Fatalf("expected dataProductId to be %q, got %q", tc.wantDataProductID, prodID)
			}
			assetID, ok := entry["dataAssetId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'dataAssetId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantDataAssetID != "" && assetID != tc.wantDataAssetID {
				t.Fatalf("expected dataAssetId to be %q, got %q", tc.wantDataAssetID, assetID)
			}

			// Assert output is cleaned
			if _, ok := entry["uid"]; ok {
				t.Errorf("expected entry to NOT have 'uid' field, but it was found")
			}
			if _, ok := entry["etag"]; ok {
				t.Errorf("expected entry to NOT have 'etag' field, but it was found")
			}
			if _, ok := entry["createTime"]; ok {
				t.Errorf("expected entry to NOT have 'createTime' field, but it was found")
			}
		})
	}
}

func runDataplexGetDataAssetToolInvokeTest(t *testing.T, dataProductId string, dataAssetId string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name              string
		api               string
		requestHeader     map[string]string
		requestBody       io.Reader
		wantStatusCode    int
		expectResult      bool
		wantLocationID    string
		wantDataProductID string
		wantDataAssetID   string
	}{
		{
			name:              "Success - Get Data Asset (Authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-auth-dataplex-get-data-asset-tool/invoke",
			requestHeader:     map[string]string{"my-google-auth_token": idToken},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId,
			wantDataAssetID:   dataAssetId,
		},
		{
			name:              "Success - Get Data Asset (Un-authorized)",
			api:               "http://127.0.0.1:5000/api/tool/my-dataplex-get-data-asset-tool/invoke",
			requestHeader:     map[string]string{},
			requestBody:       bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode:    200,
			expectResult:      true,
			wantLocationID:    "us",
			wantDataProductID: dataProductId,
			wantDataAssetID:   dataAssetId,
		},
		{
			name:           "Failure - Invalid Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-get-data-asset-tool/invoke",
			requestHeader:  map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:           "Failure - Without Authorization Token",
			api:            "http://127.0.0.1:5000/api/tool/my-auth-dataplex-get-data-asset-tool/invoke",
			requestHeader:  map[string]string{},
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf("{\"locationId\":\"us\",\"dataProductId\":\"%s\",\"dataAssetId\":\"%s\"}", dataProductId, dataAssetId))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}

			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &entry); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			locID, ok := entry["locationId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'locationId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantLocationID != "" && locID != tc.wantLocationID {
				t.Fatalf("expected locationId to be %q, got %q", tc.wantLocationID, locID)
			}
			prodID, ok := entry["dataProductId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'dataProductId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantDataProductID != "" && prodID != tc.wantDataProductID {
				t.Fatalf("expected dataProductId to be %q, got %q", tc.wantDataProductID, prodID)
			}
			assetID, ok := entry["dataAssetId"].(string)
			if !ok {
				t.Fatalf("expected entry to have key 'dataAssetId' as string, but it was not found or not a string in %v", entry)
			}
			if tc.wantDataAssetID != "" && assetID != tc.wantDataAssetID {
				t.Fatalf("expected dataAssetId to be %q, got %q", tc.wantDataAssetID, assetID)
			}

			// Assert output is cleaned
			if _, ok := entry["uid"]; ok {
				t.Errorf("expected entry to NOT have 'uid' field, but it was found")
			}
			if _, ok := entry["etag"]; ok {
				t.Errorf("expected entry to NOT have 'etag' field, but it was found")
			}
			if _, ok := entry["createTime"]; ok {
				t.Errorf("expected entry to NOT have 'createTime' field, but it was found")
			}
		})
	}
}

func runDataplexCreateAndUpdateDataProductToolsInvokeTest(t *testing.T, client *dataplex.DataProductClient, dataProductIdAuth string, dataProductIdUnauth string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	newDisplayNameAuth := dataProductIdAuth + "-updated-auth"
	newDescriptionAuth := "Updated description authorized"

	newDisplayNameUnauth := dataProductIdUnauth + "-updated-unauth"
	newDescriptionUnauth := "Updated description unauthorized"

	testCases := []struct {
		name                string
		api                 string
		requestHeader       map[string]string
		requestBody         io.Reader
		wantStatusCode      int
		expectResult        bool
		isUpdate            bool
		dataProductId       string
		expectedDisplayName string
		expectedDescription string
	}{
		{
			name:          "Success - Create Data Product (Authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-create-data-product-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s","description":"Temporary Data Product for create integration test","ownerEmails":["%s"],"accessGroups":[{"id":"test-group","displayName":"Test Group","description":"Test Group Desc","googleGroup":"%s"}]}`,
				dataProductIdAuth, dataProductIdAuth, tests.ServiceAccountEmail, tests.ServiceAccountEmail,
			))),
			wantStatusCode: 200,
			expectResult:   true,
			isUpdate:       false,
		},
		{
			name:          "Success - Create Data Product (Un-authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-dataplex-create-data-product-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s","description":"Temporary Data Product for create integration test","ownerEmails":["%s"],"accessGroups":[{"id":"test-group","displayName":"Test Group","description":"Test Group Desc","googleGroup":"%s"}]}`,
				dataProductIdUnauth, dataProductIdUnauth, tests.ServiceAccountEmail, tests.ServiceAccountEmail,
			))),
			wantStatusCode: 200,
			expectResult:   true,
			isUpdate:       false,
		},
		{
			name:          "Failure - Create Data Product Without Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-create-data-product-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s","ownerEmails":["%s"]}`,
				dataProductIdAuth, dataProductIdAuth, tests.ServiceAccountEmail,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Failure - Create Data Product With Invalid Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-create-data-product-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s","ownerEmails":["%s"]}`,
				dataProductIdAuth, dataProductIdAuth, tests.ServiceAccountEmail,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Success - Update Data Product (Authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-product-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s","description":"%s","ownerEmails":["different-owner@google.com"],"updateMask":["displayName","description"]}`,
				dataProductIdAuth, newDisplayNameAuth, newDescriptionAuth,
			))),
			wantStatusCode:      200,
			expectResult:        true,
			isUpdate:            true,
			dataProductId:       dataProductIdAuth,
			expectedDisplayName: newDisplayNameAuth,
			expectedDescription: newDescriptionAuth,
		},
		{
			name:          "Success - Update Data Product (Un-authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-dataplex-update-data-product-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s","description":"%s","ownerEmails":["different-owner-unauth@google.com"],"updateMask":["displayName","description"]}`,
				dataProductIdUnauth, newDisplayNameUnauth, newDescriptionUnauth,
			))),
			wantStatusCode:      200,
			expectResult:        true,
			isUpdate:            true,
			dataProductId:       dataProductIdUnauth,
			expectedDisplayName: newDisplayNameUnauth,
			expectedDescription: newDescriptionUnauth,
		},
		{
			name:          "Failure - Update Data Product Without Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-product-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s"}`,
				dataProductIdAuth, newDisplayNameAuth,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Failure - Update Data Product With Invalid Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-product-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","displayName":"%s"}`,
				dataProductIdAuth, newDisplayNameAuth,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}
			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var invokeResp map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &invokeResp); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			opId, ok := invokeResp["operationId"].(string)
			if !ok || opId == "" {
				t.Fatalf("expected 'operationId' in response, got %v", invokeResp)
			}
			locId, ok := invokeResp["locationId"].(string)
			if !ok || locId == "" {
				t.Fatalf("expected 'locationId' in response, got %v", invokeResp)
			}

			opName := fmt.Sprintf("projects/%s/locations/%s/operations/%s", DataplexProject, locId, opId)

			var completed bool
			for i := 0; i < 12; i++ {
				op, err := client.GetOperation(context.Background(), &longrunningpb.GetOperationRequest{Name: opName})
				if err == nil && op.GetDone() {
					if op.GetError() != nil {
						t.Fatalf("Data Product operation failed: %v", op.GetError())
					}
					completed = true
					break
				}
				time.Sleep(5 * time.Second)
			}
			if !completed {
				t.Fatalf("Data Product operation did not complete in time")
			}

			if tc.isUpdate {
				dpName := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s", DataplexProject, locId, tc.dataProductId)
				dp, err := client.GetDataProduct(context.Background(), &dataplexpb.GetDataProductRequest{Name: dpName})
				if err != nil {
					t.Fatalf("failed to retrieve updated Data Product %s: %v", dpName, err)
				}
				if dp.GetDisplayName() != tc.expectedDisplayName {
					t.Errorf("expected displayName to be %q, got %q", tc.expectedDisplayName, dp.GetDisplayName())
				}
				if dp.GetDescription() != tc.expectedDescription {
					t.Errorf("expected description to be %q, got %q", tc.expectedDescription, dp.GetDescription())
				}

				// Field "ownerEmails" (omitted from update mask) should not have changed
				if len(dp.GetOwnerEmails()) != 1 || dp.GetOwnerEmails()[0] != tests.ServiceAccountEmail {
					t.Errorf("expected owner emails to still be [%q], got %v", tests.ServiceAccountEmail, dp.GetOwnerEmails())
				}
			}
		})
	}
}

func runDataplexCreateAndUpdateDataAssetToolsInvokeTest(
	t *testing.T,
	client *dataplex.DataProductClient,
	dataProductId string,
	dataAssetIdAuth string,
	dataAssetIdUnauth string,
	datasetNameAuth string,
	tableNameAuth string,
	datasetNameUnauth string,
	tableNameUnauth string,
) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name               string
		api                string
		requestHeader      map[string]string
		requestBody        io.Reader
		wantStatusCode     int
		expectResult       bool
		isUpdate           bool
		dataAssetId        string
		expectedEnvLabel   string
		expectedViewerRole string
	}{
		{
			name:          "Success - Create Data Asset (Authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-create-data-asset-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","resourceUri":"//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s","labels":{"env":"test"},"accessGroupConfigs":{"test-group":["roles/bigquery.dataViewer"]}}`,
				dataProductId, dataAssetIdAuth, DataplexProject, datasetNameAuth, tableNameAuth,
			))),
			wantStatusCode: 200,
			expectResult:   true,
			isUpdate:       false,
		},
		{
			name:          "Success - Create Data Asset (Un-authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-dataplex-create-data-asset-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","resourceUri":"//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s","labels":{"env":"test"},"accessGroupConfigs":{"test-group":["roles/bigquery.dataViewer"]}}`,
				dataProductId, dataAssetIdUnauth, DataplexProject, datasetNameUnauth, tableNameUnauth,
			))),
			wantStatusCode: 200,
			expectResult:   true,
			isUpdate:       false,
		},
		{
			name:          "Failure - Create Data Asset Without Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-create-data-asset-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","resourceUri":"//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s"}`,
				dataProductId, dataAssetIdAuth, DataplexProject, datasetNameAuth, tableNameAuth,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Failure - Create Data Asset With Invalid Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-create-data-asset-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","resourceUri":"//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s"}`,
				dataProductId, dataAssetIdAuth, DataplexProject, datasetNameAuth, tableNameAuth,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Success - Update Data Asset (Authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-asset-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","labels":{"env":"prod"},"accessGroupConfigs":{"test-group":["roles/bigquery.dataViewer"]},"updateMask":["labels","accessGroupConfigs"]}`,
				dataProductId, dataAssetIdAuth,
			))),
			wantStatusCode:     200,
			expectResult:       true,
			isUpdate:           true,
			dataAssetId:        dataAssetIdAuth,
			expectedEnvLabel:   "prod",
			expectedViewerRole: "roles/bigquery.dataViewer",
		},
		{
			name:          "Success - Update Data Asset (Un-authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-dataplex-update-data-asset-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","labels":{"env":"prod-unauth"},"accessGroupConfigs":{"test-group":["roles/bigquery.metadataViewer"]},"updateMask":["labels"]}`,
				dataProductId, dataAssetIdUnauth,
			))),
			wantStatusCode:     200,
			expectResult:       true,
			isUpdate:           true,
			dataAssetId:        dataAssetIdUnauth,
			expectedEnvLabel:   "prod-unauth",
			expectedViewerRole: "roles/bigquery.dataViewer", // should remain dataViewer (omitted from updateMask)
		},
		{
			name:          "Failure - Update Data Asset Without Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-asset-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","labels":{"env":"prod"}}`,
				dataProductId, dataAssetIdAuth,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Failure - Update Data Asset With Invalid Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-asset-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","dataAssetId":"%s","labels":{"env":"prod"}}`,
				dataProductId, dataAssetIdAuth,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}

			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var invokeResp map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &invokeResp); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			opId, ok := invokeResp["operationId"].(string)
			if !ok || opId == "" {
				t.Fatalf("expected 'operationId' in response, got %v", invokeResp)
			}
			locId, ok := invokeResp["locationId"].(string)
			if !ok || locId == "" {
				t.Fatalf("expected 'locationId' in response, got %v", invokeResp)
			}

			opName := fmt.Sprintf("projects/%s/locations/%s/operations/%s", DataplexProject, locId, opId)

			var completed bool
			for i := 0; i < 12; i++ {
				op, err := client.GetOperation(context.Background(), &longrunningpb.GetOperationRequest{Name: opName})
				if err == nil && op.GetDone() {
					if op.GetError() != nil {
						t.Fatalf("Data Asset operation failed: %v", op.GetError())
					}
					completed = true
					break
				}
				time.Sleep(5 * time.Second)
			}
			if !completed {
				t.Fatalf("Data Asset operation did not complete in time")
			}

			if tc.isUpdate {
				daName := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s/dataAssets/%s", DataplexProject, locId, dataProductId, tc.dataAssetId)
				da, err := client.GetDataAsset(context.Background(), &dataplexpb.GetDataAssetRequest{Name: daName})
				if err != nil {
					t.Fatalf("failed to retrieve updated Data Asset %s: %v", daName, err)
				}

				if da.GetLabels()["env"] != tc.expectedEnvLabel {
					t.Errorf("expected label 'env' to be %q, got %q", tc.expectedEnvLabel, da.GetLabels()["env"])
				}

				agc := da.GetAccessGroupConfigs()
				if tc.expectedViewerRole != "" {
					if len(agc) != 1 || agc["test-group"] == nil {
						t.Errorf("expected accessGroupConfigs to have test-group, got %v", agc)
					} else {
						roles := agc["test-group"].GetIamRoles()
						if len(roles) != 1 || roles[0] != tc.expectedViewerRole {
							t.Errorf("expected test-group roles to be [%q], got %v", tc.expectedViewerRole, roles)
						}
					}
				} else {
					if len(agc) != 0 {
						t.Errorf("expected accessGroupConfigs to be empty, got %v", agc)
					}
				}
			}
		})
	}
}

func runDataplexEnrichmentToolInvokeTest(t *testing.T, tableName string, datasetName string, bucketName string, client *dataplex.DataScanClient) {
	ctx := context.Background()
	tableResource := fmt.Sprintf("//bigquery.googleapis.com/projects/%s/datasets/%s/tables/%s", DataplexProject, datasetName, tableName)
	bucketResource := fmt.Sprintf("//storage.googleapis.com/projects/%s/buckets/%s", DataplexProject, bucketName)

	testCases := []struct {
		name              string
		generateToolName  string
		generateReqBody   map[string]any
		getResultToolName string
	}{
		{
			name:             "Generate Data Profile Lifecycle",
			generateToolName: "my-dataplex-generate-data-profile-tool",
			generateReqBody: map[string]any{
				"resourcePath": tableResource,
				"location":     "us-central1",
				"publish":      false,
			},
			getResultToolName: "my-dataplex-get-data-profile-tool",
		},
		{
			name:             "Generate Data Insights Lifecycle",
			generateToolName: "my-dataplex-generate-data-insights-tool",
			generateReqBody: map[string]any{
				"resourcePath": tableResource,
				"location":     "us-central1",
				"publish":      false,
			},
			getResultToolName: "my-dataplex-get-data-insights-tool",
		},
		{
			name:             "Discover Metadata Lifecycle",
			generateToolName: "my-dataplex-discover-metadata-tool",
			generateReqBody: map[string]any{
				"resourcePath": bucketResource,
				"location":     "us-central1",
			},
			getResultToolName: "my-dataplex-get-discovery-results-tool",
		},
		{
			name:             "Check Data Quality Lifecycle",
			generateToolName: "my-dataplex-check-data-quality-tool",
			generateReqBody: map[string]any{
				"resourcePath": tableResource,
				"location":     "us-central1",
				"specJSON":     `{"rules": [{"column": "col1", "dimension": "COMPLETENESS", "nonNullExpectation": {}}]}`,
				"publish":      false,
			},
			getResultToolName: "my-dataplex-get-data-quality-results-tool",
		},
	}

	getOpURL := "http://127.0.0.1:5000/api/tool/my-dataplex-get-operation-tool/invoke"
	runStatusURL := "http://127.0.0.1:5000/api/tool/my-dataplex-get-run-status-tool/invoke"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Invoke Generate/Discover/Check Tool
			generateURL := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.generateToolName)
			reqBytes, _ := json.Marshal(tc.generateReqBody)
			resp, err := http.Post(generateURL, "application/json", bytes.NewBuffer(reqBytes))
			if err != nil {
				t.Fatalf("failed to invoke %s: %v", tc.generateToolName, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("%s response status got %d, want 200. Body: %s", tc.generateToolName, resp.StatusCode, string(body))
			}

			var invokeResult map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&invokeResult); err != nil {
				t.Fatalf("failed to decode response from %s: %v", tc.generateToolName, err)
			}
			resultStr, ok := invokeResult["result"].(string)
			if !ok || resultStr == "" {
				t.Fatalf("expected string result from %s, got: %v", tc.generateToolName, invokeResult)
			}

			var resMap map[string]string
			if err := json.Unmarshal([]byte(resultStr), &resMap); err != nil {
				t.Fatalf("failed to unmarshal generate tool result: %v. Raw result: %s", err, resultStr)
			}
			operationName := resMap["operation_id"]
			if operationName == "" {
				t.Fatalf("operation_id was empty in generate tool result: %s", resultStr)
			}
			if !strings.Contains(operationName, "/operations/") {
				t.Fatalf("expected operation name in result, got: %s", operationName)
			}

			// Step 2: Poll Operation Status
			opReqBody := map[string]any{"operationName": operationName}
			opReqBytes, _ := json.Marshal(opReqBody)

			var scanID string
			var opDone bool
			for i := 0; i < 90; i++ {
				opResp, err := http.Post(getOpURL, "application/json", bytes.NewBuffer(opReqBytes))
				if err != nil {
					t.Fatalf("failed to invoke get_operation: %v", err)
				}
				if opResp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(opResp.Body)
					opResp.Body.Close()
					t.Fatalf("get_operation response status got %d, want 200. Body: %s", opResp.StatusCode, string(body))
				}
				var opResult map[string]any
				if err := json.NewDecoder(opResp.Body).Decode(&opResult); err != nil {
					opResp.Body.Close()
					t.Fatalf("failed to decode response from get_operation: %v", err)
				}
				opResp.Body.Close()

				opResultStr, ok := opResult["result"].(string)
				if !ok {
					t.Fatalf("get_operation returned invalid result field: %v", opResult)
				}
				var innerOp map[string]any
				if err := json.Unmarshal([]byte(opResultStr), &innerOp); err != nil {
					t.Fatalf("failed to unmarshal inner operation details: %v", err)
				}

				if done, _ := innerOp["done"].(bool); done {
					opDone = true
					responseMap, ok := innerOp["response"].(map[string]any)
					if !ok {
						t.Fatalf("operation done but response missing or invalid: %s", opResultStr)
					}
					name, ok := responseMap["name"].(string)
					if !ok {
						t.Fatalf("DataScan response missing name: %v", responseMap)
					}
					parts := strings.Split(name, "/")
					scanID = parts[len(parts)-1]
					break
				}
				time.Sleep(5 * time.Second)
			}

			if !opDone {
				t.Fatalf("timed out waiting for scan template creation LRO to finish: %s", operationName)
			}
			if scanID == "" {
				t.Fatalf("scanID was empty after operation completed")
			}

			// Ensure DataScan is cleaned up from Dataplex
			defer func() {
				parent := fmt.Sprintf("projects/%s/locations/us-central1", DataplexProject)
				deleteReq := &dataplexpb.DeleteDataScanRequest{
					Name: fmt.Sprintf("%s/dataScans/%s", parent, scanID),
				}
				op, err := client.DeleteDataScan(ctx, deleteReq)
				if err != nil {
					t.Logf("cleanup: failed to delete scan template %s: %v", scanID, err)
					return
				}
				if err := op.Wait(ctx); err != nil {
					t.Logf("cleanup: wait for delete scan template %s failed: %v", scanID, err)
				}
				t.Logf("cleanup: successfully deleted scan template %s", scanID)
			}()

			// Step 3: Get Run Status
			runReqBody := map[string]any{"scanId": scanID, "location": "us-central1"}
			runReqBytes, _ := json.Marshal(runReqBody)
			runResp, err := http.Post(runStatusURL, "application/json", bytes.NewBuffer(runReqBytes))
			if err != nil {
				t.Fatalf("failed to invoke get_run_status: %v", err)
			}
			defer runResp.Body.Close()

			if runResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(runResp.Body)
				t.Fatalf("get_run_status response status got %d, want 200. Body: %s", runResp.StatusCode, string(body))
			}

			var runResult map[string]any
			if err := json.NewDecoder(runResp.Body).Decode(&runResult); err != nil {
				t.Fatalf("failed to decode response from get_run_status: %v", err)
			}
			runResultStr, _ := runResult["result"].(string)

			var jobStatus map[string]any
			if err := json.Unmarshal([]byte(runResultStr), &jobStatus); err != nil {
				t.Fatalf("get_run_status returned invalid JSON: %v. Raw: %s", err, runResultStr)
			}

			// Step 4: Get Profile / Insights / Discovery / Quality Results
			getResultURL := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.getResultToolName)
			resultReqBody := map[string]any{"scanId": scanID, "location": "us-central1"}
			resultReqBytes, _ := json.Marshal(resultReqBody)
			resultResp, err := http.Post(getResultURL, "application/json", bytes.NewBuffer(resultReqBytes))
			if err != nil {
				t.Fatalf("failed to invoke %s: %v", tc.getResultToolName, err)
			}
			defer resultResp.Body.Close()

			if resultResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resultResp.Body)
				t.Fatalf("%s response status got %d, want 200. Body: %s", tc.getResultToolName, resultResp.StatusCode, string(body))
			}

			var innerResult map[string]any
			if err := json.NewDecoder(resultResp.Body).Decode(&innerResult); err != nil {
				t.Fatalf("failed to decode response from %s: %v", tc.getResultToolName, err)
			}
			innerResultStr, _ := innerResult["result"].(string)

			var scanData map[string]any
			if err := json.Unmarshal([]byte(innerResultStr), &scanData); err != nil {
				t.Fatalf("%s returned invalid JSON: %v. Raw: %s", tc.getResultToolName, err, innerResultStr)
			}

			expectedScanName := fmt.Sprintf("projects/%s/locations/us-central1/dataScans/%s", DataplexProject, scanID)
			if scanData["name"] != expectedScanName {
				t.Fatalf("expected scan name %s, got: %s", expectedScanName, scanData["name"])
			}
		})
	}
}

func runDataplexUpdateDataProductAspectsToolInvokeTest(t *testing.T, dataProductId string, aspectTypeId string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	testCases := []struct {
		name                 string
		api                  string
		requestHeader        map[string]string
		requestBody          io.Reader
		wantStatusCode       int
		expectResult         bool
		expectedAspectTypeId string
		expectedAspectValue  string
	}{
		{
			name:          "Success - Update Data Product Aspects (Authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-product-aspects-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","aspects":[{"projectId":"dataplex-types","locationId":"global","aspectTypeId":"overview","data":{"content":"Auth updated description"}}]}`,
				dataProductId,
			))),
			wantStatusCode:       200,
			expectResult:         true,
			expectedAspectTypeId: "overview",
			expectedAspectValue:  "Auth updated description",
		},
		{
			name:          "Success - Update Data Product Aspects (Un-authorized)",
			api:           "http://127.0.0.1:5000/api/tool/my-dataplex-update-data-product-aspects-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","aspects":[{"projectId":"%s","locationId":"us","aspectTypeId":"%s","data":{}}]}`,
				dataProductId, DataplexProject, aspectTypeId,
			))),
			wantStatusCode:       200,
			expectResult:         true,
			expectedAspectTypeId: aspectTypeId,
			expectedAspectValue:  "",
		},
		{
			name:          "Failure - Without Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-product-aspects-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","aspects":[{"projectId":"dataplex-types","locationId":"global","aspectTypeId":"overview","data":{"content":"Unauth"}}]}`,
				dataProductId,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
		{
			name:          "Failure - Invalid Authorization Token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-dataplex-update-data-product-aspects-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "invalid_token"},
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"locationId":"us","dataProductId":"%s","aspects":[{"projectId":"dataplex-types","locationId":"global","aspectTypeId":"overview","data":{"content":"Invalid"}}]}`,
				dataProductId,
			))),
			wantStatusCode: 401,
			expectResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code got %d, want %d\nResponse body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}
			if !tc.expectResult {
				return
			}

			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}
			var invokeResp map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &invokeResp); err != nil {
				t.Fatalf("error unmarshalling result string: %v", err)
			}

			aspectsList, ok := invokeResp["aspects"].([]interface{})
			if !ok {
				t.Fatalf("expected 'aspects' field to be an array, got %T. Full response: %s", invokeResp["aspects"], resultStr)
			}

			var foundAspect bool
			for _, rawAspect := range aspectsList {
				aspect, ok := rawAspect.(map[string]interface{})
				if !ok {
					continue
				}
				typeId, _ := aspect["aspectTypeId"].(string)
				if typeId == tc.expectedAspectTypeId {
					projId, _ := aspect["projectId"].(string)
					locId, _ := aspect["locationId"].(string)
					data, _ := aspect["data"].(map[string]interface{})

					if typeId == "overview" {
						if data != nil {
							desc, _ := data["content"].(string)
							if desc == tc.expectedAspectValue {
								if projId != "dataplex-types" {
									t.Errorf("expected aspect projectId to be 'dataplex-types', got %q", projId)
								}
								if locId != "global" {
									t.Errorf("expected aspect locationId to be 'global', got %q", locId)
								}
								foundAspect = true
								break
							}
						}
					} else {
						// Custom aspect type validations
						if projId != DataplexProject {
							t.Errorf("expected aspect projectId to be %q, got %q", DataplexProject, projId)
						}
						if locId != "us" {
							t.Errorf("expected aspect locationId to be 'us', got %q", locId)
						}
						foundAspect = true
						break
					}
				}
			}

			if !foundAspect {
				t.Fatalf("expected aspect %q with value %q in returned aspects, but not found. Response: %v", tc.expectedAspectTypeId, tc.expectedAspectValue, invokeResp)
			}

			entryType, _ := invokeResp["entryType"].(string)
			if entryType == "" {
				t.Errorf("expected non-empty 'entryType' in response. Response: %v", invokeResp)
			}
			if !strings.Contains(entryType, "/entryTypes/") {
				t.Errorf("expected 'entryType' to contain '/entryTypes/', got %q", entryType)
			}

			if _, ok := invokeResp["entrySource"]; !ok {
				t.Errorf("expected 'entrySource' key in response. Response: %v", invokeResp)
			}
		})
	}
}
