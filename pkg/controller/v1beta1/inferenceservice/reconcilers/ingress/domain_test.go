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
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateDomainName(t *testing.T) {
	type args struct {
		name          string
		obj           v1.ObjectMeta
		ingressConfig *v1beta1.IngressConfig
	}

	obj := v1.ObjectMeta{
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
