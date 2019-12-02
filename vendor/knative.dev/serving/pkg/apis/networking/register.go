/*
Copyright 2018 The Knative Authors

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

package networking

const (
	// GroupName is the name for the networking API group.
	GroupName = "networking.internal.knative.dev"

	// IngressClassAnnotationKey is the annotation for the
	// explicit class of ClusterIngress that a particular resource has
	// opted into. For example,
	//
	//    networking.knative.dev/ingress.class: some-network-impl
	//
	// This uses a different domain because unlike the resource, it is
	// user-facing.
	//
	// The parent resource may use its own annotations to choose the
	// annotation value for the ClusterIngress it uses.  Based on such
	// value a different reconciliation logic may be used (for examples,
	// Istio-based ClusterIngress will reconcile into a VirtualService).
	IngressClassAnnotationKey = "networking.knative.dev/ingress.class"

	// ClusterIngressLabelKey is the label key attached to underlying network programming
	// resources to indicate which ClusterIngress triggered their creation.
	ClusterIngressLabelKey = GroupName + "/clusteringress"

	// IngressLabelKey is the label key attached to underlying network programming
	// resources to indicate which Ingress triggered their creation.
	IngressLabelKey = GroupName + "/ingress"

	// SKSLabelKey is the label key that SKS Controller attaches to the
	// underlying resources it controls.
	SKSLabelKey = GroupName + "/serverlessservice"

	// ServiceTypeKey is the label key attached to a service specifying the type of service.
	// e.g. Public, Metrics
	ServiceTypeKey = GroupName + "/serviceType"

	// OriginSecretNameLabelKey is the label key attached to the TLS secret to indicate
	// the name of the origin secret that the TLS secret is copied from.
	OriginSecretNameLabelKey = GroupName + "/originSecretName"

	// OriginSecretNamespaceLabelKey is the label key attached to the TLS secret
	// to indicate the namespace of the origin secret that the TLS secret is copied from.
	OriginSecretNamespaceLabelKey = GroupName + "/originSecretNamespace"

	// CertificateClassAnnotationKey is the annotation for the
	// explicit class of Certificate that a particular resource has
	// opted into. For example,
	//
	//    networking.internal.knative.dev/certificate.class: some-network-impl
	//
	// This uses a different domain because unlike the resource, it is
	// user-facing.
	//
	// The parent resource may use its own annotations to choose the
	// annotation value for the Certificate it uses.  Based on such
	// value a different reconciliation logic may be used (for examples,
	// Cert-Manager-based Certificate will reconcile into a Cert-Manager Certificate).
	CertificateClassAnnotationKey = GroupName + "/certificate.class"

	// ActivatorServiceName is the name of the activator Kubernetes service.
	ActivatorServiceName = "activator-service"

	// DisableWildcardCertLabelKey is the label key attached to a namespace to indicate that
	// a wildcard certificate should be not created for it.
	DisableWildcardCertLabelKey = GroupName + "/disableWildcardCert"

	// WildcardCertDomainLabelKey is the label key attached to a certificate to indicate the
	// domain for which it was issued.
	WildcardCertDomainLabelKey = "networking.knative.dev/wildcardDomain"
)

// ServiceType is the enumeration type for the Kubernetes services
// that we have in our system, classified by usage purpose.
type ServiceType string

const (
	// ServiceTypePrivate is the label value for internal only services
	// for user applications.
	ServiceTypePrivate ServiceType = "Private"
	// ServiceTypePublic is the label value for externally reachable
	// services for user applications.
	ServiceTypePublic ServiceType = "Public"
	// ServiceTypeMetrics is the label value for Metrics services. Such services
	// are used for meric scraping.
	ServiceTypeMetrics ServiceType = "Metrics"
)

// Pseudo-constants
var (
	// DefaultRetryCount will be set if Attempts not specified.
	DefaultRetryCount = 3
)
