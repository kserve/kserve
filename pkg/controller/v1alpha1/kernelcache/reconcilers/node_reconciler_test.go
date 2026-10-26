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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func kernelCacheConfigMap(enabled bool) *corev1.ConfigMap {
	value := `{"enabled":false}`
	if enabled {
		value = `{"enabled":true}`
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Data: map[string]string{"kernelcache": value},
	}
}

func newKernelCacheClient(scheme *runtime.Scheme, enabled bool, objects ...client.Object) client.Client {
	objects = append(objects, kernelCacheConfigMap(enabled))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestKernelCacheNodeReconcilerSkipsWhenDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node", Labels: map[string]string{"role": "gpu"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	}
	existingNode := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: "existing-node"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	k8sClient := newKernelCacheClient(scheme, false, group, node, existingNode, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: group.Name}})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	require.Error(t, k8sClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, &v1alpha1.KernelCacheNode{}))
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(existingNode), &v1alpha1.KernelCacheNode{}))
	updatedDaemonSet := &appsv1.DaemonSet{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(daemonSet), updatedDaemonSet))
	require.Equal(t, daemonSet.Spec, updatedDaemonSet.Spec)
}

// Delete KCN objects whose nodes no longer match any node group.
func TestKernelCacheNodeReconcilerRemovesOrphanNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node", Labels: map[string]string{"role": "gpu"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	}
	orphan := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	k8sClient := newKernelCacheClient(scheme, true, group, node, orphan, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(group)})
	require.NoError(t, err)

	require.Error(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(orphan), &v1alpha1.KernelCacheNode{}))
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, &v1alpha1.KernelCacheNode{}))
}

// Keep KCN objects when their nodes temporarily become not ready.
func TestKernelCacheNodeReconcilerKeepsNotReadyMatchingNode(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node", Labels: map[string]string{"role": "gpu"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}},
	}
	kernelCacheNode := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: node.Name}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	k8sClient := newKernelCacheClient(scheme, true, group, node, kernelCacheNode, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(group)})
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(kernelCacheNode), &v1alpha1.KernelCacheNode{}))
}

func TestKernelCacheNodeReconcilerCreatesNotReadyMatchingNode(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node", Labels: map[string]string{"role": "gpu"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}},
	}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	k8sClient := newKernelCacheClient(scheme, true, group, node, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(group)})
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, &v1alpha1.KernelCacheNode{}))
}

func TestKernelCacheNodeReconcilerDoesNotUpdateDaemonSetForInvalidNodeGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{ObjectMeta: metav1.ObjectMeta{Name: "invalid"}}
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"role": "worker"},
		}}},
	}
	k8sClient := newKernelCacheClient(scheme, true, group, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(group)})
	require.EqualError(t, err, `KernelCacheNodeGroup "invalid" requires a non-empty nodeSelector`)

	updated := &appsv1.DaemonSet{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(daemonSet), updated))
	require.Equal(t, daemonSet.Spec.Template.Spec.NodeSelector, updated.Spec.Template.Spec.NodeSelector)
	require.Equal(t, daemonSet.Spec.Template.Spec.Affinity, updated.Spec.Template.Spec.Affinity)
	require.Equal(t, daemonSet.Spec.Template.Spec.Tolerations, updated.Spec.Template.Spec.Tolerations)
}

func TestKernelCacheNodeReconcilerEnqueuesSingleGlobalRequestForNodeEvent(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{}

	requests := reconciler.nodeToNodeGroupRequests(context.Background(), &corev1.Node{})

	require.Equal(t, []reconcile.Request{{
		NamespacedName: client.ObjectKey{Name: kernelCacheNodeReconcileRequestName},
	}}, requests)
}

func TestKernelCacheNodeReconcilerConfigMapPredicate(t *testing.T) {
	reconciler := &KernelCacheNodeReconciler{}
	predicate := reconciler.inferenceServiceConfigMapPredicate()

	matching := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      constants.InferenceServiceConfigMapName,
		Namespace: constants.KServeNamespace,
	}}
	if !predicate.Create(event.CreateEvent{Object: matching}) {
		t.Fatal("expected the InferenceService ConfigMap to enqueue the controller")
	}
	if predicate.Create(event.CreateEvent{Object: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      constants.InferenceServiceConfigMapName,
		Namespace: "other",
	}}}) {
		t.Fatal("did not expect a ConfigMap from another namespace to enqueue the controller")
	}
	if predicate.Create(event.CreateEvent{Object: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      "other",
		Namespace: constants.KServeNamespace,
	}}}) {
		t.Fatal("did not expect another ConfigMap to enqueue the controller")
	}
}

// Delete KCN objects when their node group has been removed.
func TestKernelCacheNodeReconcilerRemovesNodesAfterGroupDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	orphan := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	k8sClient := newKernelCacheClient(scheme, true, orphan, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "deleted-group"}})
	require.NoError(t, err)
	require.Error(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(orphan), &v1alpha1.KernelCacheNode{}))
}

