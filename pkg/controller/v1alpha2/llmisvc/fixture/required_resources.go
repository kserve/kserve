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
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/kmeta"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"

	"github.com/kserve/kserve/pkg/testing"

	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"
)

const (
	defaultGatewayClass = "istio"
)

func RequiredResources(ctx context.Context, c client.Client, ns string) {
	gomega.Expect(c.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	})).To(gomega.Succeed())

	gomega.Expect(c.Create(ctx, InferenceServiceCfgMap(ns))).To(gomega.Succeed())

	for _, preset := range SharedConfigPresets(ns) {
		gomega.Expect(c.Create(ctx, preset)).To(gomega.Succeed())
	}

	gomega.Expect(c.Create(ctx, DefaultGateway(ns))).To(gomega.Succeed())
	gomega.Expect(c.Create(ctx, DefaultGatewayClass())).To(gomega.Succeed())
	gomega.Expect(c.Create(ctx, DefaultClusterStorageContainer())).To(gomega.Succeed())
	gomega.Expect(c.Create(ctx, DefaultModelcarClusterStorageContainer())).To(gomega.Succeed())
}

// DefaultClusterStorageContainer returns a cluster-scoped CSC covering the
// download-based URI schemes (s3://, hf://, https://, …) exercised by the
// storage integration tests.
func DefaultClusterStorageContainer() *v1alpha1.ClusterStorageContainer {
	return &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name:  "storage-initializer",
				Image: "kserve/storage-initializer:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "s3://"},
				{Prefix: "gs://"},
				{Prefix: "hf://"},
				{Prefix: "https://"},
				{Prefix: "http://"},
			},
			WorkloadType:               v1alpha1.InitContainer,
			SupportsMultiModelDownload: ptr.To(true),
		},
	}
}

// DefaultModelcarClusterStorageContainer returns a cluster-scoped CSC that
// matches oci:// URIs. The container spec supplies modelcar-sized resources;
// the image and args are overridden at pod-injection time (image from the
// oci:// URI, args to the shared-namespace symlink command).
func DefaultModelcarClusterStorageContainer() *v1alpha1.ClusterStorageContainer {
	return &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "modelcar",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: corev1.Container{
				Name: "modelcar",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("15Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("15Mi"),
					},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{
				{Prefix: "oci://"},
			},
			WorkloadType: v1alpha1.InitContainer,
		},
	}
}

func IstioShadowService(name, ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(name, "istio-shadow"),
			Namespace: ns,
			Labels: map[string]string{
				"istio.io/inferencepool-name": kmeta.ChildName(name, "-inference-pool"),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 8000},
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.IntOrString{IntVal: 8001},
				},
			},
		},
	}
}

func DefaultGateway(ns string) *gwapiv1.Gateway {
	defaultGateway := Gateway(constants.GatewayName,
		InNamespace[*gwapiv1.Gateway](ns),
		WithClassName(defaultGatewayClass),
		WithInfrastructureLabels("serving.kserve.io/gateway", constants.GatewayName),
		WithListeners(gwapiv1.Listener{
			Name:     "http",
			Port:     80,
			Protocol: gwapiv1.HTTPProtocolType,
			AllowedRoutes: &gwapiv1.AllowedRoutes{
				Namespaces: &gwapiv1.RouteNamespaces{
					From: ptr.To(gwapiv1.NamespacesFromAll),
				},
			},
		}),
	)

	return defaultGateway
}

func DefaultGatewayClass() *gwapiv1.GatewayClass {
	return &gwapiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultGatewayClass,
		},
		Spec: gwapiv1.GatewayClassSpec{
			ControllerName: "istio.io/gateway-controller",
		},
	}
}

func InferenceServiceCfgMap(ns string) *corev1.ConfigMap {
	return InferenceServiceCfgMapWithUrlScheme(ns, "")
}

func InferenceServiceCfgMapWithUrlScheme(ns, urlScheme string) *corev1.ConfigMap {
	urlSchemeConfig := ""
	if urlScheme != "" {
		urlSchemeConfig = `,"urlScheme": "` + urlScheme + `"`
	}
	configs := map[string]string{
		"ingress": `{
				"enableGatewayApi": true,
				"enableLLMInferenceServiceTLS": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"]` + urlSchemeConfig + `
			}`,
		"storageInitializer": `{
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"cpuModelcar": "10m",
				"memoryModelcar": "15Mi",
				"enableModelcar": true
			}`,
		"credentials": `{
				"s3": {
					"s3AccessKeyIDName": "AWS_ACCESS_KEY_ID",
					"s3SecretAccessKeyName": "AWS_SECRET_ACCESS_KEY"
				}
			}`,
		"autoscaling-wva-controller-config": `{
				"prometheus": {
					"url": "http://prometheus.monitoring:9090"
				}
			}`,
	} // #nosec G101
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: ns,
		},
		Data: configs,
	}

	return configMap
}

// SharedConfigPresets loads preset files shared as kustomize manifests that are stored in projects config.
// Every file prefixed with `config-` is treated as such
func SharedConfigPresets(ns string) []*v1alpha2.LLMInferenceServiceConfig {
	configDir := filepath.Join(testing.ProjectRoot(), "config", "llmisvcconfig")
	var configs []*v1alpha2.LLMInferenceServiceConfig
	err := filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") || !strings.HasPrefix(info.Name(), "config-") {
			return nil
		}

		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}

		config := &v1alpha2.LLMInferenceServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return err
		}

		configs = append(configs, config)

		return nil
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return configs
}
