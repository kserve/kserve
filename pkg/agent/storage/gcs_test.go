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

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kserve/kserve/pkg/agent/mocks"
)

func writeGCSObject(t *testing.T, provider *GCSProvider, bucketName string, objectName string, contents string) {
	t.Helper()

	writer := provider.Client.Bucket(bucketName).Object(objectName).NewWriter(context.Background())
	if _, err := writer.Write([]byte(contents)); err != nil {
		t.Fatalf("failed to write object %q: %v", objectName, err)
	}
}

func newTestGCSProvider(t *testing.T, bucketName string) *GCSProvider {
	t.Helper()

	client := mocks.NewMockClient()
	if err := client.Bucket(bucketName).Create(context.Background(), "test", nil); err != nil {
		t.Fatalf("failed to create bucket %q: %v", bucketName, err)
	}
	return &GCSProvider{Client: client}
}

func TestGCSDownloadAllowsNestedObjectPath(t *testing.T) {
	const (
		bucketName    = "testBucket"
		modelName     = "model1"
		modelContents = "Model Contents"
	)

	provider := newTestGCSProvider(t, bucketName)
	writeGCSObject(t, provider, bucketName, "models/nested/model.bin", modelContents)

	modelDir := t.TempDir()
	if err := provider.DownloadModel(modelDir, modelName, "gs://testBucket/models"); err != nil {
		t.Fatalf("expected download to succeed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelDir, modelName, "nested", "model.bin"))
	if err != nil {
		t.Fatalf("failed to read downloaded model: %v", err)
	}
	if string(got) != modelContents {
		t.Fatalf("downloaded contents = %q, want %q", string(got), modelContents)
	}
}

func TestGCSDownloadRejectsPathTraversal(t *testing.T) {
	const (
		bucketName       = "testBucket"
		modelName        = "model1"
		originalContents = "do not overwrite"
	)

	tmpDir := t.TempDir()
	outsidePath := filepath.Join(tmpDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte(originalContents), 0o644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	provider := newTestGCSProvider(t, bucketName)
	writeGCSObject(t, provider, bucketName, "models/../../outside.txt", "malicious")

	modelDir := filepath.Join(tmpDir, "models")
	if err := provider.DownloadModel(modelDir, modelName, "gs://testBucket/models"); err == nil {
		t.Fatal("expected path traversal object to be rejected")
	}

	got, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatalf("failed to read outside file: %v", err)
	}
	if string(got) != originalContents {
		t.Fatalf("outside file contents = %q, want %q", string(got), originalContents)
	}
}