// Keep all KCN objects when the node list cannot be read safely.
func TestKernelCacheNodeReconcilerDoesNotDeleteOnNodeListFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	orphan := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	baseClient := newKernelCacheClient(scheme, true, group, orphan, daemonSet)
	k8sClient := &failingListClient{Client: baseClient, fail: func(list client.ObjectList) bool {
		_, ok := list.(*corev1.NodeList)
		return ok
	}}
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(group)})
	require.EqualError(t, err, "node list failed")
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(orphan), &v1alpha1.KernelCacheNode{}))
}

// Keep all KCN objects when the node group list cannot be read safely.
func TestKernelCacheNodeReconcilerDoesNotDeleteOnNodeGroupListFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	orphan := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	baseClient := newKernelCacheClient(scheme, true, orphan, daemonSet)
	k8sClient := &failingListClient{Client: baseClient, fail: func(list client.ObjectList) bool {
		_, ok := list.(*v1alpha1.KernelCacheNodeGroupList)
		return ok
	}}
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "deleted-group"}})
	require.EqualError(t, err, "kernel cache node group list failed")
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(orphan), &v1alpha1.KernelCacheNode{}))
}

// Keep all KCN objects when the KCN list cannot be read safely.
func TestKernelCacheNodeReconcilerDoesNotDeleteOnKernelCacheNodeListFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node", Labels: map[string]string{"role": "gpu"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	}
	orphan := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	baseClient := newKernelCacheClient(scheme, true, group, node, orphan, daemonSet)
	k8sClient := &failingListClient{Client: baseClient, fail: func(list client.ObjectList) bool {
		_, ok := list.(*v1alpha1.KernelCacheNodeList)
		return ok
	}}
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(group)})
	require.EqualError(t, err, "kernel cache node list failed")
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(orphan), &v1alpha1.KernelCacheNode{}))
}

type failingListClient struct {
	client.Client
	fail func(client.ObjectList) bool
}

func (c *failingListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.fail(list) {
		switch list.(type) {
		case *corev1.NodeList:
			return errors.New("node list failed")
		case *v1alpha1.KernelCacheNodeGroupList:
			return errors.New("kernel cache node group list failed")
		default:
			return errors.New("kernel cache node list failed")
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func TestKernelCacheNodeReconcilerConfiguresAgentDaemonSetFromNodeGroups(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	h100 := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "h100"},
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"nvidia.com/gpu.product": "NVIDIA-H100"},
			Tolerations:  []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	l40s := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "l40s"},
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"nvidia.com/gpu.product": "NVIDIA-L40S"},
			Tolerations:  []corev1.Toleration{{Key: "workload", Operator: corev1.TolerationOpEqual, Value: "gpu", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"kserve/localmodel": "worker"},
		}}},
	}

	k8sClient := newKernelCacheClient(scheme, true, h100, l40s, daemonSet)
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: h100.Name}})
	require.NoError(t, err)

	updated := &appsv1.DaemonSet{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(daemonSet), updated))
	require.Nil(t, updated.Spec.Template.Spec.NodeSelector)
	terms := updated.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	require.ElementsMatch(t, []corev1.NodeSelectorTerm{
		{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "nvidia.com/gpu.product", Operator: corev1.NodeSelectorOpIn, Values: []string{"NVIDIA-H100"}}}},
		{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "nvidia.com/gpu.product", Operator: corev1.NodeSelectorOpIn, Values: []string{"NVIDIA-L40S"}}}},
	}, terms)
	require.ElementsMatch(t, append(h100.Spec.Tolerations, l40s.Spec.Tolerations...), updated.Spec.Template.Spec.Tolerations)

	require.NoError(t, k8sClient.Delete(context.Background(), l40s))
	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: l40s.Name}})
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(daemonSet), updated))
	require.Equal(t, []corev1.NodeSelectorTerm{{
		MatchExpressions: []corev1.NodeSelectorRequirement{{
			Key: "nvidia.com/gpu.product", Operator: corev1.NodeSelectorOpIn, Values: []string{"NVIDIA-H100"},
		}},
	}}, updated.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)
	require.Equal(t, h100.Spec.Tolerations, updated.Spec.Template.Spec.Tolerations)
}

func TestAgentDaemonSetDoesNotScheduleWithoutNodeGroups(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(daemonSet).Build()
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient}

	require.NoError(t, reconciler.reconcileAgentDaemonSet(context.Background(), nil))
	updated := &appsv1.DaemonSet{}
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(daemonSet), updated))
	terms := updated.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	require.Equal(t, []corev1.NodeSelectorTerm{{
		MatchExpressions: []corev1.NodeSelectorRequirement{{
			Key: kernelCacheNodeAgentDisabledKey, Operator: corev1.NodeSelectorOpIn, Values: []string{kernelCacheNodeAgentDisabledValue},
		}},
	}}, terms)
}
