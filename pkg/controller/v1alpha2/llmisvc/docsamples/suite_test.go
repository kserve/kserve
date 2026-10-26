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

// Package docsamples_test admission-checks the LLMInferenceService samples
// published under docs/samples/llmisvc against a real API server.
//
// The samples are documentation: users copy and paste them. Nothing else in
// CI applies them, so without this suite they can only be verified by hand.
// Most of them require GPUs, multi-node topologies or hardware that CI does
// not have, which rules out running them end to end. Admission is the part
// that can be checked everywhere: CRD structural schema, the CEL rules
// (x-kubernetes-validations) compiled into the CRD, and the validating
// webhooks. That covers API-shape drift, which is what breaks samples when
// the API graduates or a field is renamed.
//
// What this suite deliberately does not cover: whether a sample reconciles,
// pulls its images, finds its PVCs or serves a single token. Objects are
// submitted with DryRun, so nothing is ever persisted or reconciled.
package docsamples_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

func TestDocumentationSamples(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Documentation Samples Suite")
}

var envTest *pkgtest.Client

// fixture.SetupTestEnv is reused rather than assembling a webhook-only
// environment, so the samples meet the same admission chain production runs:
// it installs config/webhook/llmisvc/manifests.yaml itself. It also starts the
// controllers, which stay idle - DryRun objects are never persisted, so
// nothing reconciles.
var _ = BeforeSuite(func(ctx SpecContext) {
	envTest = fixture.SetupTestEnv(ctx)

	// One namespace for the whole suite. Dry-run submissions never persist, so
	// there is nothing for the samples to collide over, but the namespace still
	// has to exist for the API server to admit a create against it.
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: samplesNamespace}}
	Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
})
