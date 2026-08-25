/*
Copyright 2023 The KServe Authors.

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
)

func TestGenerateUrlPath(t *testing.T) {
	type args struct {
		name          string
		namespace     string
		ingressConfig *v1beta1.IngressConfig
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "empty path template",
			args: args{
				name:      "model",
				namespace: "user",
				ingressConfig: &v1beta1.IngressConfig{
					IngressDomain: "my.domain",
				},
			},
			want: "",
		},
		{
			name: "valid path template",
			args: args{
				name:      "model",
				namespace: "user",
				ingressConfig: &v1beta1.IngressConfig{
					PathTemplate: "/path/to/{{ .Namespace }}/{{ .Name }}",
				},
			},
			want: "/path/to/user/model",
		},
		{
			name: "invalid path template (not parsable)",
			args: args{
				name:      "model",
				namespace: "user",
				ingressConfig: &v1beta1.IngressConfig{
					UrlScheme:     "https",
					IngressDomain: "my.domain",
					PathTemplate:  "/{{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid path template (unknown keys)",
			args: args{
				name:      "model",
				namespace: "user",
				ingressConfig: &v1beta1.IngressConfig{
					UrlScheme:     "https",
					IngressDomain: "my.domain",
					PathTemplate:  "/{{ .Unknownfield }}/serving/{{ .Namespace }}/{{ .Name }}",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid path template (with host)",
			args: args{
				name:      "model",
				namespace: "user",
				ingressConfig: &v1beta1.IngressConfig{
					UrlScheme:     "https",
					IngressDomain: "my.domain",
					PathTemplate:  "myhost/serving/{{ .Namespace }}/{{ .Name }}",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid path template (with scheme)",
			args: args{
				name:      "model",
				namespace: "user",
				ingressConfig: &v1beta1.IngressConfig{
					UrlScheme:     "https",
					IngressDomain: "my.domain",
					PathTemplate:  "http://myhost/serving/{{ .Namespace }}/{{ .Name }}",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateUrlPath(tt.args.name, tt.args.namespace, tt.args.ingressConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateUrlPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Test %q unexpected url (-want +got): %v", tt.name, diff)
			}
		})
	}
}
