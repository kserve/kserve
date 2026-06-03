/*
Copyright 2025 The KServe Authors.

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

package kernelcachenode

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type FileSystemInterface interface {
	removeCache(storageKey string) error
	hasCacheFolder(storageKey string) (bool, error)
	getCacheFolders() ([]os.DirEntry, error)
	ensureCacheRootFolderExists() error
}

type FileSystemHelper struct {
	cachesRootFolder string
}

func NewFileSystemHelper(cachesRootFolder string) *FileSystemHelper {
	return &FileSystemHelper{
		cachesRootFolder: cachesRootFolder,
	}
}

// getCacheFolder returns the full path to a cache directory
// cachesRootFolder: /mnt/kernel-cache/kernel-cache
// storageKey: hash of image URI
// returns: /mnt/kernel-cache/kernel-cache/<hash>
func getCacheFolder(rootFolderName string, storageKey string) string {
	return filepath.Join(rootFolderName, storageKey)
}

func (f *FileSystemHelper) removeCache(storageKey string) error {
	path := getCacheFolder(f.cachesRootFolder, storageKey)
	return os.RemoveAll(path)
}

func (f *FileSystemHelper) getCacheFolders() ([]os.DirEntry, error) {
	entries, err := os.ReadDir(f.cachesRootFolder)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}

func (f *FileSystemHelper) hasCacheFolder(storageKey string) (bool, error) {
	folder := getCacheFolder(f.cachesRootFolder, storageKey)
	_, err := os.ReadDir(folder)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (f *FileSystemHelper) ensureCacheRootFolderExists() error {
	// If the folder already exists, this will do nothing
	if err := os.MkdirAll(f.cachesRootFolder, os.ModePerm); err != nil { //nolint:gosec // G301: kernel cache must be readable by model server running as a different UID
		return err
	}
	return nil
}
