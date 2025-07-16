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

package webhook_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/webhook"

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"

	"k8s.io/client-go/rest"

	"github.com/kserve/kserve/pkg/constants"

	ctrl "sigs.k8s.io/controller-runtime"

	pkgtest "github.com/kserve/kserve/pkg/testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWebhooks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LLMInferenceService Controller Suite")
}

var envTest *pkgtest.Client

var _ = SynchronizedBeforeSuite(func() {
	duration, err := time.ParseDuration(constants.GetEnvOrDefault("ENVTEST_DEFAULT_TIMEOUT", "10s"))
	Expect(err).NotTo(HaveOccurred())
	SetDefaultEventuallyTimeout(duration)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	By("Setting up the test environment")
	systemNs := constants.KServeNamespace

	ctx, cancel := context.WithCancel(context.Background())
	envTest = pkgtest.NewEnvTest(
		pkgtest.WithWebhookManifests(filepath.Join(pkgtest.ProjectRoot(), "test", "webhooks")),
	).
		WithWebhooks(func(cfg *rest.Config, mgr ctrl.Manager) error {
			clientSet, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				return err
			}
			llmInferenceServiceConfigValidator := webhook.LLMInferenceServiceConfigValidator{
				ClientSet: clientSet,
			}
			if err := llmInferenceServiceConfigValidator.SetupWithManager(mgr); err != nil {
				return err
			}

			llmInferenceServiceValidator := webhook.LLMInferenceServiceValidator{}
			return llmInferenceServiceValidator.SetupWithManager(mgr)
		}).
		Start(ctx)
	DeferCleanup(func() {
		cancel()
		Expect(envTest.Stop()).To(Succeed())
	})

	fixture.RequiredResources(context.Background(), envTest.Client, systemNs)
}, func() {})
