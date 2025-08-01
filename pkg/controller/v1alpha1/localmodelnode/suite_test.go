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

package localmodelnode

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	fsMock    *mockFileSystem
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "v1alpha1 Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel := context.WithCancel(context.TODO())
	By("bootstrapping test environment")
	crdDirectoryPaths := []string{
		filepath.Join(pkgtest.ProjectRoot(), "test", "crds"),
	}
	testEnv := pkgtest.SetupEnvTest(crdDirectoryPaths)
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	DeferCleanup(func() {
		cancel()
		By("tearing down the test environment")
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	})

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = v1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	clientset, err := kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(clientset).ToNot(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	// Creates namespace
	kserveNamespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.KServeNamespace,
		},
	}
	jobsNamespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kserve-localmodel-jobs",
		},
	}
	Expect(k8sClient.Create(context.Background(), kserveNamespaceObj)).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), jobsNamespaceObj)).Should(Succeed())

	fsMock = newMockFileSystem()
	fsHelper = fsMock

	err = (&LocalModelNodeReconciler{
		Client:    k8sManager.GetClient(),
		Clientset: clientset,
		Scheme:    scheme.Scheme,
		Log:       ctrl.Log.WithName("v1alpha1LocalModelAgent"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())
	defer GinkgoRecover()
	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()
})
