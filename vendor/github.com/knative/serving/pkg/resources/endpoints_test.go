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

package resources

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace = "test-namespace"
	testService   = "test-service"
)

func TestFetchReadyAddressCount(t *testing.T) {
	kubeClient := fakek8s.NewSimpleClientset()
	endpointsClient := kubeinformers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Endpoints()
	createEndpoints := func(ep *corev1.Endpoints) {
		kubeClient.CoreV1().Endpoints(testNamespace).Create(ep)
		endpointsClient.Informer().GetIndexer().Add(ep)
	}

	tests := []struct {
		name      string
		endpoints *corev1.Endpoints
		want      int
	}{{
		name:      "no endpoints at all",
		endpoints: nil,
		want:      0,
	}, {
		name:      "no ready addresses",
		endpoints: endpoints(0),
		want:      0,
	}, {
		name:      "one ready address",
		endpoints: endpoints(1),
		want:      1,
	}, {
		name:      "ten ready addresses",
		endpoints: endpoints(10),
		want:      10,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.endpoints != nil {
				createEndpoints(test.endpoints)
			}
			if got, _ := FetchReadyAddressCount(endpointsClient.Lister(), testNamespace, testService); got != test.want {
				t.Errorf("ReadyAddressCount() = %d, want: %d", got, test.want)
			}
		})
	}
}

func TestReadyAddressCount(t *testing.T) {
	tests := []struct {
		name      string
		endpoints *corev1.Endpoints
		want      int
	}{{
		name:      "no ready addresses",
		endpoints: endpoints(0),
		want:      0,
	}, {
		name:      "one ready address",
		endpoints: endpoints(1),
		want:      1,
	}, {
		name:      "ten ready addresses",
		endpoints: endpoints(10),
		want:      10,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ReadyAddressCount(test.endpoints); got != test.want {
				t.Errorf("ReadyAddressCount() = %d, want: %d", got, test.want)
			}
		})
	}
}

func endpoints(ipCount int) *corev1.Endpoints {
	ep := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testService,
		},
	}
	addresses := make([]corev1.EndpointAddress, ipCount)
	for i := 0; i < ipCount; i++ {
		addresses[i] = corev1.EndpointAddress{IP: fmt.Sprintf("127.0.0.%v", i+1)}
	}
	ep.Subsets = []corev1.EndpointSubset{{
		Addresses: addresses,
	}}
	return ep
}

func TestParentResourceFromService(t *testing.T) {
	tests := map[string]string{
		"":      "",
		"a":     "a",
		"a-":    "a",
		"a-b":   "a",
		"a-b-c": "a-b",
	}
	for in, want := range tests {
		if got := ParentResourceFromService(in); got != want {
			t.Errorf("%s => got: %s, want: %s", in, got, want)
		}
	}
}
