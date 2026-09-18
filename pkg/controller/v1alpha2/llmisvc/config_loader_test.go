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

package llmisvc_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func TestNewConfigConvertsCipherSuitesForOpenSSL(t *testing.T) {
	ingressConfig := &v1beta1.IngressConfig{
		LLMInferenceServiceTLSCipherSuites: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	}

	got := llmisvc.NewConfig(ingressConfig, nil, nil, nil)
	if want := "ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384"; got.TLSCipherSuitesOpenSSL != want {
		t.Fatalf("TLSCipherSuitesOpenSSL = %q, want %q", got.TLSCipherSuitesOpenSSL, want)
	}
}

func TestLoadConfigValidatesAndNormalizesTLSProfile(t *testing.T) {
	cm := fixture.InferenceServiceCfgMap(constants.KServeNamespace)
	fixture.SetIngressConfigKey(cm, "llmInferenceServiceTLSMinVersion", " VersionTLS12 ")
	fixture.SetIngressConfigKey(cm, "llmInferenceServiceTLSCipherSuites", " TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 ")
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(cm).Build()

	got, err := llmisvc.LoadConfig(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, "VersionTLS12", got.TLSMinVersion)
	require.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", got.TLSCipherSuites)

	fixture.SetIngressConfigKey(cm, "llmInferenceServiceTLSMinVersion", "VersionTLS11")
	c = fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(cm).Build()
	_, err = llmisvc.LoadConfig(t.Context(), c)
	require.ErrorContains(t, err, "unrecognized TLS version")
}

func TestLoadConfigLoRAModelRoutingStrategy(t *testing.T) {
	for _, tt := range []struct {
		input   string
		want    llmisvc.LoRAModelRoutingStrategy
		wantErr bool
	}{
		{input: "", want: llmisvc.LoRAModelRoutingStrategyExact},
		{input: "exact", want: llmisvc.LoRAModelRoutingStrategyExact},
		{input: "Exact", want: llmisvc.LoRAModelRoutingStrategyExact},
		{input: "REGEX", want: llmisvc.LoRAModelRoutingStrategyRegex},
		{input: "RegEx", want: llmisvc.LoRAModelRoutingStrategyRegex},
		{input: "Unsupported", wantErr: true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			cm := fixture.InferenceServiceCfgMap(constants.KServeNamespace)
			if tt.input != "" {
				fixture.SetIngressConfigKey(cm, "loraModelRoutingStrategy", tt.input)
			}
			c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(cm).Build()

			got, err := llmisvc.LoadConfig(t.Context(), c)

			if tt.wantErr {
				require.ErrorContains(t, err, "loraModelRoutingStrategy", "an unsupported value fails config loading like any other invalid ingress key")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.LoRAModelRoutingStrategy, "the loaded value is defaulted and lowercased")
		})
	}
}

func TestNewSchedulerConfig(t *testing.T) {
	tests := []struct {
		name                      string
		configMapData             map[string]string
		wantErr                   bool
		wantExpirationAnnotations []string
	}{
		{
			name:                      "missing scheduler key uses defaults",
			configMapData:             map[string]string{},
			wantExpirationAnnotations: llmisvc.DefaultExpirationAnnotations,
		},
		{
			name: "empty JSON object uses defaults",
			configMapData: map[string]string{
				"scheduler": `{}`,
			},
			wantExpirationAnnotations: llmisvc.DefaultExpirationAnnotations,
		},
		{
			name: "custom expiration annotations",
			configMapData: map[string]string{
				"scheduler": `{"expirationAnnotations":["custom.io/expiration-v2","custom.io/expiration"]}`,
			},
			wantExpirationAnnotations: []string{"custom.io/expiration-v2", "custom.io/expiration"},
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
		})
	}
}

func TestLoadConfig(t *testing.T) {
	cachedConfigMap := fixture.InferenceServiceCfgMapWithUrlScheme(constants.KServeNamespace, "https")
	c := fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme).
		WithObjects(cachedConfigMap).
		Build()

	got, err := llmisvc.LoadConfig(t.Context(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.UrlScheme != "https" {
		t.Fatalf("UrlScheme = %q, want %q", got.UrlScheme, "https")
	}
	if got.IngressGatewayNamespace != constants.KServeNamespace {
		t.Fatalf("IngressGatewayNamespace = %q, want %q", got.IngressGatewayNamespace, constants.KServeNamespace)
	}
	if got.IngressGatewayName != "kserve-ingress-gateway" {
		t.Fatalf("IngressGatewayName = %q, want %q", got.IngressGatewayName, "kserve-ingress-gateway")
	}
	if got.StorageConfig == nil {
		t.Fatal("StorageConfig = nil, want populated config")
	}
	if got.CredentialConfig == nil {
		t.Fatal("CredentialConfig = nil, want populated config")
	}
	if got.SchedulerConfig == nil {
		t.Fatal("SchedulerConfig = nil, want populated config")
	}
}
