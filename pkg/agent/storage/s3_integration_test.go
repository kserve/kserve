/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//go:build integration

// Integration tests for S3Provider using a local S3-compatible server.
//
// These tests exercise the real S3Provider code paths (UploadObject, DownloadModel)
// against a local S3-compatible backend to verify the aws-sdk-go-v2 transfermanager
// integration works end-to-end.
//
// Prerequisites:
//
// Start a local S3-compatible server on port 9000 with access key "minioadmin"
// and secret key "minioadmin". Any S3-compatible server will work.
//
// Option 1: SeaweedFS (lightweight, single binary)
//
//	# Using a container runtime (podman or docker):
//	podman run -d --name s3-test -p 9000:8333 chrislusf/seaweedfs:latest \
//	  server -s3 -s3.config=/dev/null -s3.port=8333
//
//	# Or with the binary directly:
//	weed server -s3 -s3.port=9000
//
//	Note: SeaweedFS uses "minioadmin"/"minioadmin" as default S3 credentials when
//	no config is provided. If your version requires explicit config, create a
//	s3.json with:
//	  {"identities":[{"name":"admin","credentials":[{"accessKey":"minioadmin","secretKey":"minioadmin"}],"actions":["Admin","Read","Write","List","Tagging"]}]}
//
// Option 2: MinIO
//
//	podman run -d --name s3-test -p 9000:9000 \
//	  -e MINIO_ROOT_USER=minioadmin \
//	  -e MINIO_ROOT_PASSWORD=minioadmin \
//	  quay.io/minio/minio:latest server /data
//
// Run the tests:
//
//	go test ./pkg/agent/storage/ -tags=integration -run TestS3ProviderIntegration -v
//
// Clean up:
//
//	podman stop s3-test && podman rm s3-test

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	testEndpoint  = "http://localhost:9000"
	testAccessKey = "minioadmin"
	testSecretKey = "minioadmin"
	testBucket    = "test-kserve-bucket"
	testRegion    = "us-east-1"
)

func setupMinioClient(t *testing.T) *s3.Client {
	t.Helper()
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(testRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(testAccessKey, testSecretKey, "")),
	)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(testEndpoint)
		o.UsePathStyle = true
	})

	// Create test bucket
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(testBucket),
	})
	if err != nil {
		// Ignore if bucket already exists
		t.Logf("CreateBucket (may already exist): %v", err)
	}

	return client
}

func TestS3ProviderIntegration_UploadAndDownload(t *testing.T) {
	client := setupMinioClient(t)

	provider := &S3Provider{
		Client:         client,
		TransferClient: transfermanager.New(client),
	}

	// 1. Upload an object
	testContent := []byte("this is a test model file content")
	err := provider.UploadObject(testBucket, "models/model1/model.pt", testContent)
	if err != nil {
		t.Fatalf("UploadObject failed: %v", err)
	}
	t.Log("Upload succeeded")

	// 2. Verify the object exists via ListObjectsV2
	ctx := context.Background()
	listResp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(testBucket),
		Prefix: aws.String("models/model1/"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 failed: %v", err)
	}
	if len(listResp.Contents) == 0 {
		t.Fatal("Expected at least one object, got none")
	}
	t.Logf("Listed %d object(s) under models/model1/", len(listResp.Contents))
	for _, obj := range listResp.Contents {
		t.Logf("  - %s", *obj.Key)
	}

	// 3. Download using the real S3Provider.DownloadModel path
	modelDir := t.TempDir()
	err = provider.DownloadModel(modelDir, "model1", "s3://"+testBucket+"/models/model1/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}
	t.Log("Download succeeded")

	// 4. Verify the downloaded file content
	downloadedFile := filepath.Join(modelDir, "model1", "model.pt")
	data, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(data) != string(testContent) {
		t.Fatalf("Content mismatch:\n  got:      %q\n  expected: %q", string(data), string(testContent))
	}
	t.Log("Content verified - matches uploaded data")

	// 5. Clean up: delete the object and bucket
	_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String("models/model1/model.pt"),
	})
}

func TestS3ProviderIntegration_DownloadMultipleObjects(t *testing.T) {
	client := setupMinioClient(t)

	provider := &S3Provider{
		Client:         client,
		TransferClient: transfermanager.New(client),
	}

	// Upload multiple objects
	files := map[string]string{
		"multi-model/config.json": `{"model_type": "sklearn"}`,
		"multi-model/weights.bin": "fake-binary-weights-data-here",
		"multi-model/vocab.txt":   "hello\nworld\nfoo\nbar",
	}

	for key, content := range files {
		if err := provider.UploadObject(testBucket, key, []byte(content)); err != nil {
			t.Fatalf("UploadObject(%s) failed: %v", key, err)
		}
	}
	t.Logf("Uploaded %d objects", len(files))

	// Download all objects via DownloadModel
	modelDir := t.TempDir()
	err := provider.DownloadModel(modelDir, "mymodel", "s3://"+testBucket+"/multi-model/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	// Verify each file
	for key, expectedContent := range files {
		relPath := filepath.Base(key)
		filePath := filepath.Join(modelDir, "mymodel", relPath)
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", filePath, err)
		}
		if string(data) != expectedContent {
			t.Fatalf("Content mismatch for %s:\n  got:      %q\n  expected: %q", relPath, string(data), expectedContent)
		}
		t.Logf("Verified %s", relPath)
	}

	// Clean up
	ctx := context.Background()
	for key := range files {
		_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(key),
		})
	}
}

func TestS3ProviderIntegration_DownloadNonExistentBucket(t *testing.T) {
	client := setupMinioClient(t)

	provider := &S3Provider{
		Client:         client,
		TransferClient: transfermanager.New(client),
	}

	modelDir := t.TempDir()
	err := provider.DownloadModel(modelDir, "model1", "s3://nonexistent-bucket-xyz/model/")
	if err == nil {
		t.Fatal("Expected error for nonexistent bucket, got nil")
	}
	t.Logf("Got expected error: %v", err)
}

func TestS3ProviderIntegration_DownloadEmptyPrefix(t *testing.T) {
	client := setupMinioClient(t)
	ctx := context.Background()

	// Create a dedicated empty bucket
	emptyBucket := "test-empty-prefix-bucket"
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(emptyBucket),
	})
	if err != nil {
		t.Logf("CreateBucket (may already exist): %v", err)
	}

	provider := &S3Provider{
		Client:         client,
		TransferClient: transfermanager.New(client),
	}

	modelDir := t.TempDir()
	err = provider.DownloadModel(modelDir, "model1", "s3://"+emptyBucket+"/nonexistent-prefix/")
	if err == nil {
		t.Fatal("Expected error for empty prefix, got nil")
	}
	t.Logf("Got expected error: %v", err)

	// Clean up
	_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(emptyBucket),
	})
}