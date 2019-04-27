/*
Copyright 2019 The Knative Authors.

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
	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/kmeta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Certificate is responsible for provisioning a SSL certificate for the
// given hosts. It is a Knative abstraction for various SSL certificate
// provisioning solutions (such as cert-manager or self-signed SSL certificate).
type Certificate struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the Certificate.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec CertificateSpec `json:"spec,omitempty"`

	// Status is the current state of the Certificate.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status CertificateStatus `json:"status,omitempty"`
}

// Verify that Certificate adheres to the appropriate interfaces.
var (
	// Check that Certificate may be validated and defaulted.
	_ apis.Validatable = (*Certificate)(nil)
	_ apis.Defaultable = (*Certificate)(nil)

	// Check that we can create OwnerReferences to a Certificate..
	_ kmeta.OwnerRefable = (*Certificate)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CertificateList is a collection of `Certificate`.
type CertificateList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of `Certificate`.
	Items []Certificate `json:"items"`
}

// CertificateSpec defines the desired state of a `Certificate`.
type CertificateSpec struct {
	// DNSNames is a list of DNS names the Certificate could support.
	// The wildcard format of DNSNames (e.g. *.default.example.com) is supported.
	DNSNames []string `json:"dnsNames"`

	// SecretName is the name of the secret resource to store the SSL certificate in.
	SecretName string `json:"secretName"`
}

// CertificateStatus defines the observed state of a `Certificate`.
type CertificateStatus struct {
	duckv1beta1.Status `json:",inline"`

	// The expiration time of the TLS certificate stored in the secret named
	// by this resource in spec.secretName.
	// +optional
	NotAfter *metav1.Time `json:"notAfter,omitempty"`
}
