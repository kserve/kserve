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
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestApplyDeploymentRolloutStrategy(t *testing.T) {
	t.Run("nil workload leaves deployment strategy empty", func(t *testing.T) {
		d := &appsv1.Deployment{}
		applyDeploymentRolloutStrategy(d, nil)
		assert.Equal(t, appsv1.DeploymentStrategy{}, d.Spec.Strategy)
	})

	t.Run("nil rollout strategy leaves deployment strategy empty", func(t *testing.T) {
		d := &appsv1.Deployment{}
		applyDeploymentRolloutStrategy(d, &v1alpha2.WorkloadSpec{})
		assert.Equal(t, appsv1.DeploymentStrategy{}, d.Spec.Strategy)
	})

	t.Run("empty rollout strategy leaves deployment strategy empty", func(t *testing.T) {
		d := &appsv1.Deployment{}
		applyDeploymentRolloutStrategy(d, &v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{},
		})
		assert.Equal(t, appsv1.DeploymentStrategy{}, d.Spec.Strategy)
	})

	t.Run("maxUnavailable only sets rolling update strategy", func(t *testing.T) {
		d := &appsv1.Deployment{}
		mu := intstr.FromInt32(1)
		applyDeploymentRolloutStrategy(d, &v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{
				MaxUnavailable: &mu,
			},
		})
		assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, d.Spec.Strategy.Type)
		require.NotNil(t, d.Spec.Strategy.RollingUpdate)
		assert.Equal(t, intstr.FromInt32(1), *d.Spec.Strategy.RollingUpdate.MaxUnavailable)
		assert.Nil(t, d.Spec.Strategy.RollingUpdate.MaxSurge)
	})

	t.Run("maxSurge only sets rolling update strategy", func(t *testing.T) {
		d := &appsv1.Deployment{}
		ms := intstr.FromInt32(2)
		applyDeploymentRolloutStrategy(d, &v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{
				MaxSurge: &ms,
			},
		})
		assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, d.Spec.Strategy.Type)
		require.NotNil(t, d.Spec.Strategy.RollingUpdate)
		assert.Nil(t, d.Spec.Strategy.RollingUpdate.MaxUnavailable)
		assert.Equal(t, intstr.FromInt32(2), *d.Spec.Strategy.RollingUpdate.MaxSurge)
	})

	t.Run("both fields set", func(t *testing.T) {
		d := &appsv1.Deployment{}
		mu := intstr.FromInt32(1)
		ms := intstr.FromString("50%")
		applyDeploymentRolloutStrategy(d, &v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{
				MaxUnavailable: &mu,
				MaxSurge:       &ms,
			},
		})
		assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, d.Spec.Strategy.Type)
		require.NotNil(t, d.Spec.Strategy.RollingUpdate)
		assert.Equal(t, intstr.FromInt32(1), *d.Spec.Strategy.RollingUpdate.MaxUnavailable)
		assert.Equal(t, intstr.FromString("50%"), *d.Spec.Strategy.RollingUpdate.MaxSurge)
	})
}

func TestRollingUpdateConfigFromWorkloadSpec(t *testing.T) {
	t.Run("nil workload returns nil", func(t *testing.T) {
		assert.Nil(t, rollingUpdateConfigFromWorkloadSpec(nil))
	})

	t.Run("nil rollout strategy returns nil", func(t *testing.T) {
		assert.Nil(t, rollingUpdateConfigFromWorkloadSpec(&v1alpha2.WorkloadSpec{}))
	})

	t.Run("empty rollout strategy returns nil", func(t *testing.T) {
		assert.Nil(t, rollingUpdateConfigFromWorkloadSpec(&v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{},
		}))
	})

	t.Run("maxUnavailable only", func(t *testing.T) {
		mu := intstr.FromInt32(1)
		config := rollingUpdateConfigFromWorkloadSpec(&v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{
				MaxUnavailable: &mu,
			},
		})
		require.NotNil(t, config)
		assert.Equal(t, intstr.FromInt32(1), config.MaxUnavailable)
		assert.Equal(t, intstr.FromInt32(0), config.MaxSurge)
	})

	t.Run("maxSurge only", func(t *testing.T) {
		ms := intstr.FromInt32(2)
		config := rollingUpdateConfigFromWorkloadSpec(&v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{
				MaxSurge: &ms,
			},
		})
		require.NotNil(t, config)
		assert.Equal(t, intstr.FromInt32(1), config.MaxUnavailable)
		assert.Equal(t, intstr.FromInt32(2), config.MaxSurge)
	})

	t.Run("both fields", func(t *testing.T) {
		mu := intstr.FromInt32(1)
		ms := intstr.FromString("100%")
		config := rollingUpdateConfigFromWorkloadSpec(&v1alpha2.WorkloadSpec{
			RolloutStrategy: &v1alpha2.RolloutStrategy{
				MaxUnavailable: &mu,
				MaxSurge:       &ms,
			},
		})
		require.NotNil(t, config)
		assert.Equal(t, intstr.FromInt32(1), config.MaxUnavailable)
		assert.Equal(t, intstr.FromString("100%"), config.MaxSurge)
	})
}
