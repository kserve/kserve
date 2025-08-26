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

package fixture

import (
	"context"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/llmisvc"
	"github.com/kserve/kserve/pkg/controller/llmisvc/validation"
	pkgtest "github.com/kserve/kserve/pkg/testing"

	ctrl "sigs.k8s.io/controller-runtime"
)

func SetupTestEnv() *pkgtest.Client {
	duration, err := time.ParseDuration(constants.GetEnvOrDefault("ENVTEST_DEFAULT_TIMEOUT", "30s"))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.SetDefaultEventuallyTimeout(duration)
	gomega.SetDefaultEventuallyPollingInterval(250 * time.Millisecond)
	gomega.EnforceDefaultTimeoutsWhenUsingContexts()

	ginkgo.By("Setting up the test environment")
	systemNs := constants.KServeNamespace

	ctx, cancel := context.WithCancel(context.Background())

	llmCtrlFunc := func(cfg *rest.Config, mgr ctrl.Manager) error {
		eventBroadcaster := record.NewBroadcaster()
		clientSet, err := kubernetes.NewForConfig(cfg)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		llmCtrl := llmisvc.LLMInferenceServiceReconciler{
			Client:    mgr.GetClient(),
			Clientset: clientSet,
			// TODO fix it to be set up similar to main.go, for now it's stub
			EventRecorder: eventBroadcaster.NewRecorder(mgr.GetScheme(), corev1.EventSource{Component: "v1beta1Controllers"}),
		}
		return llmCtrl.SetupWithManager(mgr)
	}

	webhookManifests := pkgtest.WithWebhookManifests(filepath.Join(pkgtest.ProjectRoot(), "test", "webhooks"))
	webhooks := func(cfg *rest.Config, mgr ctrl.Manager) error {
		clientSet, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return err
		}
		llmInferenceServiceConfigValidator := validation.LLMInferenceServiceConfigValidator{
			ClientSet: clientSet,
		}
		if err := llmInferenceServiceConfigValidator.SetupWithManager(mgr); err != nil {
			return err
		}

		llmInferenceServiceValidator := validation.LLMInferenceServiceValidator{}
		return llmInferenceServiceValidator.SetupWithManager(mgr)
	}

	envTest := pkgtest.NewEnvTest(webhookManifests).
		WithWebhooks(webhooks).
		WithControllers(llmCtrlFunc).
		Start(ctx)

	ginkgo.DeferCleanup(func() {
		cancel()
		gomega.Expect(envTest.Stop()).To(gomega.Succeed())
	})

	RequiredResources(context.Background(), envTest.Client, systemNs)

	return envTest
}
