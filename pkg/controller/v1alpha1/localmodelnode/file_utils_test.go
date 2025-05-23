/*
Copyright 2023 The KServe Authors.

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

package localmodelnode

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFileSystemHelper_hasModelFolder tests the hasModelFolder method.
func TestFileSystemHelper_hasModelFolder(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	helper := NewFileSystemHelper(tempDir)

	// Case 1: Model folder does not exist
	exists, err := helper.hasModelFolder("nonexistent-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Errorf("expected model folder to not exist, got exists=true")
	}

	// Case 2: Model folder exists
	modelName := "test-model"
	modelPath := filepath.Join(tempDir, modelName)
	if err := os.Mkdir(modelPath, 0o755); err != nil {
		t.Fatalf("failed to create model folder: %v", err)
	}
	exists, err = helper.hasModelFolder(modelName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Errorf("expected model folder to exist, got exists=false")
	}

	// Case 3: Error other than not-exist (simulate permission denied)
	// Remove permission from the root folder
	if err := os.Chmod(tempDir, 0o000); err != nil {
		t.Fatalf("failed to chmod tempDir: %v", err)
	}
	defer os.Chmod(tempDir, 0o755) //nolint

	_, err = helper.hasModelFolder("any-model")
	if err == nil {
		t.Errorf("expected error due to permission denied, got nil")
	}
}

func TestFileSystemHelper_removeModel(t *testing.T) {
	tempDir := t.TempDir()
	helper := NewFileSystemHelper(tempDir)

	// Case 1: Remove non-existent model folder (should not error)
	err := helper.removeModel("nonexistent-model")
	if err != nil {
		t.Errorf("expected no error when removing non-existent model, got: %v", err)
	}

	// Case 2: Remove existing model folder
	modelName := "test-model"
	modelPath := filepath.Join(tempDir, modelName)
	if err := os.Mkdir(modelPath, 0o755); err != nil {
		t.Fatalf("failed to create model folder: %v", err)
	}
	// Create a file inside the model folder
	filePath := filepath.Join(modelPath, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil { //nolint
		t.Fatalf("failed to create file in model folder: %v", err)
	}
	err = helper.removeModel(modelName)
	if err != nil {
		t.Errorf("expected no error when removing existing model, got: %v", err)
	}
	// Ensure the folder is gone
	_, statErr := os.Stat(modelPath)
	if !os.IsNotExist(statErr) {
		t.Errorf("expected model folder to be removed, statErr: %v", statErr)
	}

	// Case 3: Remove model folder when lacking permissions
	permModel := "perm-model"
	permModelPath := filepath.Join(tempDir, permModel)
	if err := os.Mkdir(permModelPath, 0o755); err != nil {
		t.Fatalf("failed to create perm-model folder: %v", err)
	}
	// Remove write permission from parent dir
	if err := os.Chmod(tempDir, 0o555); err != nil { //nolint
		t.Fatalf("failed to chmod tempDir: %v", err)
	}
	defer os.Chmod(tempDir, 0o755) //nolint
	err = helper.removeModel(permModel)
	if err == nil {
		t.Errorf("expected error due to permission denied, got nil")
	}
}

func TestFileSystemHelper_ensureModelRootFolderExists(t *testing.T) {
	// Case 1: Folder does not exist, should be created
	tempDir := t.TempDir()
	modelsRoot := filepath.Join(tempDir, "models")
	helper := NewFileSystemHelper(modelsRoot)

	err := helper.ensureModelRootFolderExists()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	info, statErr := os.Stat(modelsRoot)
	if statErr != nil {
		t.Fatalf("expected modelsRoot folder to exist, statErr: %v", statErr)
	}
	if !info.IsDir() {
		t.Errorf("expected modelsRoot to be a directory")
	}

	// Case 2: Folder already exists, should not error
	err = helper.ensureModelRootFolderExists()
	if err != nil {
		t.Errorf("expected no error when folder already exists, got: %v", err)
	}

	// Case 3: Parent directory is not writable (simulate permission denied)
	permDir := filepath.Join(tempDir, "perm")
	if err := os.Mkdir(permDir, 0o555); err != nil {
		t.Fatalf("failed to create permDir: %v", err)
	}
	defer os.Chmod(permDir, 0o755) //nolint
	unwritableRoot := filepath.Join(permDir, "models")
	helper2 := NewFileSystemHelper(unwritableRoot)
	err = helper2.ensureModelRootFolderExists()
	if err == nil {
		t.Errorf("expected error due to permission denied, got nil")
	}
}
