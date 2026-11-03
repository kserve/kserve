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

package docsamples_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	pkgtest "github.com/kserve/kserve/pkg/testing"
)

const (
	// samplesDir is the documentation tree checked by this suite, relative to the repository root.
	samplesDir = "docs/samples/llmisvc"

	// kserveAPIGroup owns the resources whose API version the samples must keep current.
	kserveAPIGroup = "serving.kserve.io"

	// samplesNamespace holds the dry-run submissions. Nothing is ever persisted in it.
	samplesNamespace = "docs-samples"
)

// Not an Ordered container: one rejected sample must not skip the rest.
var _ = Describe("LLMInferenceService documentation samples", func() {
	It("should find documentation samples to check", func() {
		Expect(samplesWalkErr).NotTo(HaveOccurred(), "could not read %s", samplesDir)
		Expect(allSamples).NotTo(BeEmpty(),
			"no YAML documents found under %s - has the directory moved?", samplesDir)
	})

	It("should parse every documentation sample", func() {
		var failures []string
		for _, doc := range allSamples {
			if doc.err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", doc, doc.err))
			}
		}

		Expect(failures).To(BeEmpty(), "documentation samples are not valid Kubernetes YAML")
	})

	It("should use the current storage version of the KServe APIs", func(ctx SpecContext) {
		storageVersions := map[schema.GroupKind]string{}

		var outdated []string
		for _, doc := range allSamples {
			if doc.err != nil {
				continue
			}
			gvk := doc.obj.GroupVersionKind()
			if gvk.Group != kserveAPIGroup {
				continue
			}

			current, cached := storageVersions[gvk.GroupKind()]
			if !cached {
				current = storageVersionFor(ctx, gvk.GroupKind())
				storageVersions[gvk.GroupKind()] = current
			}
			if gvk.Version != current {
				outdated = append(outdated,
					fmt.Sprintf("%s declares %s but the current storage version is %s", doc, gvk.Version, current))
			}
		}

		Expect(outdated).To(BeEmpty(),
			"documentation samples must be updated when the API graduates - older versions still round-trip "+
				"through the conversion webhook, so nothing else in CI notices they went stale")
	})

	for _, doc := range allSamples {
		if doc.err != nil {
			// Already reported by the parsing spec.
			continue
		}

		group := doc.obj.GroupVersionKind().Group
		if reason, unavailable := unavailableAPIs[group]; unavailable {
			It("should skip "+doc.String(), func() {
				gvk := doc.obj.GroupVersionKind()
				// Guard the entry against going stale: once the CRD is installed the
				// sample can be checked for real, and the skip has to go.
				if _, err := envTest.Client.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version); err == nil {
					Fail(fmt.Sprintf("%s is installed after all - drop %q from unavailableAPIs so %s is checked",
						gvk, gvk.Group, doc))
				}

				Skip(reason)
			})

			continue
		}

		It("should be admitted: "+doc.String(), func(ctx SpecContext) {
			obj := doc.obj.DeepCopy()
			gvk := obj.GroupVersionKind()

			mapping, err := envTest.Client.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
			Expect(err).NotTo(HaveOccurred(),
				"no API available for %s - vendor its CRD into test/crds, or add %q to unavailableAPIs "+
					"with the reason it cannot be checked", gvk, gvk.Group)

			// The namespace is the only thing the suite changes about a published
			// sample. Anything beyond that would test something users never see.
			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				obj.SetNamespace(samplesNamespace)
			}

			// Strict field validation is what makes this check worth running: without
			// it the API server prunes unknown fields and returns success, so a
			// renamed or misspelled field looks fine here while `kubectl apply`
			// (strict since 1.25) rejects it. It does not reach into
			// x-kubernetes-preserve-unknown-fields regions, so the EPP plugin config
			// under scheduler.config.inline stays unchecked either way.
			strict := client.FieldValidation(metav1.FieldValidationStrict)
			Expect(envTest.Client.Create(ctx, obj, client.DryRunAll, strict)).To(Succeed(),
				"sample rejected by the API server - it would fail the same way for anyone following the docs")
		})
	}
})

