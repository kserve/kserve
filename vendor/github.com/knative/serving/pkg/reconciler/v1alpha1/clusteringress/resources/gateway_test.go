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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis/istio/v1alpha3"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var gateway = v1alpha3.Gateway{
	Spec: v1alpha3.GatewaySpec{
		Servers: []v1alpha3.Server{{
			Hosts: []string{"host1.example.com"},
			Port: v1alpha3.Port{
				Name:     "clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}, {
			Hosts: []string{"host2.example.com"},
			Port: v1alpha3.Port{
				Name:     "non-clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}},
	},
}

var gatewayWithPlaceholderServer = v1alpha3.Gateway{
	Spec: v1alpha3.GatewaySpec{
		Servers: []v1alpha3.Server{placeholderServer},
	},
}

var clusterIngress = v1alpha1.ClusterIngress{
	ObjectMeta: metav1.ObjectMeta{
		Name: "clusteringress",
	},
	Spec: v1alpha1.IngressSpec{
		TLS: []v1alpha1.ClusterIngressTLS{{
			Hosts:             []string{"host1.example.com"},
			SecretName:        "secret0",
			SecretNamespace:   "istio-system",
			ServerCertificate: "tls.crt",
			PrivateKey:        "tls.key",
		}},
	},
}

func TestGetServers(t *testing.T) {
	servers := GetServers(&gateway, &clusterIngress)
	expected := []v1alpha3.Server{{
		Hosts: []string{"host1.example.com"},
		Port: v1alpha3.Port{
			Name:     "clusteringress:0",
			Number:   443,
			Protocol: v1alpha3.ProtocolHTTPS,
		},
		TLS: &v1alpha3.TLSOptions{
			Mode:              v1alpha3.TLSModeSimple,
			ServerCertificate: "tls.crt",
			PrivateKey:        "tls.key",
		},
	}}

	if diff := cmp.Diff(expected, servers); diff != "" {
		t.Errorf("Unexpected servers (-want +got): %v", diff)
	}
}

func TestMakeServers(t *testing.T) {
	servers := MakeServers(&clusterIngress)
	expected := []v1alpha3.Server{{
		Hosts: []string{"host1.example.com"},
		Port: v1alpha3.Port{
			Name:     "clusteringress:0",
			Number:   443,
			Protocol: v1alpha3.ProtocolHTTPS,
		},
		TLS: &v1alpha3.TLSOptions{
			Mode:              v1alpha3.TLSModeSimple,
			ServerCertificate: "tls.crt",
			PrivateKey:        "tls.key",
		},
	}}
	if diff := cmp.Diff(expected, servers); diff != "" {
		t.Errorf("Unexpected servers (-want +got): %v", diff)
	}
}

func TestUpdateGateway(t *testing.T) {
	cases := []struct {
		name            string
		existingServers []v1alpha3.Server
		newServers      []v1alpha3.Server
		original        v1alpha3.Gateway
		expected        v1alpha3.Gateway
	}{{
		name: "Update Gateway servers.",
		existingServers: []v1alpha3.Server{{
			Hosts: []string{"host1.example.com"},
			Port: v1alpha3.Port{
				Name:     "clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}},
		newServers: []v1alpha3.Server{{
			Hosts: []string{"host-new.example.com"},
			Port: v1alpha3.Port{
				Name:     "clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}},
		original: gateway,
		expected: v1alpha3.Gateway{
			Spec: v1alpha3.GatewaySpec{
				Servers: []v1alpha3.Server{{
					// The host name was updated to the one in "newServers".
					Hosts: []string{"host-new.example.com"},
					Port: v1alpha3.Port{
						Name:     "clusteringress:0",
						Number:   443,
						Protocol: v1alpha3.ProtocolHTTPS,
					},
					TLS: &v1alpha3.TLSOptions{
						Mode:              v1alpha3.TLSModeSimple,
						ServerCertificate: "tls.crt",
						PrivateKey:        "tls.key",
					},
				}, {
					Hosts: []string{"host2.example.com"},
					Port: v1alpha3.Port{
						Name:     "non-clusteringress:0",
						Number:   443,
						Protocol: v1alpha3.ProtocolHTTPS,
					},
					TLS: &v1alpha3.TLSOptions{
						Mode:              v1alpha3.TLSModeSimple,
						ServerCertificate: "tls.crt",
						PrivateKey:        "tls.key",
					},
				}},
			},
		},
	}, {
		name: "Delete servers from Gateway",
		existingServers: []v1alpha3.Server{{
			Hosts: []string{"host1.example.com"},
			Port: v1alpha3.Port{
				Name:     "clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}},
		newServers: []v1alpha3.Server{},
		original:   gateway,
		expected: v1alpha3.Gateway{
			Spec: v1alpha3.GatewaySpec{
				// Only one server is left. The other one is deleted.
				Servers: []v1alpha3.Server{{
					Hosts: []string{"host2.example.com"},
					Port: v1alpha3.Port{
						Name:     "non-clusteringress:0",
						Number:   443,
						Protocol: v1alpha3.ProtocolHTTPS,
					},
					TLS: &v1alpha3.TLSOptions{
						Mode:              v1alpha3.TLSModeSimple,
						ServerCertificate: "tls.crt",
						PrivateKey:        "tls.key",
					},
				}},
			},
		},
	}, {
		name: "Delete servers from Gateway and no real servers are left",

		// All of the servers in the original gateway will be deleted.
		existingServers: []v1alpha3.Server{{
			Hosts: []string{"host1.example.com"},
			Port: v1alpha3.Port{
				Name:     "clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}, {
			Hosts: []string{"host2.example.com"},
			Port: v1alpha3.Port{
				Name:     "non-clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}},
		newServers: []v1alpha3.Server{},
		original:   gateway,
		expected:   gatewayWithPlaceholderServer,
	}, {
		name:            "Add servers to the gateway with only placeholder server",
		existingServers: []v1alpha3.Server{},
		newServers: []v1alpha3.Server{{
			Hosts: []string{"host1.example.com"},
			Port: v1alpha3.Port{
				Name:     "clusteringress:0",
				Number:   443,
				Protocol: v1alpha3.ProtocolHTTPS,
			},
			TLS: &v1alpha3.TLSOptions{
				Mode:              v1alpha3.TLSModeSimple,
				ServerCertificate: "tls.crt",
				PrivateKey:        "tls.key",
			},
		}},
		original: gatewayWithPlaceholderServer,
		// The placeholder server should be deleted.
		expected: v1alpha3.Gateway{
			Spec: v1alpha3.GatewaySpec{
				Servers: []v1alpha3.Server{{
					Hosts: []string{"host1.example.com"},
					Port: v1alpha3.Port{
						Name:     "clusteringress:0",
						Number:   443,
						Protocol: v1alpha3.ProtocolHTTPS,
					},
					TLS: &v1alpha3.TLSOptions{
						Mode:              v1alpha3.TLSModeSimple,
						ServerCertificate: "tls.crt",
						PrivateKey:        "tls.key",
					},
				}},
			},
		},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := UpdateGateway(&c.original, c.newServers, c.existingServers)
			if diff := cmp.Diff(&c.expected, g); diff != "" {
				t.Errorf("Unexpected gateway (-want, +got): %v", diff)
			}
		})
	}
}
