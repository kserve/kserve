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

package inferenceservice

import (
	"context"
	"encoding/json"
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

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	clientset *kubernetes.Clientset
)

func TestV1beta1APIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "v1beta1 Controller Suite")
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

	err = kedav1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = otelv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(clientset).ToNot(BeNil())

	cacheOpts, err := NewCacheOptions()
	Expect(err).ToNot(HaveOccurred())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Cache: cacheOpts,
	})
	Expect(err).ToNot(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

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
	Expect(k8sClient.Create(context.Background(), kserveNamespaceObj)).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), knativeServingNamespace)).Should(Succeed())

	// Create kantive config-autoscaler configmap
	configAutoscaler := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.AutoscalerConfigmapName,
			Namespace: constants.AutoscalerConfigmapNamespace,
		},
	}
	Expect(k8sClient.Create(context.Background(), configAutoscaler)).Should(Succeed())

	deployConfig := &v1beta1.DeployConfig{DefaultDeploymentMode: "Knative"}
	ingressConfig := &v1beta1.IngressConfig{
		IngressGateway:          constants.KnativeIngressGateway,
		LocalGateway:            constants.KnativeLocalGateway,
		LocalGatewayServiceName: "knative-local-gateway.istio-system.svc.cluster.local",
		DisableIstioVirtualHost: false,
	}
	err = (&InferenceServiceReconciler{
		Client:    k8sClient,
		Clientset: clientset,
		Scheme:    k8sClient.Scheme(),
		Log:       ctrl.Log.WithName("V1beta1InferenceServiceController"),
		Recorder:  k8sManager.GetEventRecorderFor("V1beta1InferenceServiceController"),
	}).SetupWithManager(k8sManager, deployConfig, ingressConfig)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()
})

// getBaseTestConfigs returns the common test configuration shared by all deployment modes
func getBaseTestConfigs() map[string]string {
	return map[string]string{
		"explainers": `{
				"art": {
					"image": "kserve/art-explainer",
					"defaultImageVersion": "latest"
				},
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
		"ingress": `{
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
			}`,
		"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"caBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"cpuModelcar": "10m",
				"memoryModelcar": "15Mi"
			}`,
		"inferenceService": `{}`,
	}
}

// getKnativeTestConfigs returns the default test configuration for Knative deployment mode tests
func getKnativeTestConfigs() map[string]string {
	return getBaseTestConfigs()
}

// getRawKubeTestConfigs returns the default test configuration for RawKube deployment mode tests with Gateway API enabled
func getRawKubeTestConfigs() map[string]string {
	return mergeJSONField(getBaseTestConfigs(), "ingress", map[string]interface{}{
		"enableGatewayApi":         true,
		"additionalIngressDomains": []string{"additional.example.com"},
	})
}

// mergeJSONField merges additional fields into a specific JSON field in configs.
// This allows partial updates to JSON configs without rewriting the entire JSON string.
// The key parameter can be any config key (e.g., "ingress", "explainers", "storageInitializer").
//
// Examples:
//
//	// Merge into ingress config
//	configs := mergeJSONField(getRawKubeTestConfigs(), "ingress", map[string]interface{}{
//	    "disableIngressCreation": true,
//	})
//
//	// Merge into explainers config
//	configs := mergeJSONField(getBaseTestConfigs(), "explainers", map[string]interface{}{
//	    "custom": map[string]interface{}{
//	        "image": "custom-explainer:v1",
//	    },
//	})
//
//nolint:unparam
func mergeJSONField(base map[string]string, key string, updates map[string]interface{}) map[string]string {
	result := make(map[string]string)
	// Copy base configs
	for k, v := range base {
		result[k] = v
	}

	// Get the existing JSON value
	existingJSON := base[key]
	if existingJSON == "" {
		existingJSON = "{}"
	}

	// Unmarshal existing JSON
	var existingData map[string]interface{}
	if err := json.Unmarshal([]byte(existingJSON), &existingData); err != nil {
		// If unmarshal fails, just use updates
		existingData = make(map[string]interface{})
	}

	// Merge updates into existing data
	for k, v := range updates {
		existingData[k] = v
	}

	// Marshal back to JSON
	mergedJSON, err := json.Marshal(existingData)
	if err != nil {
		// If marshal fails, return original
		return result
	}

	result[key] = string(mergedJSON)
	return result
}
