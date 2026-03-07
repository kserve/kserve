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

package llmisvc

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/constants"
)

func TestLoadConfigFromClient(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	t.Run("successfully loads config with namespace/gateway format", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"ingress": `{
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"kserveIngressGateway": "istio-system/kserve-gateway"
				}`,
				"storageInitializer": `{
					"image": "kserve/storage-initializer:latest",
					"memoryRequest": "100Mi",
					"memoryLimit": "1Gi",
					"cpuRequest": "100m",
					"cpuLimit": "1",
					"cpuModelcar": "10m",
					"memoryModelcar": "15Mi"
				}`,
				"credentials": `{
					"gcs": {"gcsCredentialFileName": "gcloud-application-credentials.json"},
					"s3": {"s3AccessKeyIDName": "AWS_ACCESS_KEY_ID"}
				}`,
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		config, err := LoadConfigFromClient(context.Background(), fakeClient)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if config.IngressGatewayNamespace != "istio-system" {
			t.Errorf("expected gateway namespace 'istio-system', got %q", config.IngressGatewayNamespace)
		}
		if config.IngressGatewayName != "kserve-gateway" {
			t.Errorf("expected gateway name 'kserve-gateway', got %q", config.IngressGatewayName)
		}
		if config.SystemNamespace != constants.KServeNamespace {
			t.Errorf("expected system namespace %q, got %q", constants.KServeNamespace, config.SystemNamespace)
		}
		if config.StorageConfig == nil {
			t.Fatal("expected StorageConfig to be set")
		}
		if config.StorageConfig.Image != "kserve/storage-initializer:latest" {
			t.Errorf("expected storage image 'kserve/storage-initializer:latest', got %q", config.StorageConfig.Image)
		}
		if config.CredentialConfig == nil {
			t.Fatal("expected CredentialConfig to be set")
		}
	})

	t.Run("successfully loads config with simple gateway name", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"ingress": `{
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"kserveIngressGateway": "kserve-gateway"
				}`,
				"storageInitializer": `{
					"image": "kserve/storage-initializer:latest",
					"memoryRequest": "100Mi",
					"memoryLimit": "1Gi",
					"cpuRequest": "100m",
					"cpuLimit": "1",
					"cpuModelcar": "10m",
					"memoryModelcar": "15Mi"
				}`,
				"credentials": `{}`,
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		config, err := LoadConfigFromClient(context.Background(), fakeClient)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if config.IngressGatewayNamespace != constants.KServeNamespace {
			t.Errorf("expected gateway namespace %q, got %q", constants.KServeNamespace, config.IngressGatewayNamespace)
		}
		if config.IngressGatewayName != "kserve-gateway" {
			t.Errorf("expected gateway name 'kserve-gateway', got %q", config.IngressGatewayName)
		}
	})

	t.Run("returns error when ConfigMap does not exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		_, err := LoadConfigFromClient(context.Background(), fakeClient)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error when ingress config is invalid JSON", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"ingress":            `{invalid-json}`,
				"storageInitializer": `{}`,
				"credentials":        `{}`,
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		_, err := LoadConfigFromClient(context.Background(), fakeClient)
		if err == nil {
			t.Fatal("expected error for invalid ingress JSON, got nil")
		}
	})
}
