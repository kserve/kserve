/*
Copyright 2024 The KServe Authors.

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

package localmodel

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testScheme *runtime.Scheme
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

		// Create namespaces required by the controllers
		for _, ns := range []string{constants.KServeNamespace, "kserve-localmodel-jobs"} {
			_, err := clientset.CoreV1().Namespaces().Create(context.Background(),
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}},
				metav1.CreateOptions{})
			if err != nil {
				return err
			}
		}

		// Create the configmap required by SetupWithManager
		_, err = clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Create(context.Background(),
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"localModel": `{
						"jobNamespace": "kserve-localmodel-jobs",
						"defaultJobImage": "kserve/storage-initializer:latest"
					}`,
				},
			},
			metav1.CreateOptions{})
		if err != nil {
			return err
		}

		// Set up LocalModelCache reconciler
		if err := (&LocalModelReconciler{
			Client:    mgr.GetClient(),
			Clientset: clientset,
			Scheme:    mgr.GetScheme(),
			Log:       ctrl.Log.WithName("v1alpha1LocalModelController"),
		}).SetupWithManager(mgr); err != nil {
			return err
		}

		// Set up LocalModelNamespaceCache reconciler
		return (&LocalModelNamespaceCacheReconciler{
			Client:    mgr.GetClient(),
			Clientset: clientset,
			Scheme:    mgr.GetScheme(),
			Log:       ctrl.Log.WithName("v1alpha1LocalModelNamespaceCacheController"),
		}).SetupWithManager(mgr)
	}

	// The suite manager/webhook must outlive BeforeSuite node context.
	envTest := pkgtest.NewEnvTest().
		WithControllers(ctrlFunc).
		Start(context.Background())

	cfg = envTest.Config
	k8sClient = envTest.Client
	testScheme = envTest.Environment.Scheme
	Expect(testScheme).NotTo(BeNil())

	// Create ClusterStorageContainer (needs scheme-aware client)
	clusterStorageContainer := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: v1alpha1.StorageContainerSpec{
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}},
			Container: corev1.Container{
				Name:  "name",
				Image: "image",
				Args: []string{
					"srcURI",
					constants.DefaultModelLocalMountPath,
				},
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				VolumeMounts:             []corev1.VolumeMount{},
			},
		},
	}
	Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())
})