// unavailableAPIs maps an API group used by the documentation samples to the reason
// its CRDs are absent from the envtest bundle (test/crds). Samples in these groups
// are reported as skipped rather than silently dropped.
//
// A sample introducing a group that is neither installed nor listed here fails the
// suite: the choice between vendoring the CRD and accepting no coverage is a
// deliberate one, so it is made in code review rather than by omission.
var unavailableAPIs = map[string]string{
	"sriovnetwork.openshift.io": "SR-IOV Network Operator CRDs are not vendored in test/crds",
	"aigateway.envoyproxy.io":   "Envoy AI Gateway CRDs are not vendored in test/crds",
	"monitoring.coreos.com":     "Prometheus Operator CRDs are not vendored in test/crds",
}

// sampleDoc is a single YAML document from a sample file. A document that could not
// be read carries err instead of obj so that the failure surfaces as a failing spec
// rather than a missing one.
type sampleDoc struct {
	path  string
	index int
	obj   *unstructured.Unstructured
	err   error
}

func (d sampleDoc) String() string {
	location := d.path
	if d.index > 0 {
		location = fmt.Sprintf("%s document %d", d.path, d.index)
	}
	if d.obj == nil {
		return location
	}

	return fmt.Sprintf("%s (%s %s)", location, d.obj.GetKind(), d.obj.GetName())
}

// kustomizationNames are the file names kustomize recognises. They configure a
// build rather than describing a cluster resource, so they are not samples.
var kustomizationNames = map[string]bool{
	"kustomization.yaml": true,
	"kustomization.yml":  true,
	"Kustomization":      true,
}

// allSamples is resolved while the spec tree is built so that every document
// becomes its own spec and shows up individually in the report. samplesWalkErr is
// kept separate: folding it into allSamples would leave the list non-empty and let
// the "did the directory move?" spec pass on a tree that could not be read at all.
var allSamples, samplesWalkErr = discoverSamples()

func discoverSamples() ([]sampleDoc, error) {
	root := filepath.Join(pkgtest.ProjectRoot(), filepath.FromSlash(samplesDir))

	var docs []sampleDoc
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if kustomizationNames[entry.Name()] {
			return nil
		}
		if ext := filepath.Ext(entry.Name()); ext != ".yaml" && ext != ".yml" {
			return nil
		}

		relPath, relErr := filepath.Rel(pkgtest.ProjectRoot(), path)
		if relErr != nil {
			relPath = path
		}

		docs = append(docs, decodeFile(path, filepath.ToSlash(relPath))...)

		return nil
	})

	return docs, walkErr
}

func decodeFile(path, relPath string) []sampleDoc {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return []sampleDoc{{path: relPath, err: err}}
	}

	var docs []sampleDoc
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
	for index := 0; ; index++ {
		chunk, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			docs = append(docs, sampleDoc{path: relPath, index: index, err: readErr})

			break
		}
		if len(bytes.TrimSpace(chunk)) == 0 {
			continue
		}

		// Strict: a duplicated key would otherwise collapse to the last occurrence
		// here, so the API server would never get the chance to notice it.
		obj := &unstructured.Unstructured{}
		if unmarshalErr := yaml.UnmarshalStrict(chunk, &obj.Object); unmarshalErr != nil {
			docs = append(docs, sampleDoc{path: relPath, index: index, err: unmarshalErr})

			continue
		}
		// A chunk holding only comments carries no fields once decoded.
		if len(obj.Object) == 0 {
			continue
		}
		if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
			docs = append(docs, sampleDoc{
				path:  relPath,
				index: index,
				err:   errors.New("document declares no apiVersion/kind"),
			})

			continue
		}

		docs = append(docs, sampleDoc{path: relPath, index: index, obj: obj})
	}

	return docs
}

// storageVersionFor reads the version a kind is persisted as from its installed CRD.
func storageVersionFor(ctx context.Context, gk schema.GroupKind) string {
	GinkgoHelper()

	mapping, err := envTest.Client.RESTMapper().RESTMapping(gk)
	Expect(err).NotTo(HaveOccurred(), "no API available for %s", gk)

	crd := &apiextv1.CustomResourceDefinition{}
	crdName := strings.Join([]string{mapping.Resource.Resource, mapping.Resource.Group}, ".")
	Expect(envTest.Client.Get(ctx, client.ObjectKey{Name: crdName}, crd)).To(Succeed())

	for _, version := range crd.Spec.Versions {
		if version.Storage {
			return version.Name
		}
	}

	Fail(fmt.Sprintf("CRD %s declares no storage version", crdName))

	return ""
}
