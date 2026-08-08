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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const testNamespace = "default"

func testReconcilerWithNamespace(t *testing.T) *LLMISVCReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
	return &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build(),
	}
}

func findEnvVar(envs []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

func TestInjectServedByMiddleware(t *testing.T) {
	t.Run("injects when annotation is true", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "llama-70b-v1",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "true"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Args: []string{"--model", "llama-70b"}},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		env := findEnvVar(podSpec.Containers[0].Env, servedByEnvVar)
		require.NotNil(t, env, "KSERVE_SERVED_BY should be set")
		assert.Equal(t, "llama-70b-v1", env.Value)

		env = findEnvVar(podSpec.Containers[0].Env, servedByMiddlewareClassVar)
		require.NotNil(t, env, "KSERVE_MIDDLEWARE_CLASS should be set")
		assert.Equal(t, servedByMiddlewareClass, env.Value)

		env = findEnvVar(podSpec.Containers[0].Env, servedByPythonPathVar)
		require.NotNil(t, env, "PYTHONPATH should be set")
		assert.Equal(t, servedByPythonPath+":$("+servedByPythonPathVar+")", env.Value)

		assert.Len(t, podSpec.Volumes, 1)
		assert.Len(t, podSpec.Containers[0].VolumeMounts, 1)

		cond := llmSvc.GetStatus().GetCondition(v1alpha2.RequestAttributionReady)
		require.NotNil(t, cond, "RequestAttributionReady should be set")
		assert.Equal(t, "True", string(cond.Status))
	})

	t.Run("skips when annotation absent", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "solo", Namespace: testNamespace},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Args: []string{"--model", "llama-70b"}},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		assert.Nil(t, findEnvVar(podSpec.Containers[0].Env, servedByMiddlewareClassVar))
		assert.Empty(t, podSpec.Volumes)
	})

	t.Run("skips when annotation is false", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "v1",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "false"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main"}},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		assert.Nil(t, findEnvVar(podSpec.Containers[0].Env, servedByMiddlewareClassVar))
	})

	t.Run("does not duplicate env vars if already present", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "v1",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "true"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Env: []corev1.EnvVar{
						{Name: servedByMiddlewareClassVar, Value: servedByMiddlewareClass},
					},
				},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		count := 0
		for _, env := range podSpec.Containers[0].Env {
			if env.Name == servedByMiddlewareClassVar {
				count++
			}
		}
		assert.Equal(t, 1, count, "should not duplicate KSERVE_MIDDLEWARE_CLASS")
	})

	t.Run("overwrites existing env var including ValueFrom", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "v2",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "true"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Env: []corev1.EnvVar{
						{
							Name:      servedByEnvVar,
							ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
						},
					},
				},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		env := findEnvVar(podSpec.Containers[0].Env, servedByEnvVar)
		require.NotNil(t, env)
		assert.Equal(t, "v2", env.Value)
		assert.Nil(t, env.ValueFrom, "ValueFrom should be cleared when overwriting with literal value")
	})

	t.Run("no-op if container not found - no volume added", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "v1",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "true"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "other-container"},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		assert.Empty(t, podSpec.Containers[0].Args)
		assert.Empty(t, podSpec.Containers[0].Env)
		assert.Empty(t, podSpec.Volumes, "volume should not be added when container is missing")
	})

	t.Run("skips injection when Rust frontend is enabled", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "v1",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "true"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Env: []corev1.EnvVar{
						{Name: "VLLM_USE_RUST_FRONTEND", Value: "1"},
					},
				},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		assert.Nil(t, findEnvVar(podSpec.Containers[0].Env, servedByMiddlewareClassVar),
			"should not inject middleware class when Rust frontend is active")
		assert.Empty(t, podSpec.Volumes, "should not add volume when Rust frontend is active")

		cond := llmSvc.GetStatus().GetCondition(v1alpha2.RequestAttributionReady)
		require.NotNil(t, cond, "RequestAttributionReady should be set")
		assert.Equal(t, "False", string(cond.Status))
		assert.Equal(t, "UnsupportedRuntime", cond.Reason)
	})

	t.Run("ignores Rust frontend when value is not 1", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "v1",
				Namespace:   testNamespace,
				Annotations: map[string]string{constants.LLMServedByAnnotationKey: "true"},
			},
		}
		podSpec := &corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Env: []corev1.EnvVar{
						{Name: "VLLM_USE_RUST_FRONTEND", Value: "0"},
					},
				},
			},
		}

		err := testReconcilerWithNamespace(t).injectServedByMiddleware(t.Context(), llmSvc, podSpec)
		require.NoError(t, err)

		assert.NotNil(t, findEnvVar(podSpec.Containers[0].Env, servedByMiddlewareClassVar),
			"should inject when Rust frontend is explicitly disabled")
	})
}

