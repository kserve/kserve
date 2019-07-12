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

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/autoscaler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	fakeK8s "k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace = "test-namespace"
	testRevision  = "test-Revision"
)

func TestUniscalerFactoryFailures(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{{
		"nil labels", nil, fmt.Sprintf("label %q not found or empty in Decider", serving.KubernetesServiceLabelKey),
	}, {
		"empty labels", map[string]string{}, fmt.Sprintf("label %q not found or empty in Decider", serving.KubernetesServiceLabelKey),
	}, {
		"config missing", map[string]string{
			serving.ServiceLabelKey:           "está la verdad",
			serving.KubernetesServiceLabelKey: "lo-digo",
		},
		fmt.Sprintf("label %q not found or empty in Decider", serving.ConfigurationLabelKey),
	}, {
		"k8s svc key is missing", map[string]string{
			serving.ConfigurationLabelKey: "blij-is",
			serving.ServiceLabelKey:       "degene-die-wijn-drinkt",
		},
		fmt.Sprintf("label %q not found or empty in Decider", serving.KubernetesServiceLabelKey),
	}, {
		"values not ascii", map[string]string{
			serving.ServiceLabelKey:           "la",
			serving.ConfigurationLabelKey:     "verité",
			serving.KubernetesServiceLabelKey: "nest-pas",
		}, "invalid value: only ASCII characters accepted",
	}, {
		"too long of a value", map[string]string{
			serving.ServiceLabelKey:           "cat is ",
			serving.ConfigurationLabelKey:     "l" + strings.Repeat("o", 253) + "ng",
			serving.KubernetesServiceLabelKey: "and-dog-is-of-proper-length",
		}, "max length must be 255 characters",
	}}

	uniScalerFactory := getTestUniScalerFactory()
	decider := &autoscaler.Decider{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testRevision,
		},
	}
	dynamicConfig := &autoscaler.DynamicConfig{}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decider.Labels = test.labels

			_, err := uniScalerFactory(decider, dynamicConfig)
			if err == nil {
				t.Fatal("No error was returned")
			}
			if got, want := err.Error(), test.want; !strings.Contains(got, want) {
				t.Errorf("Error = %q, want to contain = %q", got, want)
			}
		})
	}
}

func TestUniScalerFactoryFunc(t *testing.T) {
	uniScalerFactory := getTestUniScalerFactory()
	for _, srv := range []string{"some", ""} {
		decider := &autoscaler.Decider{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      testRevision,
				Labels: map[string]string{
					serving.RevisionLabelKey:          testRevision,
					serving.KubernetesServiceLabelKey: testRevision + "-priv",
					serving.ServiceLabelKey:           srv,
					serving.ConfigurationLabelKey:     "test-config",
				},
			},
		}
		dynamicConfig := &autoscaler.DynamicConfig{}

		if _, err := uniScalerFactory(decider, dynamicConfig); err != nil {
			t.Errorf("got error from uniScalerFactory: %v", err)
		}
	}
}

func getTestUniScalerFactory() func(decider *autoscaler.Decider, dynamicConfig *autoscaler.DynamicConfig) (autoscaler.UniScaler, error) {
	kubeClient := fakeK8s.NewSimpleClientset()
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	return uniScalerFactoryFunc(kubeInformer.Core().V1().Endpoints())
}
