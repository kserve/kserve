/*
Copyright 2026 The KServe Authors.

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

package v1beta1

import (
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

// TestPodSpecWorkloadRefRoundTrip verifies that WorkloadRef, which kserve's PodSpec
// mirrors from corev1.PodSpec (added in k8s.io/api v0.35), survives the direct
// corev1.PodSpec(*podSpec) cast performed by the custom component constructors.
//
// The cast relies on kserve's PodSpec being structurally identical to corev1.PodSpec.
// A compile error catches a missing field, but a type mismatch on WorkloadRef would
// silently fail the cast (or drop data) without this round-trip assertion.
func TestPodSpecWorkloadRefRoundTrip(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	workloadRef := &corev1.WorkloadReference{
		Name:               "my-workload",
		PodGroup:           "my-pod-group",
		PodGroupReplicaKey: "replica-0",
	}

	podSpec := &PodSpec{
		WorkloadRef:        workloadRef,
		ServiceAccountName: "test-sa",
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	scenarios := map[string]func(*PodSpec) corev1.PodSpec{
		"CustomPredictor":   func(p *PodSpec) corev1.PodSpec { return NewCustomPredictor(p).PodSpec },
		"CustomExplainer":   func(p *PodSpec) corev1.PodSpec { return NewCustomExplainer(p).PodSpec },
		"CustomTransformer": func(p *PodSpec) corev1.PodSpec { return NewCustomTransformer(p).PodSpec },
	}

	for name, cast := range scenarios {
		t.Run(name, func(t *testing.T) {
			got := cast(podSpec)
			g.Expect(got.WorkloadRef).To(gomega.Equal(workloadRef),
				"WorkloadRef must be preserved through the corev1.PodSpec cast")
		})
	}
}
