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
	"os"
	"path/filepath"
)

type FileSystemInterface interface {
	removeModel(modelName string) error
	hasModelFolder(modelName string) (bool, error)
	getModelFolders() ([]os.DirEntry, error)
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
	return os.ReadDir(f.modelsRootFolder)
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
	if err := os.MkdirAll(f.modelsRootFolder, os.ModePerm); err != nil {
		return err
	}
	return nil
}
