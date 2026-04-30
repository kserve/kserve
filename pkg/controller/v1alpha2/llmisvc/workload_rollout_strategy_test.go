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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestApplyDeploymentRolloutStrategy(t *testing.T) {
	tests := []struct {
		name         string
		workloadSpec *v1alpha2.WorkloadSpec
		config       *Config
		expected     appsv1.DeploymentStrategy
	}{
		{
			name: "user deployment strategy takes precedence over configmap rollout",
			workloadSpec: &v1alpha2.WorkloadSpec{
				DeploymentStrategy: &appsv1.DeploymentStrategy{
					Type: appsv1.RecreateDeploymentStrategyType,
				},
			},
			config: rolloutConfig("50%", "0%"),
			expected: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
		{
			name:   "configmap rollout strategy applies when user strategy is absent",
			config: rolloutConfig("50%", "0%"),
			expected: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       intstrPtr("50%"),
					MaxUnavailable: intstrPtr("0%"),
				},
			},
		},
		{
			name: "configmap rollout strategy is ignored outside standard deployment mode",
			config: &Config{
				DeployConfig: &v1beta1.DeployConfig{
					DefaultDeploymentMode: string(constants.Knative),
					DeploymentRolloutStrategy: &v1beta1.DeploymentRolloutStrategy{
						DefaultRollout: &v1beta1.RolloutSpec{
							MaxSurge:       "50%",
							MaxUnavailable: "0%",
						},
					},
				},
			},
			expected: appsv1.DeploymentStrategy{},
		},
		{
			name:     "missing rollout config leaves strategy unchanged",
			config:   &Config{},
			expected: appsv1.DeploymentStrategy{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &appsv1.DeploymentSpec{}

			applyDeploymentRolloutStrategy(spec, tt.workloadSpec, tt.config)

			assert.Equal(t, tt.expected, spec.Strategy)
		})
	}
}

func rolloutConfig(maxSurge string, maxUnavailable string) *Config {
	return &Config{
		DeployConfig: &v1beta1.DeployConfig{
			DefaultDeploymentMode: string(constants.Standard),
			DeploymentRolloutStrategy: &v1beta1.DeploymentRolloutStrategy{
				DefaultRollout: &v1beta1.RolloutSpec{
					MaxSurge:       maxSurge,
					MaxUnavailable: maxUnavailable,
				},
			},
		},
	}
}

func intstrPtr(value string) *intstr.IntOrString {
	return &intstr.IntOrString{Type: intstr.String, StrVal: value}
}
