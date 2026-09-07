/*
Copyright 2024 The KServe Authors.

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
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"syscall"

	"k8s.io/apimachinery/pkg/api/resource"
)

type FileSystemInterface interface {
	removeModel(modelName string) error
	hasModelFolder(modelName string) (bool, error)
	getModelFolders() ([]os.DirEntry, error)
	getStorageUsage() (resource.Quantity, resource.Quantity, error)
	ensureModelRootFolderExists() error
}

type FileSystemHelper struct {
	modelsRootFolder string
}

func NewFileSystemHelper(modelsRootFolder string) *FileSystemHelper {
	return &FileSystemHelper{
		modelsRootFolder: modelsRootFolder,
	}
}

// should be used only in this struct
func getModelFolder(rootFolderName string, modelName string) string {
	return filepath.Join(rootFolderName, modelName)
}

func (f *FileSystemHelper) removeModel(modelName string) error {
	path := getModelFolder(f.modelsRootFolder, modelName)
	return os.RemoveAll(path)
}

func (f *FileSystemHelper) getModelFolders() ([]os.DirEntry, error) {
	entries, err := os.ReadDir(f.modelsRootFolder)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}

func (f *FileSystemHelper) getStorageUsage() (resource.Quantity, resource.Quantity, error) {
	var used int64
	if err := filepath.WalkDir(f.modelsRootFolder, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		used += info.Size()
		return nil
	}); err != nil {
		return resource.Quantity{}, resource.Quantity{}, err
	}

	var stats syscall.Statfs_t
	if err := syscall.Statfs(f.modelsRootFolder, &stats); err != nil {
		return resource.Quantity{}, resource.Quantity{}, err
	}
	if stats.Bsize <= 0 {
		return resource.Quantity{}, resource.Quantity{}, fmt.Errorf("invalid filesystem block size: %d", stats.Bsize)
	}
	blockSize := uint64(stats.Bsize)
	if stats.Bavail > uint64(math.MaxInt64)/blockSize {
		return resource.Quantity{}, resource.Quantity{}, errors.New("available filesystem storage exceeds supported quantity")
	}
	available := int64(stats.Bavail * blockSize) //nolint:gosec // G115: bounds check above ensures the value fits int64.
	return *resource.NewQuantity(used, resource.BinarySI), *resource.NewQuantity(available, resource.BinarySI), nil
}

func (f *FileSystemHelper) hasModelFolder(modelName string) (bool, error) {
	folder := getModelFolder(f.modelsRootFolder, modelName)
	_, err := os.ReadDir(folder)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (f *FileSystemHelper) ensureModelRootFolderExists() error {
	// If the folder already exists, this will do nothing
	if err := os.MkdirAll(f.modelsRootFolder, os.ModePerm); err != nil { //nolint:gosec // G301: local model cache must be readable by model server running as a different UID
		return err
	}
	return nil
}
