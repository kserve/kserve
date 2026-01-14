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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
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

		llmCtrl := llmisvc.LLMISVCReconciler{
			Client:    mgr.GetClient(),
			Clientset: clientSet,
			// TODO fix it to be set up similar to main.go, for now it's stub
			EventRecorder: eventBroadcaster.NewRecorder(mgr.GetScheme(), corev1.EventSource{Component: "v1beta1Controllers"}),
			Validator: func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
				_, err := (&v1alpha2.LLMInferenceServiceValidator{}).ValidateCreate(ctx, llmSvc)
				return err
			},
		}
		return llmCtrl.SetupWithManager(mgr)
	}

	webhookManifests := pkgtest.WithWebhookManifests(filepath.Join(pkgtest.ProjectRoot(), "test", "webhooks"))
	webhooks := func(cfg *rest.Config, mgr ctrl.Manager) error {
		clientSet, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return err
		}

		// Create validation function for config template validation
		v2ConfigValidationFunc := func(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig) error {
			llmisvcConfig, err := llmisvc.LoadConfig(ctx, clientSet)
			if err != nil {
				return err
			}
			_, err = llmisvc.ReplaceVariables(llmisvc.LLMInferenceServiceSample(), config, llmisvcConfig)
			return err
		}
		v1ConfigValidationFunc := func(ctx context.Context, config *v1alpha1.LLMInferenceServiceConfig) error {
			v2Config := &v1alpha2.LLMInferenceServiceConfig{}
			if err := config.ConvertTo(v2Config); err != nil {
				return err
			}
			return v2ConfigValidationFunc(ctx, v2Config)
		}

		// Register v1alpha1 validators
		v1alpha1LLMValidator := &v1alpha1.LLMInferenceServiceValidator{}
		if err := v1alpha1LLMValidator.SetupWithManager(mgr); err != nil {
			return err
		}

		v1alpha1ConfigValidator := &v1alpha1.LLMInferenceServiceConfigValidator{
			ConfigValidationFunc: v1ConfigValidationFunc,
		}
		if err := v1alpha1ConfigValidator.SetupWithManager(mgr); err != nil {
			return err
		}

		// Register v1alpha2 validators
		v1alpha2LLMValidator := &v1alpha2.LLMInferenceServiceValidator{}
		if err := v1alpha2LLMValidator.SetupWithManager(mgr); err != nil {
			return err
		}

		v1alpha2ConfigValidator := &v1alpha2.LLMInferenceServiceConfigValidator{
			ConfigValidationFunc: v2ConfigValidationFunc,
		}
		return v1alpha2ConfigValidator.SetupWithManager(mgr)
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
