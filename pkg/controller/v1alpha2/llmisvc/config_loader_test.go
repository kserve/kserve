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

package llmisvc_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func TestNewSchedulerConfig(t *testing.T) {
	tests := []struct {
		name                      string
		configMapData             map[string]string
		wantErr                   bool
		wantExpirationAnnotations []string
		wantRestartAnnotation     string
	}{
		{
			name:                      "missing scheduler key uses defaults",
			configMapData:             map[string]string{},
			wantExpirationAnnotations: llmisvc.DefaultExpirationAnnotations,
			wantRestartAnnotation:     llmisvc.DefaultRestartAnnotation,
		},
		{
			name: "empty JSON object uses defaults",
			configMapData: map[string]string{
				"scheduler": `{}`,
			},
			wantExpirationAnnotations: llmisvc.DefaultExpirationAnnotations,
			wantRestartAnnotation:     llmisvc.DefaultRestartAnnotation,
		},
		{
			name: "custom expiration annotations",
			configMapData: map[string]string{
				"scheduler": `{"expirationAnnotations":["custom.io/expiration-v2","custom.io/expiration"]}`,
			},
			wantExpirationAnnotations: []string{"custom.io/expiration-v2", "custom.io/expiration"},
			wantRestartAnnotation:     llmisvc.DefaultRestartAnnotation,
		},
		{
			name: "custom restart annotation",
			configMapData: map[string]string{
				"scheduler": `{"restartAnnotation":"custom.io/cert-hash"}`,
			},
			wantExpirationAnnotations: llmisvc.DefaultExpirationAnnotations,
			wantRestartAnnotation:     "custom.io/cert-hash",
		},
		{
			name: "both fields set",
			configMapData: map[string]string{
				"scheduler": `{"expirationAnnotations":["a","b"],"restartAnnotation":"c"}`,
			},
			wantExpirationAnnotations: []string{"a", "b"},
			wantRestartAnnotation:     "c",
		},
		{
			name: "invalid JSON returns error",
			configMapData: map[string]string{
				"scheduler": `{not-json`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "inferenceservice-config"},
				Data:       tt.configMapData,
			}

			got, err := llmisvc.NewSchedulerConfig(cm)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got.ExpirationAnnotations) != len(tt.wantExpirationAnnotations) {
				t.Errorf("ExpirationAnnotations length = %d, want %d", len(got.ExpirationAnnotations), len(tt.wantExpirationAnnotations))
			}
			for i := range got.ExpirationAnnotations {
				if got.ExpirationAnnotations[i] != tt.wantExpirationAnnotations[i] {
					t.Errorf("ExpirationAnnotations[%d] = %q, want %q", i, got.ExpirationAnnotations[i], tt.wantExpirationAnnotations[i])
				}
			}
			if got.RestartAnnotation != tt.wantRestartAnnotation {
				t.Errorf("RestartAnnotation = %q, want %q", got.RestartAnnotation, tt.wantRestartAnnotation)
			}
		})
	}
}
