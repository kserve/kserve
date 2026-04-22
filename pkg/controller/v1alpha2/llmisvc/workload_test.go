/*
Copyright 2025 The KServe Authors.

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

package llmisvc

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestMigrateRoutingSidecarCommand(t *testing.T) {
	tests := []struct {
		name            string
		command         []string
		expectedCommand []string
	}{
		{
			name:            "renames --connector=value to --kv-connector=value",
			command:         []string{"/app/pd-sidecar", "--port=8000", "--connector=nixlv2", "--enable-ssrf-protection=true"},
			expectedCommand: []string{"/app/pd-sidecar", "--port=8000", "--kv-connector=nixlv2", "--enable-ssrf-protection=true"},
		},
		{
			name:            "renames --connector value (separate) to --kv-connector value",
			command:         []string{"/app/pd-sidecar", "--port=8000", "--connector", "nixlv2", "--enable-ssrf-protection=true"},
			expectedCommand: []string{"/app/pd-sidecar", "--port=8000", "--kv-connector", "nixlv2", "--enable-ssrf-protection=true"},
		},
		{
			name:            "already uses --kv-connector - no change",
			command:         []string{"/app/pd-sidecar", "--port=8000", "--kv-connector=nixlv2", "--enable-ssrf-protection=true"},
			expectedCommand: []string{"/app/pd-sidecar", "--port=8000", "--kv-connector=nixlv2", "--enable-ssrf-protection=true"},
		},
		{
			name:            "no --connector flag at all - no change",
			command:         []string{"/app/pd-sidecar", "--port=8000", "--enable-ssrf-protection=true"},
			expectedCommand: []string{"/app/pd-sidecar", "--port=8000", "--enable-ssrf-protection=true"},
		},
		{
			name:            "--connector as last arg (separate form, no value follows)",
			command:         []string{"/app/pd-sidecar", "--connector"},
			expectedCommand: []string{"/app/pd-sidecar", "--connector"},
		},
		{
			name:            "empty command - no panic",
			command:         []string{},
			expectedCommand: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			c := &corev1.Container{
				Name:    routingSidecarContainerName,
				Command: tt.command,
			}
			migrateRoutingSidecarCommand(c)
			g.Expect(c.Command).To(Equal(tt.expectedCommand))
		})
	}
}

func TestRoutingSidecarMigrationOnOldPodSpec(t *testing.T) {
	// Realistic old v0.6 PodSpec with --connector in the routing sidecar's Command.
	oldPodSpec := corev1.PodSpec{
		InitContainers: []corev1.Container{
			{
				Name: routingSidecarContainerName,
				Command: []string{
					"/app/pd-sidecar",
					"--port=8000",
					"--vllm-port=8001",
					"--connector=nixlv2",
					"--enable-ssrf-protection=true",
					"--pool-group=inference.networking.x-k8s.io",
				},
			},
		},
		Containers: []corev1.Container{
			{Name: "main", Command: []string{"/app/vllm"}},
		},
	}

	tests := []struct {
		name     string
		podSpec  corev1.PodSpec
		validate func(g Gomega, pod corev1.PodSpec)
	}{
		{
			name:    "old config with --connector gets migrated",
			podSpec: *oldPodSpec.DeepCopy(),
			validate: func(g Gomega, pod corev1.PodSpec) {
				s := routingSidecar(&pod)
				g.Expect(s).NotTo(BeNil())
				g.Expect(s.Command).To(ContainElement("--kv-connector=nixlv2"))
				g.Expect(s.Command).NotTo(ContainElement("--connector=nixlv2"))
				// Other flags preserved
				g.Expect(s.Command).To(ContainElement("--port=8000"))
				g.Expect(s.Command).To(ContainElement("--vllm-port=8001"))
				g.Expect(s.Command).To(ContainElement("--enable-ssrf-protection=true"))
			},
		},
		{
			name: "new config with --kv-connector left untouched",
			podSpec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: routingSidecarContainerName,
						Command: []string{
							"/app/pd-sidecar",
							"--port=8000",
							"--kv-connector=nixlv2",
							"--enable-ssrf-protection=true",
						},
					},
				},
				Containers: []corev1.Container{
					{Name: "main"},
				},
			},
			validate: func(g Gomega, pod corev1.PodSpec) {
				s := routingSidecar(&pod)
				g.Expect(s).NotTo(BeNil())
				g.Expect(s.Command).To(ContainElement("--kv-connector=nixlv2"))
				g.Expect(s.Command).To(HaveLen(4))
			},
		},
		{
			name: "no routing sidecar - no panic",
			podSpec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main"},
				},
			},
			validate: func(g Gomega, pod corev1.PodSpec) {
				g.Expect(routingSidecar(&pod)).To(BeNil())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			s := routingSidecar(&tt.podSpec)
			if s != nil {
				migrateRoutingSidecarCommand(s)
			}

			tt.validate(g, tt.podSpec)
		})
	}
}