func TestEnsureServedByConfigMap(t *testing.T) {
	t.Run("creates ConfigMap when not present", func(t *testing.T) {
		r := testReconcilerWithNamespace(t)

		ensured, err := r.ensureServedByConfigMap(t.Context(), testNamespace)
		require.NoError(t, err)
		assert.True(t, ensured)

		cm := &corev1.ConfigMap{}
		err = r.Get(t.Context(), clientKeyForConfigMap(), cm)
		require.NoError(t, err)
		assert.Equal(t, servedByMiddlewareSource, cm.Data["served_by.py"])
		assert.Equal(t, servedByManagedByValue, cm.Labels["app.kubernetes.io/managed-by"])
		require.NotNil(t, cm.Immutable, "ConfigMap should be immutable")
		assert.True(t, *cm.Immutable)
	})

	t.Run("no-op when content matches", func(t *testing.T) {
		r := testReconcilerWithNamespace(t)

		ensured, err := r.ensureServedByConfigMap(t.Context(), testNamespace)
		require.NoError(t, err)
		assert.True(t, ensured)

		ensured, err = r.ensureServedByConfigMap(t.Context(), testNamespace)
		require.NoError(t, err)
		assert.True(t, ensured)
	})

	t.Run("deletes and recreates when content differs and label is present", func(t *testing.T) {
		r := testReconcilerWithNamespace(t)

		stale := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      servedByConfigMapName,
				Namespace: testNamespace,
				Labels:    map[string]string{"app.kubernetes.io/managed-by": servedByManagedByValue},
			},
			Data: map[string]string{
				"__init__.py":  "",
				"served_by.py": "old content",
			},
		}
		require.NoError(t, r.Create(t.Context(), stale))

		ensured, err := r.ensureServedByConfigMap(t.Context(), testNamespace)
		require.NoError(t, err)
		assert.True(t, ensured)

		cm := &corev1.ConfigMap{}
		require.NoError(t, r.Get(t.Context(), clientKeyForConfigMap(), cm))
		assert.Equal(t, servedByMiddlewareSource, cm.Data["served_by.py"])
		assert.NotNil(t, cm.Immutable, "recreated ConfigMap should be immutable")
		assert.True(t, *cm.Immutable)
	})

	t.Run("skips update when ConfigMap is not controller-managed", func(t *testing.T) {
		r := testReconcilerWithNamespace(t)

		tenant := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      servedByConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"served_by.py": "tenant data",
			},
		}
		require.NoError(t, r.Create(t.Context(), tenant))

		ensured, err := r.ensureServedByConfigMap(t.Context(), testNamespace)
		require.NoError(t, err)
		assert.False(t, ensured, "should return false to skip injection for unmanaged ConfigMap")

		cm := &corev1.ConfigMap{}
		require.NoError(t, r.Get(t.Context(), clientKeyForConfigMap(), cm))
		assert.Equal(t, "tenant data", cm.Data["served_by.py"], "tenant data should be preserved")
	})

	t.Run("creates ConfigMap in a different namespace", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		require.NoError(t, v1alpha2.AddToScheme(scheme))
		r := &LLMISVCReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		ensured, err := r.ensureServedByConfigMap(t.Context(), "other-ns")
		require.NoError(t, err)
		assert.True(t, ensured)
	})
}

func clientKeyForConfigMap() types.NamespacedName {
	return types.NamespacedName{Name: servedByConfigMapName, Namespace: testNamespace}
}
