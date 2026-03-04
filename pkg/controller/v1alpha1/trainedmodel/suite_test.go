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

package trainedmodel

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/reconcilers/modelconfig"
	pkgtest "github.com/kserve/kserve/pkg/testing"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "v1alpha1 Controller Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	ctrlFunc := func(restCfg *rest.Config, mgr ctrl.Manager) error {
		clientset, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			return err
		}

		return (&TrainedModelReconciler{
			Client:                mgr.GetClient(),
			Clientset:             clientset,
			Scheme:                mgr.GetScheme(),
			Log:                   ctrl.Log.WithName("v1beta1TrainedModelController"),
			Recorder:              record.NewBroadcaster().NewRecorder(mgr.GetScheme(), corev1.EventSource{Component: "v1betaController"}),
			ModelConfigReconciler: modelconfig.NewModelConfigReconciler(mgr.GetClient(), clientset, mgr.GetScheme()),
		}).SetupWithManager(mgr)
	}

	envTest := pkgtest.NewEnvTest().
		WithControllers(ctrlFunc).
		// The suite manager/webhook must outlive BeforeSuite node context.
		Start(context.Background())

	cfg = envTest.Config
	k8sClient = envTest.Client

	// Create namespace
	kserveNamespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.KServeNamespace,
		},
	}
	Expect(k8sClient.Create(ctx, kserveNamespaceObj)).Should(Succeed())
})
