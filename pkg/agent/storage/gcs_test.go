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
	"syscall"
	"testing"

	"github.com/kserve/kserve/pkg/agent/mocks"
)

func writeGCSObject(t *testing.T, provider *GCSProvider, objectName string, contents string) {
	t.Helper()

	writer := provider.Client.Bucket("testBucket").Object(objectName).NewWriter(context.Background())
	if _, err := writer.Write([]byte(contents)); err != nil {
		t.Fatalf("failed to write object %q: %v", objectName, err)
	}
}

func newTestGCSProvider(t *testing.T) *GCSProvider {
	t.Helper()

	client := mocks.NewMockClient()
	if err := client.Bucket("testBucket").Create(context.Background(), "test", nil); err != nil {
		t.Fatalf("failed to create test bucket: %v", err)
	}
	return &GCSProvider{Client: client}
}

func TestGCSDownloadAllowsNestedObjectPath(t *testing.T) {
	const (
		modelName     = "model1"
		modelContents = "Model Contents"
	)

	provider := newTestGCSProvider(t)
	writeGCSObject(t, provider, "models/nested/model.bin", modelContents)

	modelDir := t.TempDir()
	if err := provider.DownloadModel(modelDir, modelName, "gs://testBucket/models"); err != nil {
		t.Fatalf("expected download to succeed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelDir, modelName, "nested", "model.bin")) //nolint:gosec // G304: test path is rooted in t.TempDir
	if err != nil {
		t.Fatalf("failed to read downloaded model: %v", err)
	}
	if string(got) != modelContents {
		t.Fatalf("downloaded contents = %q, want %q", string(got), modelContents)
	}
}

func TestGCSDownloadRejectsPathTraversal(t *testing.T) {
	const (
		modelName        = "model1"
		originalContents = "do not overwrite"
	)

	tmpDir := t.TempDir()
	outsidePath := filepath.Join(tmpDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte(originalContents), 0o600); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	provider := newTestGCSProvider(t)
	writeGCSObject(t, provider, "models/../../outside.txt", "malicious")

	modelDir := filepath.Join(tmpDir, "models")
	if err := provider.DownloadModel(modelDir, modelName, "gs://testBucket/models"); err == nil {
		t.Fatal("expected path traversal object to be rejected")
	}

	got, err := os.ReadFile(outsidePath) //nolint:gosec // G304: test path is rooted in t.TempDir
	if err != nil {
		t.Fatalf("failed to read outside file: %v", err)
	}
	if string(got) != originalContents {
		t.Fatalf("outside file contents = %q, want %q", string(got), originalContents)
	}
}

func TestGCSDownloadAllowsEmptyModelName(t *testing.T) {
	const (
		modelContents = "Model Contents"
	)

	provider := newTestGCSProvider(t)
	writeGCSObject(t, provider, "nested/", "")
	writeGCSObject(t, provider, "nested/model.bin", modelContents)

	modelDir := t.TempDir()
	if err := provider.DownloadModel(modelDir, "", "gs://testBucket/"); err != nil {
		t.Fatalf("expected whole-bucket download to succeed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelDir, "nested", "model.bin")) //nolint:gosec // G304: test path is rooted in t.TempDir
	if err != nil {
		t.Fatalf("failed to read downloaded model: %v", err)
	}
	if string(got) != modelContents {
		t.Fatalf("downloaded contents = %q, want %q", string(got), modelContents)
	}
}

func TestGCSDownloadWithEmptyModelNameRejectsPathTraversal(t *testing.T) {
	const (
		originalContents = "do not overwrite"
	)

	tmpDir := t.TempDir()
	outsidePath := filepath.Join(tmpDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte(originalContents), 0o600); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	provider := newTestGCSProvider(t)
	writeGCSObject(t, provider, "../../outside.txt", "malicious")

	modelDir := filepath.Join(tmpDir, "models")
	if err := provider.DownloadModel(modelDir, "", "gs://testBucket/"); err == nil {
		t.Fatal("expected whole-bucket traversal object to be rejected")
	}

	got, err := os.ReadFile(outsidePath) //nolint:gosec // G304: test path is rooted in t.TempDir
	if err != nil {
		t.Fatalf("failed to read outside file: %v", err)
	}
	if string(got) != originalContents {
		t.Fatalf("outside file contents = %q, want %q", string(got), originalContents)
	}
}

func TestGCSDownloadAllowsExactObjectPath(t *testing.T) {
	const (
		modelName     = "model1"
		modelContents = "Model Contents"
	)

	provider := newTestGCSProvider(t)
	writeGCSObject(t, provider, "models/model.bin", modelContents)

	modelDir := t.TempDir()
	if err := provider.DownloadModel(modelDir, modelName, "gs://testBucket/models/model.bin"); err != nil {
		t.Fatalf("expected exact-object download to succeed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelDir, modelName, "model.bin")) //nolint:gosec // G304: test path is rooted in t.TempDir
	if err != nil {
		t.Fatalf("failed to read downloaded model: %v", err)
	}
	if string(got) != modelContents {
		t.Fatalf("downloaded contents = %q, want %q", string(got), modelContents)
	}
}

func TestGCSDownloadReplacesExistingFilePermissions(t *testing.T) {
	previousUmask := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	const (
		modelName     = "model1"
		modelContents = "Model Contents"
	)

	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "existing0640", mode: 0o640},
		{name: "existing0444", mode: 0o444},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := newTestGCSProvider(t)
			writeGCSObject(t, provider, "models/model.bin", modelContents)

			modelDir := t.TempDir()
			modelPath := filepath.Join(modelDir, modelName)
			if err := os.Mkdir(modelPath, 0o750); err != nil {
				t.Fatalf("failed to create model directory: %v", err)
			}
			fileName := filepath.Join(modelPath, "model.bin")
			if err := os.WriteFile(fileName, []byte("corrupted model contents"), tc.mode); err != nil {
				t.Fatalf("failed to create existing model file: %v", err)
			}

			if err := provider.DownloadModel(modelDir, modelName, "gs://testBucket/models"); err != nil {
				t.Fatalf("expected download to replace existing model file: %v", err)
			}

			got, err := os.ReadFile(fileName) //nolint:gosec // G304: test path is rooted in t.TempDir
			if err != nil {
				t.Fatalf("failed to read downloaded model: %v", err)
			}
			if string(got) != modelContents {
				t.Fatalf("downloaded contents = %q, want %q", string(got), modelContents)
			}
			info, err := os.Stat(fileName)
			if err != nil {
				t.Fatalf("failed to stat downloaded model: %v", err)
			}
			if got := info.Mode().Perm(); got != 0o666 {
				t.Fatalf("downloaded file permissions = %04o, want 0666", got)
			}
		})
	}
}
