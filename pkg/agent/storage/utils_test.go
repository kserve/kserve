/*
Copyright 2021 The KServe Authors.

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
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kserve/kserve/pkg/agent/mocks"
	"github.com/onsi/gomega"
)

func TestCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// This would get called in StartPullerAndProcessModels
	syscall.Umask(0)

	tmpDir, _ := os.MkdirTemp("", "test-create-")
	defer os.RemoveAll(tmpDir)

	folderPath := path.Join(tmpDir, "foo")
	filePath := path.Join(folderPath, "bar.txt")
	f, err := Create(filePath)
	defer f.Close()

	g.Expect(err).To(gomega.BeNil())
	g.Expect(folderPath).To(gomega.BeADirectory())

	info, _ := os.Stat(folderPath)
	mode := info.Mode()
	expectedMode := os.FileMode(0777)
	g.Expect(mode.Perm()).To(gomega.Equal(expectedMode))
}

func TestFileExists(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	syscall.Umask(0)
	tmpDir, _ := os.MkdirTemp("", "test")
	defer os.RemoveAll(tmpDir)

	// Test case for existing file
	f, err := os.CreateTemp(tmpDir, "tmpfile")
	g.Expect(err).To(gomega.BeNil())
	g.Expect(FileExists(f.Name())).To(gomega.BeTrue())
	f.Close()

	// Test case for not existing file
	path := filepath.Join(tmpDir, "fileNotExist")
	g.Expect(FileExists(path)).To(gomega.BeFalse())

	// Test case for directory as filename
	g.Expect(FileExists(tmpDir)).To(gomega.BeFalse())
}

func TestRemoveDir(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	syscall.Umask(0)
	tmpDir, _ := os.MkdirTemp("", "test")
	subDir, _ := os.MkdirTemp(tmpDir, "test")
	os.CreateTemp(subDir, "tmp")
	os.CreateTemp(tmpDir, "tmp")

	err := RemoveDir(tmpDir)
	g.Expect(err).To(gomega.BeNil())
	_, err = os.Stat(tmpDir)
	g.Expect(os.IsNotExist(err)).To(gomega.BeTrue())

	// Test case for non existing directory
	err = RemoveDir("directoryNotExist")
	g.Expect(err).NotTo(gomega.BeNil())
}

func TestGetProvider(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// When providers map already have specified provider
	mockProviders := map[Protocol]Provider{
		S3: &S3Provider{
			Client:     &mocks.MockS3Client{},
			Downloader: &mocks.MockS3Downloader{},
		},
	}
	provider, err := GetProvider(mockProviders, S3)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(provider).Should(gomega.Equal(mockProviders[S3]))

	// When providers map does not have specified provider
	for _, protocol := range SupportedProtocols {
		provider, err = GetProvider(map[Protocol]Provider{}, protocol)
		g.Expect(err).To(gomega.BeNil())
		g.Expect(provider).ShouldNot(gomega.BeNil())
	}
}
