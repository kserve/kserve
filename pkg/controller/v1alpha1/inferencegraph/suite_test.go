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

package inferencegraph

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InferenceGraph Controller Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	ctrlFunc := func(restCfg *rest.Config, mgr ctrl.Manager) error {
		clientset, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			return err
		}

		deployConfig := &v1beta1.DeployConfig{DefaultDeploymentMode: "Knative"}

		return (&InferenceGraphReconciler{
			Client:    mgr.GetClient(),
			Clientset: clientset,
			Scheme:    mgr.GetScheme(),
			Log:       ctrl.Log.WithName("V1alpha1InferenceGraphController"),
			Recorder:  mgr.GetEventRecorderFor("V1alpha1InferenceGraphController"),
		}).SetupWithManager(mgr, deployConfig)
	}

	envTest := pkgtest.NewEnvTest().
		WithControllers(ctrlFunc).
		// The suite manager/webhook must outlive BeforeSuite node context.
		Start(context.Background())

	cfg = envTest.Config
	k8sClient = envTest.Client

	// Create namespaces
	kserveNamespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.KServeNamespace,
		},
	}
	knativeServingNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.DefaultKnServingNamespace,
		},
	}
	Expect(k8sClient.Create(ctx, kserveNamespaceObj)).Should(Succeed())
	Expect(k8sClient.Create(ctx, knativeServingNamespace)).Should(Succeed())

	// Create knative config-autoscaler configmap
	configAutoscaler := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.AutoscalerConfigmapName,
			Namespace: constants.AutoscalerConfigmapNamespace,
		},
	}
	Expect(k8sClient.Create(ctx, configAutoscaler)).Should(Succeed())
})
