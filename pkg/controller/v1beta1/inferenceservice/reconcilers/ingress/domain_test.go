/*
Copyright 2022 The KServe Authors.

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

package ingress

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/network"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestGenerateDomainName(t *testing.T) {
	type args struct {
		name          string
		obj           metav1.ObjectMeta
		ingressConfig *v1beta1.IngressConfig
	}

	obj := metav1.ObjectMeta{
		Name:      "model",
		Namespace: "test",
		Annotations: map[string]string{
			"annotation": "annotation-value",
		},
		Labels: map[string]string{
			"label": "label-value",
		},
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "default domain template",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  v1beta1.DefaultIngressDomain,
					DomainTemplate: v1beta1.DefaultDomainTemplate,
				},
			},
			want: "model-test.example.com",
		},
		{
			name: "template with dot",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  v1beta1.DefaultIngressDomain,
					DomainTemplate: "{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}",
				},
			},
			want: "model.test.example.com",
		},
		{
			name: "template with annotation",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  v1beta1.DefaultIngressDomain,
					DomainTemplate: "{{ .Name }}.{{ .Namespace }}.{{ .Annotations.annotation }}.{{ .IngressDomain }}",
				},
			},
			want: "model.test.annotation-value.example.com",
		},
		{
			name: "template with label",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  v1beta1.DefaultIngressDomain,
					DomainTemplate: "{{ .Name }}.{{ .Namespace }}.{{ .Labels.label }}.{{ .IngressDomain }}",
				},
			},
			want: "model.test.label-value.example.com",
		},
		{
			name: "unknown variable",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  v1beta1.DefaultIngressDomain,
					DomainTemplate: "{{ .ModelName }}.{{ .Namespace }}.{{ .IngressDomain }}",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid domain name",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  v1beta1.DefaultIngressDomain,
					DomainTemplate: "{{ .Name }}_{{ .Namespace }}_{{ .IngressDomain }}",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateDomainName(tt.args.name, tt.args.obj, tt.args.ingressConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateDomainName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Test %q unexpected domain (-want +got): %v", tt.name, diff)
			}
		})
	}
}

func TestGetAdditionalHosts(t *testing.T) {
	tests := []struct {
		name        string
		domainList  *[]string
		serviceHost string
		config      *v1beta1.IngressConfig
		want        *[]string
	}{
		{
			name:        "nil domain list",
			domainList:  nil,
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com"},
			},
			want: &[]string{},
		},
		{
			name:        "empty domain list",
			domainList:  &[]string{},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com"},
			},
			want: &[]string{},
		},
		{
			name:        "nil additional domains",
			domainList:  &[]string{"example.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: nil,
			},
			want: &[]string{},
		},
		{
			name:        "empty additional domains",
			domainList:  &[]string{"example.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{},
			},
			want: &[]string{},
		},
		{
			name:        "no matching domain",
			domainList:  &[]string{"other.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com"},
			},
			want: &[]string{},
		},
		{
			name:        "single additional domain",
			domainList:  &[]string{"example.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com"},
			},
			want: &[]string{"model-test.secondary.com"},
		},
		{
			name:        "multiple additional domains",
			domainList:  &[]string{"example.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com", "tertiary.com"},
			},
			want: &[]string{"model-test.secondary.com", "model-test.tertiary.com"},
		},
		{
			name:        "duplicate additional domains",
			domainList:  &[]string{"example.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com", "secondary.com", "tertiary.com"},
			},
			want: &[]string{"model-test.secondary.com", "model-test.tertiary.com"},
		},
		{
			name:        "invalid domain name",
			domainList:  &[]string{"example.com"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"invalid_domain", "secondary.com"},
			},
			want: &[]string{"model-test.secondary.com"},
		},
		{
			name:        "multiple domains in domain list",
			domainList:  &[]string{"example.org", "example.com", "example.net"},
			serviceHost: "model-test.example.com",
			config: &v1beta1.IngressConfig{
				AdditionalIngressDomains: &[]string{"secondary.com", "tertiary.com"},
			},
			want: &[]string{"model-test.secondary.com", "model-test.tertiary.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAdditionalHosts(tt.domainList, tt.serviceHost, tt.config)
			if diff := cmp.Diff(*tt.want, *got); diff != "" {
				t.Errorf("GetAdditionalHosts() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGenerateInternalDomainName(t *testing.T) {
	type args struct {
		name          string
		obj           metav1.ObjectMeta
		ingressConfig *v1beta1.IngressConfig
	}

	// Save and restore the original cluster domain name
	origClusterDomain := network.GetClusterDomainName()
	defer func() {
		// Restore the original cluster domain name if possible
		_ = origClusterDomain // No-op, as we cannot set it back without using reflection on unexported fields
	}()

	// NOTE: We cannot set the cluster domain name directly as it is unexported in the network package.
	// For testing purposes, we assume the cluster domain is "svc.cluster.local".
	// If you need to test with a different cluster domain, consider refactoring the code to allow injection for testing.
	mockClusterDomain := "svc.cluster.local"
	_ = mockClusterDomain // No-op, as we cannot set it directly

	obj := metav1.ObjectMeta{
		Name:      "model",
		Namespace: "test",
		Annotations: map[string]string{
			"annotation": "annotation-value",
		},
		Labels: map[string]string{
			"label": "label-value",
		},
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "default domain template",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  "should-not-be-used",
					DomainTemplate: v1beta1.DefaultDomainTemplate,
				},
			},
			want: "model-test.cluster.local",
		},
		{
			name: "template with dot",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  "should-not-be-used",
					DomainTemplate: "{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}",
				},
			},
			want: "model.test.cluster.local",
		},
		{
			name: "template with annotation",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  "should-not-be-used",
					DomainTemplate: "{{ .Name }}.{{ .Namespace }}.{{ .Annotations.annotation }}.{{ .IngressDomain }}",
				},
			},
			want: "model.test.annotation-value.cluster.local",
		},
		{
			name: "template with label",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  "should-not-be-used",
					DomainTemplate: "{{ .Name }}.{{ .Namespace }}.{{ .Labels.label }}.{{ .IngressDomain }}",
				},
			},
			want: "model.test.label-value.cluster.local",
		},
		{
			name: "unknown variable",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  "should-not-be-used",
					DomainTemplate: "{{ .ModelName }}.{{ .Namespace }}.{{ .IngressDomain }}",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid domain name",
			args: args{
				name: "model",
				obj:  obj,
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain:  "should-not-be-used",
					DomainTemplate: "{{ .Name }}_{{ .Namespace }}_{{ .IngressDomain }}",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateInternalDomainName(tt.args.name, tt.args.obj, tt.args.ingressConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateInternalDomainName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GenerateInternalDomainName() = %v, want %v", got, tt.want)
			}
		})
	}
}
