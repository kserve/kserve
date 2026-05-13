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

package validation

import (
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestValidateBlockedEnvVars(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := map[string]struct {
		containers  []corev1.Container
		blockedVars []string
		expectErr   bool
		errContains string
	}{
		"no containers": {
			containers:  []corev1.Container{},
			blockedVars: DefaultBlockedEnvVars,
			expectErr:   false,
		},
		"no blocked vars": {
			containers: []corev1.Container{
				{
					Name: "kserve-container",
					Env: []corev1.EnvVar{
						{Name: "PYTHONPATH", Value: "/custom"},
					},
				},
			},
			blockedVars: []string{},
			expectErr:   false,
		},
		"allowed env vars pass": {
			containers: []corev1.Container{
				{
					Name: "kserve-container",
					Env: []corev1.EnvVar{
						{Name: "MODEL_NAME", Value: "my-model"},
						{Name: "HTTP_PORT", Value: "8080"},
					},
				},
			},
			blockedVars: DefaultBlockedEnvVars,
			expectErr:   false,
		},
		"PYTHONPATH blocked in single container": {
			containers: []corev1.Container{
				{
					Name: "kserve-container",
					Env: []corev1.EnvVar{
						{Name: "PYTHONPATH", Value: "/malicious/path"},
					},
				},
			},
			blockedVars: DefaultBlockedEnvVars,
			expectErr:   true,
			errContains: "PYTHONPATH",
		},
		"PYTHONPATH blocked in second container": {
			containers: []corev1.Container{
				{
					Name: "init-container",
					Env: []corev1.EnvVar{
						{Name: "MODEL_NAME", Value: "ok"},
					},
				},
				{
					Name: "sidecar",
					Env: []corev1.EnvVar{
						{Name: "PYTHONPATH", Value: "/injected"},
					},
				},
			},
			blockedVars: DefaultBlockedEnvVars,
			expectErr:   true,
			errContains: "sidecar",
		},
		"multiple blocked vars": {
			containers: []corev1.Container{
				{
					Name: "kserve-container",
					Env: []corev1.EnvVar{
						{Name: "LD_PRELOAD", Value: "/lib/inject.so"},
					},
				},
			},
			blockedVars: []string{"PYTHONPATH", "LD_PRELOAD"},
			expectErr:   true,
			errContains: "LD_PRELOAD",
		},
		"container with no env vars passes": {
			containers: []corev1.Container{
				{Name: "kserve-container"},
			},
			blockedVars: DefaultBlockedEnvVars,
			expectErr:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateBlockedEnvVars(tc.containers, tc.blockedVars)
			if tc.expectErr {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(err.Error()).To(gomega.ContainSubstring(tc.errContains))
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}
