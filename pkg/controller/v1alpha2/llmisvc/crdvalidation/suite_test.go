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

package crdvalidation_test

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kservescheme "github.com/kserve/kserve/pkg/scheme"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

func TestCRDSchemaValidation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD Schema Validation Suite")
}

var envTest *pkgtest.Client

var _ = BeforeSuite(func() {
	By("Starting minimal envtest with CRDs from config/crd/full/")
	// Load CRDs from config/crd/full/ (the source of truth) instead of
	// test/crds/ (a derived kustomize bundle). This validates the actual
	// generated CRD and catches stale Makefile stripping.
	crdRoot := filepath.Join(pkgtest.ProjectRoot(), "config", "crd", "full")
	envTest = pkgtest.Configure(
		pkgtest.WithCRDs(filepath.Join(crdRoot, "llmisvc")),
		pkgtest.WithScheme(kservescheme.AddAll),
	).Start(context.Background())
})
