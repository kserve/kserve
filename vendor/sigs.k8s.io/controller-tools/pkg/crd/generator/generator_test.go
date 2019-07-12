/*
Copyright 2018 The Kubernetes Authors.

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

package generator_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/afero"
	crdgenerator "sigs.k8s.io/controller-tools/pkg/crd/generator"
)

func TestGenerator(t *testing.T) {
	currDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("unable to get current directory: %v", err)
	}
	// in-memory file system for storing the generated CRDs
	outFs := afero.NewMemMapFs()
	g := &crdgenerator.Generator{
		OutFs:     outFs,
		OutputDir: "/tmp",
		RootPath:  filepath.Join(currDir, "testData"),
	}
	err = g.ValidateAndInitFields()
	if err != nil {
		t.Fatalf("generator validate should have succeeded %v", err)
	}

	err = g.Do()
	if err != nil {
		t.Fatalf("generator validate should have succeeded %v", err)
	}
	for _, f := range []string{"fun_v1alpha1_toy.yaml"} {
		crdOuputFile := filepath.Join("/tmp", f)
		crdContent, err := afero.ReadFile(outFs, crdOuputFile)
		if err != nil {
			t.Fatalf("reading file failed %v", err)
		}
		expectedContent, err := ioutil.ReadFile(filepath.Join(currDir, "testData", "config", "crds", f))
		if err != nil {
			t.Fatalf("reading file failed %v", err)
		}
		if !reflect.DeepEqual(crdContent, expectedContent) {
			t.Fatalf("CRD output does not match exp:%v got:%v \n", string(expectedContent), string(crdContent))
		}
	}
	// examine content of the in-memory filesystem
	// outFs.(*afero.MemMapFs).List()
}
