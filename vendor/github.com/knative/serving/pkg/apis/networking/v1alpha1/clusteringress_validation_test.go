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

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIngressSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		cis  *IngressSpec
		want *apis.FieldError
	}{{
		name: "valid",
		cis: &IngressSpec{
			TLS: []ClusterIngressTLS{{
				SecretNamespace: "secret-space",
				SecretName:      "secret-name",
			}},
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceName:      "revision-000",
								ServiceNamespace: "default",
								ServicePort:      intstr.FromInt(8080),
							},
						}},
						Retries: &HTTPRetry{
							Attempts: 3,
						},
					}},
				},
			}},
		},
		want: nil,
	}, {
		name: "empty",
		cis:  &IngressSpec{},
		want: apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "missing-rule",
		cis: &IngressSpec{
			TLS: []ClusterIngressTLS{{
				SecretName:      "secret-name",
				SecretNamespace: "secret-namespace",
			}},
		},
		want: apis.ErrMissingField("rules"),
	}, {
		name: "empty-rule",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{}},
		},
		want: apis.ErrMissingField("rules[0]"),
	}, {
		name: "missing-http",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
			}},
		},
		want: apis.ErrMissingField("rules[0].http"),
	}, {
		name: "missing-http-paths",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP:  &HTTPClusterIngressRuleValue{},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths"),
	}, {
		name: "empty-http-paths",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0]"),
	}, {
		name: "backend-wrong-percentage",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceName:      "revision-000",
								ServiceNamespace: "default",
								ServicePort:      intstr.FromInt(8080),
							},
							Percent: 199,
						}},
					}},
				},
			}},
		},
		want: apis.ErrInvalidValue(199, "rules[0].http.paths[0].splits[0].percent"),
	}, {
		name: "missing-split",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{},
						AppendHeaders: map[string]string{
							"foo": "bar",
						},
					}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0].splits"),
	}, {
		name: "empty-split",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{}},
						AppendHeaders: map[string]string{
							"foo": "bar",
						},
					}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0].splits[0]"),
	}, {
		name: "missing-split-backend",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{},
							Percent:               100,
						}},
					}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0].splits[0]"),
	}, {
		name: "missing-backend-name",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceNamespace: "default",
								ServicePort:      intstr.FromInt(8080),
							},
						}},
					}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0].splits[0].serviceName"),
	}, {
		name: "missing-backend-namespace",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceName: "service-name",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0].splits[0].serviceNamespace"),
	}, {
		name: "missing-backend-port",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceName:      "service-name",
								ServiceNamespace: "default",
							},
						}},
					}},
				},
			}},
		},
		want: apis.ErrMissingField("rules[0].http.paths[0].splits[0].servicePort"),
	}, {
		name: "split-percent-sum-not-100",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceName:      "revision-000",
								ServiceNamespace: "default",
								ServicePort:      intstr.FromInt(8080),
							},
							Percent: 30,
						}},
					}},
				},
			}},
		},
		want: &apis.FieldError{
			Message: "Traffic split percentage must total to 100, but was 30",
			Paths:   []string{"rules[0].http.paths[0].splits"},
		},
	}, {
		name: "wrong-retry-attempts",
		cis: &IngressSpec{
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
				HTTP: &HTTPClusterIngressRuleValue{
					Paths: []HTTPClusterIngressPath{{
						Splits: []ClusterIngressBackendSplit{{
							ClusterIngressBackend: ClusterIngressBackend{
								ServiceName:      "revision-000",
								ServiceNamespace: "default",
								ServicePort:      intstr.FromInt(8080),
							},
						}},
						Retries: &HTTPRetry{
							Attempts: -1,
						},
					}},
				},
			}},
		},
		want: apis.ErrInvalidValue(-1, "rules[0].http.paths[0].retries.attempts"),
	}, {
		name: "empty-tls",
		cis: &IngressSpec{
			TLS: []ClusterIngressTLS{{}},
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
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
		},
		want: apis.ErrMissingField("tls[0]"),
	}, {
		name: "missing-tls-secret-namespace",
		cis: &IngressSpec{
			TLS: []ClusterIngressTLS{{
				SecretName: "secret",
			}},
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
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
		},
		want: apis.ErrMissingField("tls[0].secretNamespace"),
	}, {
		name: "missing-tls-secret-name",
		cis: &IngressSpec{
			TLS: []ClusterIngressTLS{{
				SecretNamespace: "secret-space",
			}},
			Rules: []ClusterIngressRule{{
				Hosts: []string{"example.com"},
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
		},
		want: apis.ErrMissingField("tls[0].secretName"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.cis.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}
func TestClusterIngressValidation(t *testing.T) {
	tests := []struct {
		name string
		ci   *ClusterIngress
		want *apis.FieldError
	}{{
		name: "valid",
		ci: &ClusterIngress{
			Spec: IngressSpec{
				TLS: []ClusterIngressTLS{{
					SecretNamespace: "secret-space",
					SecretName:      "secret-name",
				}},
				Rules: []ClusterIngressRule{{
					Hosts: []string{"example.com"},
					HTTP: &HTTPClusterIngressRuleValue{
						Paths: []HTTPClusterIngressPath{{
							Splits: []ClusterIngressBackendSplit{{
								ClusterIngressBackend: ClusterIngressBackend{
									ServiceName:      "revision-000",
									ServiceNamespace: "default",
									ServicePort:      intstr.FromInt(8080),
								},
							}},
							Retries: &HTTPRetry{
								Attempts: 3,
							},
						}},
					},
				}},
			},
		},
		want: nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.ci.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}
