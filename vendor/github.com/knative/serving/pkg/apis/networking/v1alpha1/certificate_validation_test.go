/*
Copyright 2019 The Knative Authors

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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
)

func TestCertificateSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		cs   *CertificateSpec
		want *apis.FieldError
	}{{
		name: "valid",
		cs: &CertificateSpec{
			DNSNames:   []string{"host.example"},
			SecretName: "secret",
		},
		want: nil,
	}, {
		name: "missing-dnsnames",
		cs: &CertificateSpec{
			DNSNames:   []string{},
			SecretName: "secret",
		},
		want: apis.ErrMissingField("dnsNames"),
	}, {
		name: "empty-dnsname",
		cs: &CertificateSpec{
			DNSNames:   []string{"host.example", ""},
			SecretName: "secret",
		},
		want: apis.ErrInvalidArrayValue("", "dnsNames", 1),
	}, {
		name: "missing-secret-name",
		cs: &CertificateSpec{
			DNSNames:   []string{"host.example"},
			SecretName: "",
		},
		want: apis.ErrMissingField("secretName"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.cs.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}
func TestCertificateValidation(t *testing.T) {
	tests := []struct {
		name string
		c    *Certificate
		want *apis.FieldError
	}{{
		name: "valid",
		c: &Certificate{
			Spec: CertificateSpec{
				DNSNames:   []string{"host.example"},
				SecretName: "secret",
			},
		},
		want: nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.c.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}
