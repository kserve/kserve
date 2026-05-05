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

package utils

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/constants"
)

func TestGetInferenceServiceConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	t.Run("returns ConfigMap when it exists", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"ingress": `{"ingressGateway": "kserve/kserve-gateway"}`,
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

		result, err := GetInferenceServiceConfigMap(context.Background(), fakeClient)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result.Name != constants.InferenceServiceConfigMapName {
			t.Errorf("expected name %s, got %s", constants.InferenceServiceConfigMapName, result.Name)
		}
		if result.Namespace != constants.KServeNamespace {
			t.Errorf("expected namespace %s, got %s", constants.KServeNamespace, result.Namespace)
		}
		if result.Data["ingress"] != cm.Data["ingress"] {
			t.Errorf("expected data to match, got %v", result.Data)
		}
	})

	t.Run("returns error when ConfigMap does not exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		_, err := GetInferenceServiceConfigMap(context.Background(), fakeClient)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
