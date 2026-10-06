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

package config

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestLoad(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}

	tests := []struct {
		name       string
		configMap  *corev1.ConfigMap
		wantEnable bool
		wantErr    string
	}{
		{
			name: "loads kernelcache configuration",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data:       map[string]string{v1beta1.KernelCacheConfigName: `{"enabled":true,"jobNamespace":"kernelcache-jobs"}`},
			},
			wantEnable: true,
		},
		{
			name:    "reports missing configmap",
			wantErr: "inferenceservice-config was not found",
		},
		{
			name: "reports invalid configuration",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data:       map[string]string{v1beta1.KernelCacheConfigName: "not-json"},
			},
			wantErr: "parse KernelCache configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{}
			if tt.configMap != nil {
				objects = append(objects, tt.configMap)
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()

			got, err := Load(context.Background(), reader)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.Enabled != tt.wantEnable {
				t.Fatalf("Load() Enabled = %v, want %v", got.Enabled, tt.wantEnable)
			}
			if got.JobNamespace != "kernelcache-jobs" {
				t.Fatalf("Load() JobNamespace = %q, want %q", got.JobNamespace, "kernelcache-jobs")
			}
		})
	}
}
