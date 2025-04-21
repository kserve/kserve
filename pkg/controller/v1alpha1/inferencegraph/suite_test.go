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
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"knative.dev/operator/pkg/apis/operator/base"
	operatorv1beta1 "knative.dev/operator/pkg/apis/operator/v1beta1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kfservingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	cancel    context.CancelFunc
	ctx       context.Context
	clientset kubernetes.Interface
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "inferencegraph Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())
	By("bootstrapping test environment")
	crdDirectoryPaths := []string{
		filepath.Join("..", "..", "..", "..", "test", "crds"),
	}
	testEnv = pkgtest.SetupEnvTest(crdDirectoryPaths)
	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = kfservingv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = v1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = knservingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = netv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = routev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(clientset).ToNot(BeNil())

	// Create namespaces
	kfservingNamespaceObj := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.KServeNamespace,
		},
	}
	knativeServingNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.DefaultKnServingNamespace,
		},
	}
	Expect(k8sClient.Create(context.Background(), kfservingNamespaceObj)).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), knativeServingNamespace)).Should(Succeed())

	// Create knativeserving custom resource
	knativeCr := &operatorv1beta1.KnativeServing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.DefaultKnServingName,
			Namespace: constants.DefaultKnServingNamespace,
		},
		Spec: operatorv1beta1.KnativeServingSpec{
			CommonSpec: base.CommonSpec{
				Config: base.ConfigMapData{
					"autoscaler": map[string]string{
						"allow-zero-initial-scale": "true",
					},
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), knativeCr)).Should(Succeed())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	deployConfig := &v1beta1.DeployConfig{DefaultDeploymentMode: "Serverless"}

	err = (&InferenceGraphReconciler{
		Client:    k8sClient,
		Clientset: clientset,
		Scheme:    k8sClient.Scheme(),
		Log:       ctrl.Log.WithName("V1alpha1InferenceGraphController"),
		Recorder:  k8sManager.GetEventRecorderFor("V1alpha1InferenceGraphController"),
	}).SetupWithManager(k8sManager, deployConfig)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
