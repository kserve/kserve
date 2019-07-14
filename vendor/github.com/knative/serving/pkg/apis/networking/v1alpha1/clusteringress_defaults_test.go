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

package v1alpha1

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/knative/serving/pkg/apis/networking"
)

func TestClusterIngressDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *ClusterIngress
		want *ClusterIngress
	}{{
		name: "empty",
		in:   &ClusterIngress{},
		want: &ClusterIngress{
			Spec: IngressSpec{
				Visibility: IngressVisibilityExternalIP,
			},
		},
	}, {
		name: "has-visibility",
		in: &ClusterIngress{
			Spec: IngressSpec{
				Visibility: IngressVisibilityClusterLocal,
			},
		},
		want: &ClusterIngress{
			Spec: IngressSpec{
				Visibility: IngressVisibilityClusterLocal,
			},
		},
	}, {
		name: "tls-defaulting",
		in: &ClusterIngress{
			Spec: IngressSpec{
				TLS: []ClusterIngressTLS{{
					SecretNamespace: "secret-space",
					SecretName:      "secret-name",
				}},
			},
		},
		want: &ClusterIngress{
			Spec: IngressSpec{
				TLS: []ClusterIngressTLS{{
					SecretNamespace: "secret-space",
					SecretName:      "secret-name",
					// Default secret keys are filled in.
					ServerCertificate: "tls.crt",
					PrivateKey:        "tls.key",
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
	}, {
		name: "tls-not-defaulting",
		in: &ClusterIngress{
			Spec: IngressSpec{
				TLS: []ClusterIngressTLS{{
					SecretNamespace:   "secret-space",
					SecretName:        "secret-name",
					ServerCertificate: "custom.tls.cert",
					PrivateKey:        "custom.tls.key",
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
		want: &ClusterIngress{
			Spec: IngressSpec{
				TLS: []ClusterIngressTLS{{
					SecretNamespace: "secret-space",
					SecretName:      "secret-name",
					// Default secret keys are kept intact.
					ServerCertificate: "custom.tls.cert",
					PrivateKey:        "custom.tls.key",
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
	}, {
		name: "split-timeout-retry-defaulting",
		in: &ClusterIngress{
			Spec: IngressSpec{
				Rules: []ClusterIngressRule{{
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
							}},
						}},
					},
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
		want: &ClusterIngress{
			Spec: IngressSpec{
				Rules: []ClusterIngressRule{{
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								// Percent is filled in.
								Percent: 100,
							}},
							// Timeout and Retries are filled in.
							Timeout: &metav1.Duration{Duration: networking.DefaultTimeout},
							Retries: &HTTPRetry{
								PerTryTimeout: &metav1.Duration{Duration: networking.DefaultTimeout},
								Attempts:      networking.DefaultRetryCount,
							},
						}},
					},
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
	}, {
		name: "split-timeout-retry-not-defaulting",
		in: &ClusterIngress{
			Spec: IngressSpec{
				Rules: []ClusterIngressRule{{
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								Percent: 30,
							}, {
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-001",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								Percent: 70,
							}},
							Timeout: &metav1.Duration{Duration: 10 * time.Second},
							Retries: &HTTPRetry{
								PerTryTimeout: &metav1.Duration{Duration: 10 * time.Second},
								Attempts:      2,
							},
						}},
					},
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
		want: &ClusterIngress{
			Spec: IngressSpec{
				Rules: []ClusterIngressRule{{
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								// Percent is kept intact.
								Percent: 30,
							}, {
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-001",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								// Percent is kept intact.
								Percent: 70,
							}},
							// Timeout and Retries are kept intact.
							Timeout: &metav1.Duration{Duration: 10 * time.Second},
							Retries: &HTTPRetry{
								PerTryTimeout: &metav1.Duration{Duration: 10 * time.Second},
								Attempts:      2,
							},
						}},
					},
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
	}, {
		name: "perTryTimeout-in-retry-defaulting",
		in: &ClusterIngress{
			Spec: IngressSpec{
				Rules: []ClusterIngressRule{{
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								Percent: 30,
							}, {
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-001",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								Percent: 70,
							}},
							Timeout: &metav1.Duration{Duration: 10 * time.Second},
							Retries: &HTTPRetry{
								Attempts: 2,
							},
						}},
					},
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
		want: &ClusterIngress{
			Spec: IngressSpec{
				Rules: []ClusterIngressRule{{
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								// Percent is kept intact.
								Percent: 30,
							}, {
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-001",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
								// Percent is kept intact.
								Percent: 70,
							}},
							// Timeout and Retries are kept intact.
							Timeout: &metav1.Duration{Duration: 10 * time.Second},
							Retries: &HTTPRetry{
								// PerTryTimeout is filled in.
								PerTryTimeout: &metav1.Duration{Duration: networking.DefaultTimeout},
								Attempts:      2,
							},
						}},
					},
				}},
				Visibility: IngressVisibilityExternalIP,
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.SetDefaults(context.Background())
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("SetDefaults (-want, +got) = %v", diff)
			}
		})
	}

}
