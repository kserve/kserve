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
	"io/ioutil"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/onsi/gomega"
)

func TestCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// This would get called in StartPullerAndProcessModels
	syscall.Umask(0)

	tmpDir, _ := ioutil.TempDir("", "test-create-")
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
