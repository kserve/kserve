/*
Copyright 2018 The Knative Authors.

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

package network

import (
	"bytes"
	"net/http"
	"testing"
	"text/template"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/knative/pkg/system"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/knative/pkg/configmap/testing"
	_ "github.com/knative/pkg/system/testing"
)

func TestOurConfig(t *testing.T) {
	cm, example := ConfigMapsFromTestFile(t, ConfigName)

	if _, err := NewConfigFromConfigMap(cm); err != nil {
		t.Errorf("NewConfigFromConfigMap(actual) = %v", err)
	}
	if _, err := NewConfigFromConfigMap(example); err != nil {
		t.Errorf("NewConfigFromConfigMap(example) = %v", err)
	}
}

func TestConfiguration(t *testing.T) {
	nonDefaultDomainTemplate := "{{.Namespace}}.{{.Name}}.{{.Domain}}"

	networkConfigTests := []struct {
		name       string
		wantErr    bool
		wantConfig *Config
		config     *corev1.ConfigMap
	}{{
		name:    "network configuration with no network input",
		wantErr: false,
		wantConfig: &Config{
			IstioOutboundIPRanges:      "*",
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
		},
	}, {
		name:    "network configuration with invalid outbound IP range",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "10.10.10.10/33",
			},
		},
	}, {
		name:    "network configuration with empty network",
		wantErr: false,
		wantConfig: &Config{
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "",
			},
		},
	}, {
		name:    "network configuration with both valid and some invalid range",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "10.10.10.10/12,invalid",
			},
		},
	}, {
		name:    "network configuration with invalid network range",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "10.10.10.10/12,-1.1.1.1/10",
			},
		},
	}, {
		name:    "network configuration with invalid network key",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "this is not an IP range",
			},
		},
	}, {
		name:    "network configuration with invalid network",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "*,*",
			},
		},
	}, {
		name:    "network configuration with incomplete network array",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "*,",
			},
		},
	}, {
		name:    "network configuration with invalid network string",
		wantErr: false,
		wantConfig: &Config{
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: ", ,",
			},
		},
	}, {
		name:    "network configuration with invalid network string",
		wantErr: false,
		wantConfig: &Config{
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: ",,",
			},
		},
	}, {
		name:    "network configuration with invalid network range",
		wantErr: false,
		wantConfig: &Config{
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: ",",
			},
		},
	}, {
		name:    "network configuration with valid CIDR network range",
		wantErr: false,
		wantConfig: &Config{
			IstioOutboundIPRanges:      "10.10.10.0/24",
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "10.10.10.0/24",
			},
		},
	}, {
		name:    "network configuration with multiple valid network ranges",
		wantErr: false,
		wantConfig: &Config{
			IstioOutboundIPRanges:      "10.10.10.0/24,10.240.10.0/14,192.192.10.0/16",
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "10.10.10.0/24,10.240.10.0/14,192.192.10.0/16",
			},
		},
	}, {
		name:    "network configuration with valid network",
		wantErr: false,
		wantConfig: &Config{
			IstioOutboundIPRanges:      "*",
			DefaultClusterIngressClass: "istio.ingress.networking.knative.dev",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey: "*",
			},
		},
	}, {
		name:    "network configuration with non-Istio ingress type",
		wantErr: false,
		wantConfig: &Config{
			IstioOutboundIPRanges:      "*",
			DefaultClusterIngressClass: "foo-ingress",
			DomainTemplate:             DefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
			},
		},
	}, {
		name:    "network configuration with diff domain template",
		wantErr: false,
		wantConfig: &Config{
			IstioOutboundIPRanges:      "*",
			DefaultClusterIngressClass: "foo-ingress",
			DomainTemplate:             nonDefaultDomainTemplate,
		},
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
				DomainTemplateKey:             nonDefaultDomainTemplate,
			},
		},
	}, {
		name:    "network configuration with blank domain template",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
				DomainTemplateKey:             "",
			},
		},
	}, {
		name:    "network configuration with bad domain template",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
				// This is missing a closing brace.
				DomainTemplateKey: "{{.Namespace}.{{.Name}}.{{.Domain}}",
			},
		},
	}, {
		name:    "network configuration with bad domain template",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
				// This is missing a closing brace.
				DomainTemplateKey: "{{.Namespace}.{{.Name}}.{{.Domain}}",
			},
		},
	}, {
		name:    "network configuration with bad url",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
				// Paths are disallowed
				DomainTemplateKey: "{{.Domain}}/{{.Namespace}}/{{.Name}}.",
			},
		},
	}, {
		name:    "network configuration with bad variable",
		wantErr: true,
		config: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.Namespace(),
				Name:      ConfigName,
			},
			Data: map[string]string{
				IstioOutboundIPRangesKey:      "*",
				DefaultClusterIngressClassKey: "foo-ingress",
				// Bad variable
				DomainTemplateKey: "{{.Name}}.{{.NAmespace}}.{{.Domain}}",
			},
		},
	}}

	for _, tt := range networkConfigTests {
		t.Run(tt.name, func(t *testing.T) {
			actualConfig, err := NewConfigFromConfigMap(tt.config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Test: %q; NewConfigFromConfigMap() error = %v, WantErr %v",
					tt.name, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			data := DomainTemplateValues{
				Name:      "foo",
				Namespace: "bar",
				Domain:    "baz.com",
			}
			want := mustExecute(t, tt.wantConfig.GetDomainTemplate(), data)
			got := mustExecute(t, actualConfig.GetDomainTemplate(), data)
			if got != want {
				t.Errorf("DomainTemplate(data) = %s, wanted %s", got, want)
			}

			ignoreDT := cmpopts.IgnoreFields(Config{}, "DomainTemplate")

			if diff := cmp.Diff(actualConfig, tt.wantConfig, ignoreDT); diff != "" {
				t.Fatalf("want %v, but got %v",
					tt.wantConfig, actualConfig)
			}
		})
	}
}

func mustExecute(t *testing.T, tmpl *template.Template, data interface{}) string {
	t.Helper()
	buf := bytes.Buffer{}
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Errorf("Error executing the DomainTemplate: %v", err)
	}
	return buf.String()
}

func TestIsKubeletProbe(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if err != nil {
		t.Fatalf("Error building request: %v", err)
	}
	if IsKubeletProbe(req) {
		t.Error("Not a kubelet probe but counted as such")
	}
	req.Header.Set("User-Agent", kubeProbeUAPrefix+"1.14")
	if !IsKubeletProbe(req) {
		t.Error("kubelet probe but not counted as such")
	}
}
