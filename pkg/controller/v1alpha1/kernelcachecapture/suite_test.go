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

package kernelcachecapture

import (
	"context"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

var (
	cfg          *rest.Config
	k8sClient    client.Client
	mockExecutor *mockPodExecutor
)

func TestKernelCacheCaptureIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KernelCacheCapture Controller Integration Suite")
}

type execResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type execCall struct {
	namespace     string
	podName       string
	containerName string
	cmd           []string
}

type mockPodExecutor struct {
	mu      sync.Mutex
	results map[string]execResult
	calls   []execCall
}

func newMockPodExecutor() *mockPodExecutor {
	return &mockPodExecutor{
		results: make(map[string]execResult),
	}
}

func (m *mockPodExecutor) ExecInPod(_ context.Context, namespace, podName, containerName string, cmd []string) (string, string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, execCall{
		namespace:     namespace,
		podName:       podName,
		containerName: containerName,
		cmd:           cmd,
	})

	// Match by first element of cmd (e.g. "/mcv" or "buildah")
	if len(cmd) > 0 {
		if result, ok := m.results[cmd[0]]; ok {
			return result.stdout, result.stderr, result.exitCode, result.err
		}
	}

	return "", "", 0, nil
}

func (m *mockPodExecutor) setResult(cmdPrefix string, result execResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[cmdPrefix] = result
}

func (m *mockPodExecutor) getCalls() []execCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]execCall, len(m.calls))
	copy(result, m.calls)
	return result
}

func (m *mockPodExecutor) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	m.results = make(map[string]execResult)
}

var _ = BeforeSuite(func(ctx SpecContext) {
	mockExecutor = newMockPodExecutor()

	ctrlFunc := func(restCfg *rest.Config, mgr ctrl.Manager) error {
		clientset, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			return err
		}

		// Create kserve namespace for ConfigMap
		_, err = clientset.CoreV1().Namespaces().Create(context.Background(),
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.KServeNamespace}},
			metav1.CreateOptions{})
		if err != nil {
			return err
		}

		// Create ConfigMap with kernelcache enabled
		_, err = clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Create(context.Background(),
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"kernelcache": `{"enabled": true}`,
				},
			},
			metav1.CreateOptions{})
		if err != nil {
			return err
		}

		recorder := mgr.GetEventRecorderFor("KernelCacheCaptureController")

		return (&KernelCacheCaptureReconciler{
			Client:      mgr.GetClient(),
			Clientset:   clientset,
			Log:         ctrl.Log.WithName("KernelCacheCaptureReconciler"),
			Scheme:      mgr.GetScheme(),
			Recorder:    recorder,
			podExecutor: mockExecutor,
		}).SetupWithManager(mgr)
	}

	envTest := pkgtest.NewEnvTest().
		WithControllers(ctrlFunc).
		Start(context.Background())

	cfg = envTest.Config
	k8sClient = envTest.Client

	// Create test namespace
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	})).Should(Succeed())
})
