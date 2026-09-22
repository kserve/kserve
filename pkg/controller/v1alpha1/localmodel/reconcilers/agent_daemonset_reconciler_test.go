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

package reconcilers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

func gpuToleration() corev1.Toleration {
	return corev1.Toleration{
		Key:      "nvidia.com/gpu",
		Operator: corev1.TolerationOpEqual,
		Value:    "present",
		Effect:   corev1.TaintEffectNoSchedule,
	}
}

func inferenceToleration() corev1.Toleration {
	return corev1.Toleration{
		Key:      "dedicated",
		Operator: corev1.TolerationOpEqual,
		Value:    "inference",
		Effect:   corev1.TaintEffectNoSchedule,
	}
}

func makeNodeGroup(name string, tolerations []corev1.Toleration) *v1alpha1.LocalModelNodeGroup {
	return &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1alpha1.LocalModelNodeGroupSpec{Tolerations: tolerations},
	}
}

func makeAgentDaemonSet(tolerations []corev1.Toleration, annotations map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        AgentDaemonSetName,
			Namespace:   constants.KServeNamespace,
			Annotations: annotations,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Tolerations: tolerations},
			},
		},
	}
}

func newAgentDaemonSetReconciler(t *testing.T, objects ...client.Object) *AgentDaemonSetReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, kservescheme.AddControllerAPIs(scheme))
	return &AgentDaemonSetReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		Log:    ctrl.Log.WithName("AgentDaemonSetReconcilerTest"),
		Scheme: scheme,
	}
}

func reconcileAgentDaemonSet(t *testing.T, c *AgentDaemonSetReconciler, nodeGroupName string) *appsv1.DaemonSet {
	t.Helper()
	_, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: nodeGroupName},
	})
	require.NoError(t, err)

	daemonSet := &appsv1.DaemonSet{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{
		Name:      AgentDaemonSetName,
		Namespace: constants.KServeNamespace,
	}, daemonSet))
	return daemonSet
}

func TestAgentDaemonSetCopiesNodeGroupTolerations(t *testing.T) {
	c := newAgentDaemonSetReconciler(t,
		makeAgentDaemonSet(nil, nil),
		makeNodeGroup("gpu", []corev1.Toleration{gpuToleration()}),
	)

	daemonSet := reconcileAgentDaemonSet(t, c, "gpu")

	assert.Equal(t, []corev1.Toleration{gpuToleration()}, daemonSet.Spec.Template.Spec.Tolerations)

	var managed []corev1.Toleration
	require.NoError(t, json.Unmarshal([]byte(daemonSet.Annotations[ManagedTolerationsAnnotation]), &managed))
	assert.Equal(t, []corev1.Toleration{gpuToleration()}, managed)
}

func TestAgentDaemonSetUnionsTolerationsAcrossNodeGroups(t *testing.T) {
	c := newAgentDaemonSetReconciler(t,
		makeAgentDaemonSet(nil, nil),
		// Both node groups tolerate the GPU taint; it must only be applied once.
		makeNodeGroup("gpu-a", []corev1.Toleration{gpuToleration()}),
		makeNodeGroup("gpu-b", []corev1.Toleration{gpuToleration(), inferenceToleration()}),
		makeNodeGroup("cpu", nil),
	)

	daemonSet := reconcileAgentDaemonSet(t, c, "gpu-b")

	assert.Equal(t, []corev1.Toleration{gpuToleration(), inferenceToleration()}, daemonSet.Spec.Template.Spec.Tolerations)
}

func TestAgentDaemonSetKeepsInstallTimeTolerations(t *testing.T) {
	installed := corev1.Toleration{
		Key:      "node-role.kubernetes.io/control-plane",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}
	c := newAgentDaemonSetReconciler(t,
		makeAgentDaemonSet([]corev1.Toleration{installed}, nil),
		makeNodeGroup("gpu", []corev1.Toleration{gpuToleration()}),
	)

	daemonSet := reconcileAgentDaemonSet(t, c, "gpu")

	assert.Equal(t, []corev1.Toleration{installed, gpuToleration()}, daemonSet.Spec.Template.Spec.Tolerations)
}

func TestAgentDaemonSetDropsOnlyItsOwnTolerations(t *testing.T) {
	installed := corev1.Toleration{
		Key:      "node-role.kubernetes.io/control-plane",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}
	managed, err := json.Marshal([]corev1.Toleration{gpuToleration()})
	require.NoError(t, err)

	// The node group no longer declares the GPU toleration, so only that one should go away.
	c := newAgentDaemonSetReconciler(t,
		makeAgentDaemonSet(
			[]corev1.Toleration{installed, gpuToleration()},
			map[string]string{ManagedTolerationsAnnotation: string(managed)},
		),
		makeNodeGroup("gpu", nil),
	)

	daemonSet := reconcileAgentDaemonSet(t, c, "gpu")

	assert.Equal(t, []corev1.Toleration{installed}, daemonSet.Spec.Template.Spec.Tolerations)
	assert.NotContains(t, daemonSet.Annotations, ManagedTolerationsAnnotation)
}

func TestAgentDaemonSetKeepsTolerationsWhenAnnotationIsMalformed(t *testing.T) {
	c := newAgentDaemonSetReconciler(t,
		makeAgentDaemonSet(
			[]corev1.Toleration{inferenceToleration()},
			map[string]string{ManagedTolerationsAnnotation: "not-json"},
		),
		makeNodeGroup("gpu", []corev1.Toleration{gpuToleration()}),
	)

	daemonSet := reconcileAgentDaemonSet(t, c, "gpu")

	assert.Equal(t, []corev1.Toleration{inferenceToleration(), gpuToleration()}, daemonSet.Spec.Template.Spec.Tolerations)
}

func TestAgentDaemonSetReconcileIsIdempotent(t *testing.T) {
	c := newAgentDaemonSetReconciler(t,
		makeAgentDaemonSet(nil, nil),
		makeNodeGroup("gpu", []corev1.Toleration{gpuToleration()}),
	)

	first := reconcileAgentDaemonSet(t, c, "gpu")
	second := reconcileAgentDaemonSet(t, c, "gpu")

	assert.Equal(t, first.Spec.Template.Spec.Tolerations, second.Spec.Template.Spec.Tolerations)
	assert.Equal(t, first.ResourceVersion, second.ResourceVersion, "a no-op reconcile should not patch the DaemonSet")
}

func TestAgentDaemonSetMissingIsNotAnError(t *testing.T) {
	c := newAgentDaemonSetReconciler(t, makeNodeGroup("gpu", []corev1.Toleration{gpuToleration()}))

	result, err := c.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "gpu"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
