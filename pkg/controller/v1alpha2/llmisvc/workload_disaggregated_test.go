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
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

const (
	disaggTestNamespace  = "disagg"
	disaggTestPool       = "disagg-pool"
	disaggTestUserSA     = "user-sa"
	disaggConfiguredInit = "kserve/storage-initializer:configured"
	disaggDeployedInit   = "kserve/storage-initializer:deployed"
)

func disaggTestService(t *testing.T) *v1alpha2.LLMInferenceService {
	t.Helper()
	modelURL, err := apis.ParseURL("hf://org/model")
	require.NoError(t, err)
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llama",
			Namespace: disaggTestNamespace,
			UID:       "llama-uid",
			Labels: map[string]string{
				constants.KueueAPIGroupName + "/queue-name": "queue",
				"unapproved-label":                          "dropped",
			},
			Annotations: map[string]string{
				"prometheus.io/scrape":                           "true",
				"k8s.v1.cni.cncf.io/networks":                    "net",
				"leaderworkerset.sigs.k8s.io/exclusive-topology": "rack",
				"unapproved-annotation":                          "dropped",
			},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *modelURL},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Labels:   map[string]string{"decode-label": "d"},
				Annotations: map[string]string{
					"decode-annotation":                        "d",
					AnnotationModelBasedRoutingEnabled:         "true",
					constants.LLMDisaggregatedSetAnnotationKey: "true",
				},
			},
			Prefill: &v1alpha2.WorkloadSpec{
				Template:    disaggTestPod(),
				Labels:      map[string]string{"prefill-label": "p"},
				Annotations: map[string]string{"prefill-annotation": "p"},
			},
		},
	}
}

func disaggTestKEDAScaling(minReplicas, maxReplicas int32) *v1alpha2.ScalingSpec {
	return &v1alpha2.ScalingSpec{
		MinReplicas: ptr.To(minReplicas),
		MaxReplicas: maxReplicas,
		KEDA: &v1alpha2.DirectKEDAScalingSpec{
			Triggers: []kedav1alpha1.ScaleTriggers{{Type: "cpu", Metadata: map[string]string{"value": "80"}}},
		},
	}
}

// disaggTestWVAScaling returns scaling that only configures WVA, which is no longer
// supported: the controller creates no autoscaler for it, so the role is not autoscaled.
func disaggTestWVAScaling(minReplicas, maxReplicas int32) *v1alpha2.ScalingSpec {
	return &v1alpha2.ScalingSpec{
		MinReplicas: ptr.To(minReplicas),
		MaxReplicas: maxReplicas,
		WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}},
	}
}

func disaggTestPod() *corev1.PodSpec {
	return &corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "vllm:latest"}}}
}

func disaggTestPodWithSidecar() *corev1.PodSpec {
	pod := disaggTestPod()
	pod.InitContainers = []corev1.Container{{Name: constants.LLMISVCRoutingSidecarContainerName, Image: "sidecar:latest"}}
	return pod
}

func disaggTestParallelism() *v1alpha2.ParallelismSpec {
	return &v1alpha2.ParallelismSpec{Tensor: ptr.To[int32](2), Pipeline: ptr.To[int32](2)}
}

func disaggTestConfig() *Config {
	return &Config{
		StorageConfig: &kserveTypes.StorageInitializerConfig{
			Image:         disaggConfiguredInit,
			CpuRequest:    "100m",
			CpuLimit:      "1",
			MemoryRequest: "256Mi",
			MemoryLimit:   "1Gi",
		},
		CredentialConfig: &credentials.CredentialConfig{},
		FeatureGates:     FeatureGates{DisaggregatedSet: true},
	}
}

func disaggTestReconciler(t *testing.T) *LLMISVCReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	require.NoError(t, lwsapi.AddToScheme(scheme))
	require.NoError(t, disaggregatedsetv1.AddToScheme(scheme))
	require.NoError(t, igwapi.Install(scheme))

	objs := []client.Object{
		&igwapi.InferencePool{
			ObjectMeta: metav1.ObjectMeta{Name: disaggTestPool, Namespace: disaggTestNamespace},
			Spec: igwapi.InferencePoolSpec{
				Selector: igwapi.LabelSelector{MatchLabels: map[igwapi.LabelKey]igwapi.LabelValue{"pool": "llama"}},
			},
		},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: disaggTestUserSA, Namespace: disaggTestNamespace}},
	}
	return &LLMISVCReconciler{
		Client:                              fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Clientset:                           k8sfake.NewClientset(),
		DisaggregatedSetAvailable:           true,
		DisaggregatedSetRoleScalerAvailable: true,
	}
}

func TestDecideDisaggregatedSet(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(svc *v1alpha2.LLMInferenceService, config *Config, r *LLMISVCReconciler)
		wantUse      bool
		wantReason   string
		wantNoReason bool
	}{
		{
			name:    "requested and supported",
			mutate:  func(*v1alpha2.LLMInferenceService, *Config, *LLMISVCReconciler) {},
			wantUse: true,
		},
		{
			name: "not requested",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				delete(svc.Spec.Annotations, constants.LLMDisaggregatedSetAnnotationKey)
			},
			wantNoReason: true,
		},
		{
			name: "explicitly disabled",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Annotations[constants.LLMDisaggregatedSetAnnotationKey] = "false"
			},
			wantNoReason: true,
		},
		{
			name: "feature gate off",
			mutate: func(_ *v1alpha2.LLMInferenceService, config *Config, _ *LLMISVCReconciler) {
				config.FeatureGates.DisaggregatedSet = false
			},
			wantReason: reasonFeatureGateDisabled,
		},
		{
			name: "CRD not installed",
			mutate: func(_ *v1alpha2.LLMInferenceService, _ *Config, r *LLMISVCReconciler) {
				r.DisaggregatedSetAvailable = false
			},
			wantReason: reasonCRDNotInstalled,
		},
		{
			name:       "no prefill",
			mutate:     func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) { svc.Spec.Prefill = nil },
			wantReason: reasonNoPrefillWorkload,
		},
		{
			name: "decode autoscaled without the DisaggregatedSetRoleScaler CRD",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, r *LLMISVCReconciler) {
				svc.Spec.Scaling = disaggTestKEDAScaling(1, 3)
				r.DisaggregatedSetRoleScalerAvailable = false
			},
			wantReason: reasonRoleScalerNotInstalled,
		},
		{
			name: "prefill autoscaled without the DisaggregatedSetRoleScaler CRD",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, r *LLMISVCReconciler) {
				svc.Spec.Prefill.Scaling = disaggTestKEDAScaling(1, 3)
				r.DisaggregatedSetRoleScalerAvailable = false
			},
			wantReason: reasonRoleScalerNotInstalled,
		},
		{
			// WVA is no longer supported, so a role that only configures it is static.
			name: "decode with only WVA scaling without the DisaggregatedSetRoleScaler CRD",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, r *LLMISVCReconciler) {
				svc.Spec.Scaling = disaggTestWVAScaling(1, 3)
				r.DisaggregatedSetRoleScalerAvailable = false
			},
			wantUse: true,
		},
		{
			name: "static roles without the DisaggregatedSetRoleScaler CRD",
			mutate: func(_ *v1alpha2.LLMInferenceService, _ *Config, r *LLMISVCReconciler) {
				r.DisaggregatedSetRoleScalerAvailable = false
			},
			wantUse: true,
		},
		{
			name: "decode autoscaled by KEDA",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Scaling = disaggTestKEDAScaling(1, 3)
			},
			wantUse: true,
		},
		{
			name: "prefill autoscaled by KEDA",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Prefill.Scaling = disaggTestKEDAScaling(1, 3)
			},
			wantUse: true,
		},
		{
			name: "both autoscaled by KEDA",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Scaling = disaggTestKEDAScaling(1, 3)
				svc.Spec.Prefill.Scaling = disaggTestKEDAScaling(1, 3)
			},
			wantUse: true,
		},
		{
			// A DisaggregatedSet checks zero and non-zero replicas only across static roles.
			name: "static prefill scaled to zero next to an autoscaled decode",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Scaling = disaggTestKEDAScaling(1, 3)
				svc.Spec.Prefill.Replicas = ptr.To[int32](0)
			},
			wantUse: true,
		},
		{
			name: "prefill scaled to zero next to a decode with only WVA scaling",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Scaling = disaggTestWVAScaling(1, 3)
				svc.Spec.Prefill.Replicas = ptr.To[int32](0)
			},
			wantReason: reasonReplicasMismatch,
		},
		{
			name: "only prefill scaled to zero",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Prefill.Replicas = ptr.To[int32](0)
			},
			wantReason: reasonReplicasMismatch,
		},
		{
			name: "both scaled to zero",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Replicas = ptr.To[int32](0)
				svc.Spec.Prefill.Replicas = ptr.To[int32](0)
			},
			wantUse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, config, r := disaggTestService(t), disaggTestConfig(), &LLMISVCReconciler{DisaggregatedSetAvailable: true, DisaggregatedSetRoleScalerAvailable: true}
			tt.mutate(svc, config, r)

			got := r.decideDisaggregatedSet(svc, config)
			assert.Equal(t, tt.wantUse, got.Use)
			assert.Equal(t, !tt.wantNoReason, got.Requested)
			assert.Equal(t, tt.wantReason, got.Reason)
			if tt.wantReason != "" {
				assert.NotEmpty(t, got.Message)
			}
		})
	}
}

// TestMarkDisaggregatedSetDecision covers the condition and the warnings a decision
// produces. They depend only on the decision and on the workloads that exist, never on
// the service's previous status, which can be stale or lost: several cases start from a
// misleading status to check it is ignored.
func TestMarkDisaggregatedSetDecision(t *testing.T) {
	use := disaggregatedSetDecision{Requested: true, Use: true}
	fallback := disaggregatedSetDecision{Requested: true, Reason: reasonReplicasMismatch, Message: "replicas mismatch"}

	deployments := func(svc *v1alpha2.LLMInferenceService) []client.Object {
		return []client.Object{
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: mainDeploymentName(svc), Namespace: disaggTestNamespace}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: prefillDeploymentName(svc), Namespace: disaggTestNamespace}},
		}
	}
	leaderWorkerSets := func(svc *v1alpha2.LLMInferenceService) []client.Object {
		return []client.Object{
			&lwsapi.LeaderWorkerSet{ObjectMeta: metav1.ObjectMeta{Name: mainLWSName(svc), Namespace: disaggTestNamespace}},
			&lwsapi.LeaderWorkerSet{ObjectMeta: metav1.ObjectMeta{Name: prefillLWSName(svc), Namespace: disaggTestNamespace}},
		}
	}
	disaggregatedSet := func(svc *v1alpha2.LLMInferenceService) []client.Object {
		return []client.Object{
			&disaggregatedsetv1.DisaggregatedSet{ObjectMeta: metav1.ObjectMeta{Name: disaggregatedSetName(svc), Namespace: disaggTestNamespace}},
		}
	}
	staleUsed := func(svc *v1alpha2.LLMInferenceService) { svc.MarkDisaggregatedSetUsed() }
	staleNotUsed := func(svc *v1alpha2.LLMInferenceService) {
		svc.MarkDisaggregatedSetNotUsed(fallback.Reason, "%s", fallback.Message)
	}

	tests := []struct {
		name          string
		status        func(svc *v1alpha2.LLMInferenceService)
		existing      func(svc *v1alpha2.LLMInferenceService) []client.Object
		stopped       bool
		decision      disaggregatedSetDecision
		wantStatus    corev1.ConditionStatus
		wantReason    string
		wantEvent     string
		wantInMessage string
	}{
		{
			name:       "a new service starts on the DisaggregatedSet without a warning",
			decision:   use,
			wantStatus: corev1.ConditionTrue,
		},
		{
			name:          "a service on Deployments moves onto the DisaggregatedSet",
			existing:      deployments,
			decision:      use,
			wantStatus:    corev1.ConditionTrue,
			wantEvent:     disaggregatedSetMigratingToReason,
			wantInMessage: "unavailable",
		},
		{
			name:       "a service on LeaderWorkerSets moves onto the DisaggregatedSet",
			existing:   leaderWorkerSets,
			decision:   use,
			wantStatus: corev1.ConditionTrue,
			wantEvent:  disaggregatedSetMigratingToReason,
		},
		{
			name:       "a service on Deployments moves even if its status says it already uses the DisaggregatedSet",
			status:     staleUsed,
			existing:   deployments,
			decision:   use,
			wantStatus: corev1.ConditionTrue,
			wantEvent:  disaggregatedSetMigratingToReason,
		},
		{
			name:       "a service already on the DisaggregatedSet stays quiet",
			existing:   disaggregatedSet,
			decision:   use,
			wantStatus: corev1.ConditionTrue,
		},
		{
			name:          "opting out moves the service off the DisaggregatedSet",
			existing:      disaggregatedSet,
			decision:      disaggregatedSetDecision{},
			wantEvent:     disaggregatedSetMigratingFromReason,
			wantInMessage: constants.LLMDisaggregatedSetAnnotationKey,
		},
		{
			name:     "opting out without a DisaggregatedSet is quiet even if the status says it was used",
			status:   staleUsed,
			decision: disaggregatedSetDecision{},
		},
		{
			// Leaving the backend is reported once, as a migration that carries the reason.
			name:          "falling back moves the service off the DisaggregatedSet and says why",
			existing:      disaggregatedSet,
			decision:      fallback,
			wantStatus:    corev1.ConditionFalse,
			wantReason:    reasonReplicasMismatch,
			wantEvent:     disaggregatedSetMigratingFromReason,
			wantInMessage: "replicas mismatch",
		},
		{
			// The condition says why; only a migration, which replaces running
			// workloads, warns.
			name:       "falling back without a DisaggregatedSet only records why",
			decision:   fallback,
			wantStatus: corev1.ConditionFalse,
			wantReason: reasonReplicasMismatch,
		},
		{
			name:       "falling back again only records why",
			status:     staleNotUsed,
			decision:   fallback,
			wantStatus: corev1.ConditionFalse,
			wantReason: reasonReplicasMismatch,
		},
		{
			name:       "stopping a service on Deployments is not a migration",
			existing:   deployments,
			stopped:    true,
			decision:   use,
			wantStatus: corev1.ConditionTrue,
		},
		{
			name:     "stopping a service on a DisaggregatedSet is not a migration",
			existing: disaggregatedSet,
			stopped:  true,
			decision: disaggregatedSetDecision{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := disaggTestService(t)
			if tt.status != nil {
				tt.status(svc)
			}
			if tt.stopped {
				svc.Annotations[constants.StopAnnotationKey] = "true"
			}
			r := disaggTestReconciler(t)
			if tt.existing != nil {
				for _, obj := range tt.existing(svc) {
					require.NoError(t, r.Create(context.Background(), obj))
				}
			}
			recorder := record.NewFakeRecorder(10)
			r.EventRecorder = recorder

			r.markDisaggregatedSetDecision(context.Background(), svc, tt.decision)

			cond := svc.Status.GetCondition(v1alpha2.DisaggregatedSetUsed)
			if tt.wantStatus == "" {
				assert.Nil(t, cond)
			} else if assert.NotNil(t, cond) {
				assert.Equal(t, tt.wantStatus, cond.Status)
				assert.Equal(t, tt.wantReason, cond.Reason)
			}

			if tt.wantEvent == "" {
				assert.Empty(t, recorder.Events)
				return
			}
			require.Len(t, recorder.Events, 1)
			event := <-recorder.Events
			assert.True(t, strings.HasPrefix(event, corev1.EventTypeWarning+" "+tt.wantEvent+" "), event)
			assert.Contains(t, event, tt.wantInMessage)
		})
	}
}

// TestReconcileDisaggregatedSetDeletesWhenNotUsed checks that reconcileDisaggregatedSet
// owns both directions, like the other workload reconcilers: a service that does not
// use the backend has its DisaggregatedSet deleted, and one without a DisaggregatedSet
// reconciles without error.
func TestReconcileDisaggregatedSetDeletesWhenNotUsed(t *testing.T) {
	ctx := context.Background()
	svc := disaggTestService(t)
	r := disaggTestReconciler(t)
	r.EventRecorder = record.NewFakeRecorder(10)
	require.NoError(t, r.Create(ctx, &disaggregatedsetv1.DisaggregatedSet{ObjectMeta: metav1.ObjectMeta{
		Name:            disaggregatedSetName(svc),
		Namespace:       svc.Namespace,
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(svc, v1alpha2.LLMInferenceServiceGVK)},
	}}))

	ds, err := r.reconcileDisaggregatedSet(ctx, svc, disaggTestConfig(), false)
	require.NoError(t, err)
	assert.Nil(t, ds)
	err = r.Get(ctx, client.ObjectKey{Name: disaggregatedSetName(svc), Namespace: svc.Namespace}, &disaggregatedsetv1.DisaggregatedSet{})
	assert.True(t, apierrors.IsNotFound(err), "the DisaggregatedSet should be deleted, got %v", err)

	ds, err = r.reconcileDisaggregatedSet(ctx, svc, disaggTestConfig(), false)
	require.NoError(t, err, "nothing to delete is not an error")
	assert.Nil(t, ds)
}

// TestReconcileDisaggregatedSetKeepsControllerAnnotations checks that updating the
// DisaggregatedSet keeps the annotations its controller sets on it.
func TestReconcileDisaggregatedSetKeepsControllerAnnotations(t *testing.T) {
	ctx := context.Background()
	svc := disaggTestService(t)
	r := disaggTestReconciler(t)
	r.EventRecorder = record.NewFakeRecorder(10)
	// Update resolves the object's scope through the RESTMapper, which by default does
	// not know the DisaggregatedSet kind.
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{disaggregatedsetv1.GroupVersion})
	mapper.Add(disaggregatedsetv1.GroupVersion.WithKind("DisaggregatedSet"), meta.RESTScopeNamespace)
	r.Client = fake.NewClientBuilder().WithScheme(r.Scheme()).WithRESTMapper(mapper).Build()

	_, err := r.reconcileDisaggregatedSet(ctx, svc, disaggTestConfig(), true)
	require.NoError(t, err)

	key := client.ObjectKey{Name: disaggregatedSetName(svc), Namespace: svc.Namespace}
	stored := &disaggregatedsetv1.DisaggregatedSet{}
	require.NoError(t, r.Get(ctx, key, stored))
	stored.Annotations = map[string]string{
		disaggregatedsetv1.RevisionHashVersionAnnotationKey: disaggregatedsetv1.RevisionHashVersion,
		disaggregatedsetv1.MaxUnavailableAnnotationKey:      `{"decode":1,"prefill":1}`,
	}
	require.NoError(t, r.Update(ctx, stored))

	svc.Spec.Replicas = ptr.To[int32](3)
	svc.Spec.Prefill.Replicas = ptr.To[int32](3)
	ds, err := r.reconcileDisaggregatedSet(ctx, svc, disaggTestConfig(), true)
	require.NoError(t, err)

	assert.Equal(t, ptr.To[int32](3), findDisaggregatedRole(ds, constants.LLMDRoleDecode).Spec.Replicas, "the update is applied")
	assert.Equal(t, disaggregatedsetv1.RevisionHashVersion, ds.Annotations[disaggregatedsetv1.RevisionHashVersionAnnotationKey])
	assert.JSONEq(t, `{"decode":1,"prefill":1}`, ds.Annotations[disaggregatedsetv1.MaxUnavailableAnnotationKey])
}

func TestPreserveDisaggregatedSetControllerAnnotations(t *testing.T) {
	opts := &updateOptions[*disaggregatedsetv1.DisaggregatedSet]{}
	PreserveDisaggregatedSetControllerAnnotations()(opts)
	require.Len(t, opts.afterDryRunFns, 1)
	fn := opts.afterDryRunFns[0]

	withAnnotations := func(annotations map[string]string) *disaggregatedsetv1.DisaggregatedSet {
		return &disaggregatedsetv1.DisaggregatedSet{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
	}

	tests := []struct {
		name     string
		expected map[string]string
		curr     map[string]string
		want     map[string]string
	}{
		{
			name: "copies the controller annotations from curr",
			curr: map[string]string{
				disaggregatedsetv1.RevisionHashVersionAnnotationKey: disaggregatedsetv1.RevisionHashVersion,
				disaggregatedsetv1.MaxUnavailableAnnotationKey:      `{"decode":1}`,
			},
			want: map[string]string{
				disaggregatedsetv1.RevisionHashVersionAnnotationKey: disaggregatedsetv1.RevisionHashVersion,
				disaggregatedsetv1.MaxUnavailableAnnotationKey:      `{"decode":1}`,
			},
		},
		{
			name:     "keeps the annotations expected sets",
			expected: map[string]string{"kserve": "value"},
			curr:     map[string]string{disaggregatedsetv1.RevisionHashVersionAnnotationKey: disaggregatedsetv1.RevisionHashVersion},
			want: map[string]string{
				"kserve": "value",
				disaggregatedsetv1.RevisionHashVersionAnnotationKey: disaggregatedsetv1.RevisionHashVersion,
			},
		},
		{
			name: "adds no controller annotation that curr does not have",
			curr: map[string]string{"other": "value"},
			want: nil,
		},
		{
			name: "copies only annotations in the DisaggregatedSet API group",
			curr: map[string]string{
				"example.com/other": "value",
				"leaderworkerset.sigs.k8s.io/exclusive-topology": "rack",
			},
			want: nil,
		},
		{
			name:     "a value expected sets wins over curr",
			expected: map[string]string{disaggregatedsetv1.RevisionHashVersionAnnotationKey: "v3"},
			curr:     map[string]string{disaggregatedsetv1.RevisionHashVersionAnnotationKey: disaggregatedsetv1.RevisionHashVersion},
			want:     map[string]string{disaggregatedsetv1.RevisionHashVersionAnnotationKey: "v3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := withAnnotations(tt.expected)
			fn(expected, expected.DeepCopy(), withAnnotations(tt.curr))
			assert.Equal(t, tt.want, expected.Annotations)
		})
	}
}

// TestDisaggregatedSetAnnotationStaysOffPodTemplates checks that the annotation that
// requests the DisaggregatedSet backend, which the P/D presets set, never reaches a pod
// template. A pod template that gains it changes, so adding it to the presets would roll
// every existing P/D service on upgrade, whichever backend it runs on.
func TestDisaggregatedSetAnnotationStaysOffPodTemplates(t *testing.T) {
	ctx := context.Background()
	key := constants.LLMDisaggregatedSetAnnotationKey
	r := disaggTestReconciler(t)

	singleNode := disaggTestService(t)
	require.Equal(t, "true", singleNode.Spec.Annotations[key], "the service under test requests the backend")

	decode, err := r.expectedSingleNodeMainDeployment(ctx, singleNode.DeepCopy(), disaggTestConfig())
	require.NoError(t, err)
	assert.NotContains(t, decode.Spec.Template.Annotations, key, "decode Deployment")

	multiNode := disaggTestService(t)
	multiNode.Spec.Worker = disaggTestPod()
	multiNode.Spec.Parallelism = disaggTestParallelism()
	lws, err := r.expectedMainMultiNodeLWS(ctx, multiNode.DeepCopy(), disaggTestConfig())
	require.NoError(t, err)
	assert.NotContains(t, lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations, key, "decode LeaderWorkerSet worker")
	if lws.Spec.LeaderWorkerTemplate.LeaderTemplate != nil {
		assert.NotContains(t, lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations, key, "decode LeaderWorkerSet leader")
	}

	for _, svc := range []*v1alpha2.LLMInferenceService{singleNode, multiNode} {
		ds, err := r.expectedDisaggregatedSet(ctx, svc.DeepCopy(), disaggTestConfig(), nil)
		require.NoError(t, err)
		for _, role := range ds.Spec.Roles {
			group := role.Spec.LeaderWorkerTemplate
			assert.NotContains(t, group.WorkerTemplate.Annotations, key, "%s role worker", role.Name)
			if group.LeaderTemplate != nil {
				assert.NotContains(t, group.LeaderTemplate.Annotations, key, "%s role leader", role.Name)
			}
		}
	}
}

func TestDisaggregatedSetName(t *testing.T) {
	short := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llama"}}
	assert.Equal(t, "llama-kserve-pd", disaggregatedSetName(short))

	long := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "a-very-long-inference-service-name"}}
	name := disaggregatedSetName(long)
	assert.LessOrEqual(t, len(name), disaggregatedSetNameMaxLength)
	assert.True(t, strings.HasSuffix(name, disaggregatedSetNameSuffix))
	assert.Equal(t, name, disaggregatedSetName(long), "names must be deterministic")

	// The longest name the DisaggregatedSet derives is the StatefulSet label of a worker
	// pod: <ds>-<slice>-<revision:8>-<role>-<group>-<hash:10>. It must stay within 63
	// characters for up to 10,000 groups per role.
	workerLabel := strings.Join([]string{name, "0", "abcdef12", constants.LLMDRolePrefill, "9999", "0123456789"}, "-")
	assert.LessOrEqual(t, len(workerLabel), 63, workerLabel)

	other := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "a-very-long-inference-service-other"}}
	assert.NotEqual(t, name, disaggregatedSetName(other), "distinct services must get distinct names")
}

// TestDisaggregatedRolesMatchWorkloadBuilders keeps the DisaggregatedSet pod renderers
// in sync with the Deployment and LeaderWorkerSet builders they mirror, so a service
// runs the same pods whichever backend it uses.
func TestDisaggregatedRolesMatchWorkloadBuilders(t *testing.T) {
	withRouter := func(svc *v1alpha2.LLMInferenceService) {
		svc.Spec.Router = &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Pool: &v1alpha2.InferencePoolSpec{Ref: &corev1.LocalObjectReference{Name: disaggTestPool}},
			},
		}
	}
	withFeatures := func(svc *v1alpha2.LLMInferenceService) {
		kvCache := &v1alpha2.KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []v1alpha2.SecondaryTierSpec{{
				FileSystem: &v1alpha2.FileSystemTierSpec{EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("20Gi")}},
			}},
		}
		svc.Spec.KVCacheOffloading = kvCache
		svc.Spec.Prefill.KVCacheOffloading = kvCache.DeepCopy()
		svc.Spec.Tracing = &v1alpha2.TracingSpec{ExporterEndpoint: ptr.To("http://collector:4317")}
	}

	tests := []struct {
		name   string
		mutate func(svc *v1alpha2.LLMInferenceService)
		legacy bool
	}{
		{
			name: "single node",
			mutate: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Template = disaggTestPodWithSidecar()
				withRouter(svc)
				withFeatures(svc)
			},
		},
		{
			name: "single node with user service accounts",
			mutate: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Template.ServiceAccountName = disaggTestUserSA
				svc.Spec.Prefill.Template.ServiceAccountName = disaggTestUserSA
			},
		},
		{
			name: "multi node with leaders",
			mutate: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Template = disaggTestPodWithSidecar()
				svc.Spec.Worker = disaggTestPodWithSidecar()
				svc.Spec.Parallelism = disaggTestParallelism()
				svc.Spec.Prefill.Worker = disaggTestPodWithSidecar()
				svc.Spec.Prefill.Parallelism = &v1alpha2.ParallelismSpec{Data: ptr.To[int32](4), DataLocal: ptr.To[int32](2)}
				withRouter(svc)
				withFeatures(svc)
			},
		},
		{
			name: "multi node workers only",
			mutate: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Template = nil
				svc.Spec.Worker = disaggTestPod()
				svc.Spec.Parallelism = disaggTestParallelism()
				svc.Spec.Prefill.Template = nil
				svc.Spec.Prefill.Worker = disaggTestPod()
				svc.Spec.Prefill.Parallelism = disaggTestParallelism()
				withRouter(svc)
			},
		},
		{
			name: "single node decode, multi node prefill",
			mutate: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Template = disaggTestPodWithSidecar()
				svc.Spec.Prefill.Worker = disaggTestPod()
				svc.Spec.Prefill.Parallelism = disaggTestParallelism()
				withRouter(svc)
			},
		},
		{
			name: "multi node decode, single node prefill",
			mutate: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Worker = disaggTestPod()
				svc.Spec.Parallelism = disaggTestParallelism()
				withFeatures(svc)
			},
		},
	}

	legacyTests := slices.Clone(tests)
	for i := range legacyTests {
		legacyTests[i].name += " with legacy presets"
		legacyTests[i].legacy = true
	}
	for _, tt := range slices.Concat(tests, legacyTests) {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc := disaggTestService(t)
			tt.mutate(svc)
			if !tt.legacy {
				for _, pod := range []*corev1.PodSpec{svc.Spec.Template, svc.Spec.Worker, svc.Spec.Prefill.Template, svc.Spec.Prefill.Worker} {
					if sidecar := routingSidecar(pod); sidecar != nil {
						sidecar.Env = append(sidecar.Env, corev1.EnvVar{Name: "KSERVE_TRACING_CONFIG_VERSION", Value: "1"})
					}
				}
			}
			r := disaggTestReconciler(t)

			ds, err := r.expectedDisaggregatedSet(ctx, svc.DeepCopy(), disaggTestConfig(), nil)
			require.NoError(t, err)
			require.Len(t, ds.Spec.Roles, 2)
			decode, prefill := ds.Spec.Roles[0], ds.Spec.Roles[1]
			require.Equal(t, constants.LLMDRoleDecode, decode.Name)
			require.Equal(t, constants.LLMDRolePrefill, prefill.Name)
			for _, role := range ds.Spec.Roles {
				pods := []*corev1.PodSpec{&role.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec}
				if role.Spec.LeaderWorkerTemplate.LeaderTemplate != nil {
					pods = append(pods, &role.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec)
				}
				for _, pod := range pods {
					if sidecar := routingSidecar(pod); sidecar != nil {
						assert.Equal(t, svc.Spec.Tracing != nil && !tt.legacy, slices.Contains(sidecar.Args, "--tracing=true"))
					}
				}
			}

			if svc.Spec.Worker != nil {
				lws, err := r.expectedMainMultiNodeLWS(ctx, svc.DeepCopy(), disaggTestConfig())
				require.NoError(t, err)
				assertRoleMatchesLWS(t, decode, lws)
			} else {
				d, err := r.expectedSingleNodeMainDeployment(ctx, svc.DeepCopy(), disaggTestConfig())
				require.NoError(t, err)
				assertRoleMatchesDeployment(t, decode, d)
			}

			if svc.Spec.Prefill.Worker != nil {
				lws, err := r.expectedPrefillMultiNodeLWS(ctx, svc.DeepCopy(), disaggTestConfig())
				require.NoError(t, err)
				assertRoleMatchesLWS(t, prefill, lws)
			} else {
				d, err := r.expectedPrefillMainDeployment(ctx, svc.DeepCopy(), disaggTestConfig())
				require.NoError(t, err)
				assertRoleMatchesDeployment(t, prefill, d)
			}
		})
	}
}

func assertRoleMatchesDeployment(t *testing.T, role disaggregatedsetv1.DisaggregatedRoleSpec, d *appsv1.Deployment) {
	t.Helper()
	group := role.Spec.LeaderWorkerTemplate
	assert.Nil(t, group.LeaderTemplate)
	assert.Equal(t, ptr.To[int32](1), group.Size)
	// A Deployment restarts a failed container in place; recreating the group would
	// replace the pod and run its init containers, such as the model download, again.
	assert.Equal(t, lwsapi.NoneRestartPolicy, group.RestartPolicy, "restart policy of role %s", role.Name)
	assert.Equal(t, d.Spec.Template, group.WorkerTemplate, "pod template of role %s", role.Name)
	assert.Equal(t, d.Labels, role.Labels, "metadata labels of role %s", role.Name)
	assert.Equal(t, d.Annotations, role.Annotations, "metadata annotations of role %s", role.Name)
}

func assertRoleMatchesLWS(t *testing.T, role disaggregatedsetv1.DisaggregatedRoleSpec, lws *lwsapi.LeaderWorkerSet) {
	t.Helper()
	assert.Equal(t, lws.Spec.LeaderWorkerTemplate, role.Spec.LeaderWorkerTemplate, "group template of role %s", role.Name)
	assert.Equal(t, lws.Labels, role.Labels, "metadata labels of role %s", role.Name)
	assert.Equal(t, lws.Annotations, role.Annotations, "metadata annotations of role %s", role.Name)
}

func TestExpectedDisaggregatedSet(t *testing.T) {
	svc := disaggTestService(t)
	svc.Spec.Replicas = ptr.To[int32](4)
	svc.Spec.RolloutStrategy = &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(2))}
	svc.Spec.Prefill.RolloutStrategy = &v1alpha2.RolloutStrategy{MaxUnavailable: ptr.To(intstr.FromInt32(0))}

	ds, err := disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), nil)
	require.NoError(t, err)

	assert.Equal(t, "llama-kserve-pd", ds.Name)
	assert.Equal(t, disaggTestNamespace, ds.Namespace)
	assert.Equal(t, constants.LLMInferenceServicePartOfValue, ds.Labels[constants.KubernetesPartOfLabelKey],
		"the part-of label is what lets DisaggregatedSet events reach the controller")
	require.Len(t, ds.OwnerReferences, 1)
	assert.Equal(t, svc.UID, ds.OwnerReferences[0].UID)
	assert.True(t, ptr.Deref(ds.OwnerReferences[0].Controller, false))

	decode, prefill := ds.Spec.Roles[0].Spec, ds.Spec.Roles[1].Spec
	assert.Equal(t, ptr.To[int32](4), decode.Replicas)
	assert.Equal(t, ptr.To[int32](1), prefill.Replicas, "unset replicas are written as 1: a DisaggregatedSet rejects a mix of set and unset")
	for _, role := range []lwsapi.LeaderWorkerSetSpec{decode, prefill} {
		assert.Equal(t, lwsapi.RollingUpdateStrategyType, role.RolloutStrategy.Type)
		assert.Equal(t, lwsapi.LeaderCreatedStartupPolicy, role.StartupPolicy)
		assert.Equal(t, lwsapi.NoneRestartPolicy, role.LeaderWorkerTemplate.RestartPolicy, "single-node roles restart containers in place")
	}
	assert.Equal(t, &lwsapi.RollingUpdateConfiguration{
		MaxUnavailable: intstr.FromString("25%"),
		MaxSurge:       intstr.FromInt32(2),
	}, decode.RolloutStrategy.RollingUpdateConfiguration)
	assert.Equal(t, &lwsapi.RollingUpdateConfiguration{
		MaxUnavailable: intstr.FromInt32(0),
		MaxSurge:       intstr.FromString("25%"),
	}, prefill.RolloutStrategy.RollingUpdateConfiguration, "maxUnavailable: 0 must not leave maxSurge at 0, which the DisaggregatedSet rejects")
	assert.Nil(t, ds.Spec.Slices)
	assert.Nil(t, ds.Spec.PlacementPolicy)
}

// TestExpectedDisaggregatedSetResolvesZeroRollout checks that both roles of a
// DisaggregatedSet get a rollout the DisaggregatedSet accepts when the values they set
// round down to zero for both fields, and that a malformed value fails the build.
func TestExpectedDisaggregatedSetResolvesZeroRollout(t *testing.T) {
	svc := disaggTestService(t)
	svc.Spec.Replicas = ptr.To[int32](2)
	svc.Spec.RolloutStrategy = &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(0))}
	svc.Spec.Prefill.Replicas = ptr.To[int32](3)
	svc.Spec.Prefill.RolloutStrategy = &v1alpha2.RolloutStrategy{
		MaxUnavailable: ptr.To(intstr.FromString("10%")),
		MaxSurge:       ptr.To(intstr.FromInt32(0)),
	}

	ds, err := disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), nil)
	require.NoError(t, err)

	want := &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)}
	assert.Equal(t, want, ds.Spec.Roles[0].Spec.RolloutStrategy.RollingUpdateConfiguration, "decode")
	assert.Equal(t, want, ds.Spec.Roles[1].Spec.RolloutStrategy.RollingUpdateConfiguration, "prefill")

	svc.Spec.Prefill.RolloutStrategy.MaxSurge = ptr.To(intstr.FromString("lots"))
	_, err = disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), nil)
	require.ErrorContains(t, err, "prefill")
}

// TestExpectedDisaggregatedSetAutoscaledRole checks that an autoscaled role hands its
// replicas to the DisaggregatedSetRoleScaler the DisaggregatedSet creates for it, while
// a static role next to it keeps its own.
func TestExpectedDisaggregatedSetAutoscaledRole(t *testing.T) {
	svc := disaggTestService(t)
	svc.Spec.Scaling = disaggTestKEDAScaling(2, 8)
	svc.Spec.Prefill.Replicas = ptr.To[int32](3)

	ds, err := disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), nil)
	require.NoError(t, err)

	decode, prefill := ds.Spec.Roles[0], ds.Spec.Roles[1]
	assert.Equal(t, &disaggregatedsetv1.RoleScaling{Mode: disaggregatedsetv1.RoleScalingExternal}, decode.Scaling)
	assert.Nil(t, decode.Spec.Replicas, "the DisaggregatedSet ignores the replicas of an autoscaled role and warns when they are more than one")
	assert.Nil(t, prefill.Scaling)
	assert.Equal(t, ptr.To[int32](3), prefill.Spec.Replicas)
	assert.Equal(t, "llama-kserve-pd-decode", disaggregatedRoleScalerName(svc, constants.LLMDRoleDecode))
}

// TestExpectedDisaggregatedSetWVAOnlyRole checks that a role that only configures WVA,
// which is no longer supported and creates no autoscaler, stays static and keeps its
// replicas instead of handing them to a scaler nothing drives.
func TestExpectedDisaggregatedSetWVAOnlyRole(t *testing.T) {
	svc := disaggTestService(t)
	svc.Spec.Replicas = ptr.To[int32](3)
	svc.Spec.Scaling = disaggTestWVAScaling(2, 8)

	ds, err := disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), nil)
	require.NoError(t, err)

	decode := ds.Spec.Roles[0]
	assert.Nil(t, decode.Scaling)
	assert.Equal(t, ptr.To[int32](3), decode.Spec.Replicas)
}

// TestDisaggregatedRoleTargets checks that an autoscaled role is ready against the
// replicas its DisaggregatedSetRoleScaler asks for, and has no target while the
// DisaggregatedSet has not created that scaler or does not control it.
func TestDisaggregatedRoleTargets(t *testing.T) {
	ctx := context.Background()
	ds := &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{Name: "llama-kserve-pd", Namespace: disaggTestNamespace, UID: "ds-uid"},
		Spec: disaggregatedsetv1.DisaggregatedSetSpec{Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
			{Name: constants.LLMDRoleDecode, Scaling: &disaggregatedsetv1.RoleScaling{Mode: disaggregatedsetv1.RoleScalingExternal}},
			{Name: constants.LLMDRolePrefill, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{
				Spec: lwsapi.LeaderWorkerSetSpec{Replicas: ptr.To[int32](3)},
			}},
		}},
	}
	scaler := func(owner *disaggregatedsetv1.DisaggregatedSet, replicas int32) *disaggregatedsetv1.DisaggregatedSetRoleScaler {
		return &disaggregatedsetv1.DisaggregatedSetRoleScaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "llama-kserve-pd-decode",
				Namespace:       disaggTestNamespace,
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(owner, disaggregatedsetv1.GroupVersion.WithKind("DisaggregatedSet"))},
			},
			Spec: disaggregatedsetv1.DisaggregatedSetRoleScalerSpec{Replicas: replicas},
		}
	}
	foreign := ds.DeepCopy()
	foreign.UID = "other-uid"

	tests := []struct {
		name   string
		scaler *disaggregatedsetv1.DisaggregatedSetRoleScaler
		want   map[string]int32
	}{
		{name: "scaler not created yet", want: map[string]int32{constants.LLMDRolePrefill: 3}},
		{name: "scaler controlled by the DisaggregatedSet", scaler: scaler(ds, 5), want: map[string]int32{constants.LLMDRoleDecode: 5, constants.LLMDRolePrefill: 3}},
		{name: "scaler scaled to zero", scaler: scaler(ds, 0), want: map[string]int32{constants.LLMDRoleDecode: 0, constants.LLMDRolePrefill: 3}},
		{name: "scaler controlled by something else", scaler: scaler(foreign, 5), want: map[string]int32{constants.LLMDRolePrefill: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := disaggTestReconciler(t)
			if tt.scaler != nil {
				require.NoError(t, r.Create(ctx, tt.scaler))
			}
			got, err := r.disaggregatedRoleTargets(ctx, ds)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	got, err := disaggTestReconciler(t).disaggregatedRoleTargets(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDisaggregatedRollingUpdateConfig(t *testing.T) {
	deploymentDefaults := &lwsapi.RollingUpdateConfiguration{
		MaxUnavailable: intstr.FromString("25%"),
		MaxSurge:       intstr.FromString("25%"),
	}
	tests := []struct {
		name     string
		workload *v1alpha2.WorkloadSpec
		want     *lwsapi.RollingUpdateConfiguration
		wantErr  bool
	}{
		{
			name:     "single-node without a rollout strategy uses the Deployment defaults",
			workload: &v1alpha2.WorkloadSpec{Template: disaggTestPod()},
			want:     deploymentDefaults,
		},
		{
			name:     "single-node with an empty rollout strategy uses the Deployment defaults",
			workload: &v1alpha2.WorkloadSpec{Template: disaggTestPod(), RolloutStrategy: &v1alpha2.RolloutStrategy{}},
			want:     deploymentDefaults,
		},
		{
			name: "single-node with only maxUnavailable surges by the Deployment default",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxUnavailable: ptr.To(intstr.FromInt32(0))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(0), MaxSurge: intstr.FromString("25%")},
		},
		{
			name: "single-node with only maxSurge keeps the Deployment default for maxUnavailable",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(2))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromString("25%"), MaxSurge: intstr.FromInt32(2)},
		},
		{
			name: "single-node with both values uses them as set",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromInt32(1)),
					MaxSurge:       ptr.To(intstr.FromString("50%")),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromString("50%")},
		},
		// The DisaggregatedSet resizes the LeaderWorkerSet of each revision down to one
		// replica during a rollout, and the LeaderWorkerSet webhook rejects maxSurge and
		// maxUnavailable that both resolve to zero at its size. So with maxSurge: 0,
		// maxUnavailable is resolved once against the role's replicas and written as a
		// fixed number of at least one.
		{
			name: "single-node with only maxSurge: 0 on few replicas removes one replica at a time",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Replicas:        ptr.To[int32](2),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(0))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "single-node with percentages that round down to zero removes one replica at a time",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](3),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("10%")),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			// 25% of 4 is one, but 25% of the three replicas the old LeaderWorkerSet
			// shrinks to is zero.
			name: "single-node with only maxSurge: 0 on enough replicas resolves the Deployment default",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Replicas:        ptr.To[int32](4),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(0))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "single-node with maxSurge: 0 keeps the number of replicas the percentage allows",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](8),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("50%")),
					MaxSurge:       ptr.To(intstr.FromString("0%")),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(4), MaxSurge: intstr.FromString("0%")},
		},
		{
			name: "single-node with maxSurge: 0 and a fixed maxUnavailable is left as set",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](4),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromInt32(2)),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(2), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "single-node with a positive maxSurge keeps a percentage maxUnavailable",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](4),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("10%")),
					MaxSurge:       ptr.To(intstr.FromString("10%")),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromString("10%"), MaxSurge: intstr.FromString("10%")},
		},
		{
			// An autoscaled role is resolved against its minimum replicas.
			name: "autoscaled single-node is resolved against its minimum replicas",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Scaling:  disaggTestKEDAScaling(8, 16),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("25%")),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(2), MaxSurge: intstr.FromInt32(0)},
		},
		{
			// WVA is no longer supported, so the role is static and keeps its replicas.
			name: "single-node with only WVA scaling is resolved against its replicas",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](4),
				Scaling:  disaggTestWVAScaling(8, 16),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("25%")),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "autoscaled single-node with a minimum of zero is resolved against one replica",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Scaling:         disaggTestKEDAScaling(0, 4),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(0))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			// The DisaggregatedSet creates its LeaderWorkerSets with these settings even at
			// zero replicas and never updates them afterwards, only their replicas: the
			// settings must stay valid once the role scales up.
			name: "single-node scaled to zero is resolved against one replica",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Replicas:        ptr.To[int32](0),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromInt32(0))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "single-node scaled to zero with a maxUnavailable of 0 removes one replica at a time",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](0),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromInt32(0)),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name:     "single-node scaled to zero with a surge is left as set",
			workload: &v1alpha2.WorkloadSpec{Template: disaggTestPod(), Replicas: ptr.To[int32](0)},
			want:     deploymentDefaults,
		},
		{
			name: "single-node with a malformed value is an error",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxSurge: ptr.To(intstr.FromString("lots"))},
			},
			wantErr: true,
		},
		{
			name: "single-node with a negative maxUnavailable is an error",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Replicas: ptr.To[int32](4),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("-50%")),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			wantErr: true,
		},
		{
			name:     "multi-node without a rollout strategy leaves the LeaderWorkerSet defaults",
			workload: &v1alpha2.WorkloadSpec{Template: disaggTestPod(), Worker: disaggTestPod()},
			want:     nil,
		},
		{
			name: "multi-node with percentages that round down to zero removes one replica at a time",
			workload: &v1alpha2.WorkloadSpec{
				Template: disaggTestPod(),
				Worker:   disaggTestPod(),
				Replicas: ptr.To[int32](3),
				RolloutStrategy: &v1alpha2.RolloutStrategy{
					MaxUnavailable: ptr.To(intstr.FromString("10%")),
					MaxSurge:       ptr.To(intstr.FromInt32(0)),
				},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "multi-node with maxSurge: 0 keeps the number of replicas the percentage allows",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Worker:          disaggTestPod(),
				Replicas:        ptr.To[int32](4),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxUnavailable: ptr.To(intstr.FromString("50%"))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(2), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "multi-node scaled to zero is resolved against one replica",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Worker:          disaggTestPod(),
				Replicas:        ptr.To[int32](0),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxUnavailable: ptr.To(intstr.FromString("50%"))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(1), MaxSurge: intstr.FromInt32(0)},
		},
		{
			name: "multi-node with only maxUnavailable keeps the LeaderWorkerSet default for maxSurge",
			workload: &v1alpha2.WorkloadSpec{
				Template:        disaggTestPod(),
				Worker:          disaggTestPod(),
				RolloutStrategy: &v1alpha2.RolloutStrategy{MaxUnavailable: ptr.To(intstr.FromInt32(2))},
			},
			want: &lwsapi.RollingUpdateConfiguration{MaxUnavailable: intstr.FromInt32(2), MaxSurge: intstr.FromInt32(0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := disaggregatedRollingUpdateConfig(tt.workload)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExpectedDisaggregatedSetKeepsDeployedStorageInitializer(t *testing.T) {
	deployedPod := func(image string) *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: constants.StorageInitializerContainerName, Image: image}},
			Containers:     []corev1.Container{{Name: "main", Image: "vllm:deployed"}},
		}}
	}
	svc := disaggTestService(t)
	svc.Spec.Prefill.Template = nil
	svc.Spec.Prefill.Worker = disaggTestPod()
	svc.Spec.Prefill.Template = disaggTestPod()
	svc.Spec.Prefill.Parallelism = disaggTestParallelism()

	current := &disaggregatedsetv1.DisaggregatedSet{Spec: disaggregatedsetv1.DisaggregatedSetSpec{Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
		{Name: constants.LLMDRoleDecode, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{WorkerTemplate: *deployedPod(disaggDeployedInit)},
		}}},
		{Name: constants.LLMDRolePrefill, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				LeaderTemplate: deployedPod(disaggDeployedInit + "-leader"),
				WorkerTemplate: *deployedPod(disaggDeployedInit + "-worker"),
			},
		}}},
	}}}

	r := disaggTestReconciler(t)
	ds, err := r.expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), current)
	require.NoError(t, err)
	decode, prefill := ds.Spec.Roles[0].Spec.LeaderWorkerTemplate, ds.Spec.Roles[1].Spec.LeaderWorkerTemplate
	assert.Equal(t, disaggDeployedInit, storageInitializerImageIn(t, &decode.WorkerTemplate.Spec))
	assert.Equal(t, disaggDeployedInit+"-leader", storageInitializerImageIn(t, &prefill.LeaderTemplate.Spec))
	assert.Equal(t, disaggDeployedInit+"-worker", storageInitializerImageIn(t, &prefill.WorkerTemplate.Spec))

	fresh, err := r.expectedDisaggregatedSet(context.Background(), svc, disaggTestConfig(), nil)
	require.NoError(t, err)
	assert.Equal(t, disaggConfiguredInit, storageInitializerImageIn(t, &fresh.Spec.Roles[0].Spec.LeaderWorkerTemplate.WorkerTemplate.Spec))
}

func storageInitializerImageIn(t *testing.T, spec *corev1.PodSpec) string {
	t.Helper()
	require.NotNil(t, spec)
	for _, c := range spec.InitContainers {
		if c.Name == constants.StorageInitializerContainerName {
			return c.Image
		}
	}
	t.Fatalf("pod spec has no %s init container", constants.StorageInitializerContainerName)
	return ""
}

func TestExpectedDisaggregatedSetRevision(t *testing.T) {
	revisionOf := func(ds *disaggregatedsetv1.DisaggregatedSet) []string {
		revisions := make([]string, 0, len(ds.Spec.Roles))
		for _, role := range ds.Spec.Roles {
			revisions = append(revisions, role.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels[constants.LLMInferenceServiceRevisionLabelKey])
		}
		return revisions
	}
	withPlaceholders := func() *v1alpha2.LLMInferenceService {
		svc := disaggTestService(t)
		svc.Spec.Labels[constants.LLMInferenceServiceRevisionLabelKey] = ""
		svc.Spec.Prefill.Labels[constants.LLMInferenceServiceRevisionLabelKey] = ""
		return svc
	}
	r := disaggTestReconciler(t)
	ctx := context.Background()

	ds, err := r.expectedDisaggregatedSet(ctx, withPlaceholders(), disaggTestConfig(), nil)
	require.NoError(t, err)
	revisions := revisionOf(ds)
	require.Len(t, revisions, 2)
	assert.NotEmpty(t, revisions[0])
	assert.Equal(t, revisions[0], revisions[1], "decode and prefill share one revision")

	// A new configured storage-initializer image is not deployed while the role keeps
	// its current one, so it must not change the revision.
	upgradedConfig := disaggTestConfig()
	upgradedConfig.StorageConfig.Image = "kserve/storage-initializer:upgraded"
	upgraded, err := r.expectedDisaggregatedSet(ctx, withPlaceholders(), upgradedConfig, ds)
	require.NoError(t, err)
	assert.Equal(t, revisions, revisionOf(upgraded))

	changed := withPlaceholders()
	changed.Spec.Template.Containers[0].Image = "vllm:next"
	changedDS, err := r.expectedDisaggregatedSet(ctx, changed, disaggTestConfig(), ds)
	require.NoError(t, err)
	assert.NotEqual(t, revisions, revisionOf(changedDS))

	withoutPlaceholders, err := r.expectedDisaggregatedSet(ctx, disaggTestService(t), disaggTestConfig(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"", ""}, revisionOf(withoutPlaceholders))
}

func TestDisaggregatedRoleReadiness(t *testing.T) {
	type role struct {
		replicas       int32
		maxUnavailable *intstr.IntOrString
	}
	dsWith := func(generation, observed int64, r role, statuses []disaggregatedsetv1.RoleStatus, conditions ...metav1.Condition) *disaggregatedsetv1.DisaggregatedSet {
		spec := lwsapi.LeaderWorkerSetSpec{Replicas: ptr.To(r.replicas)}
		if r.maxUnavailable != nil {
			spec.RolloutStrategy.RollingUpdateConfiguration = &lwsapi.RollingUpdateConfiguration{MaxUnavailable: *r.maxUnavailable, MaxSurge: intstr.FromString("25%")}
		}
		return &disaggregatedsetv1.DisaggregatedSet{
			ObjectMeta: metav1.ObjectMeta{Generation: generation},
			Spec: disaggregatedsetv1.DisaggregatedSetSpec{Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
				{Name: constants.LLMDRoleDecode, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{Spec: spec}},
			}},
			Status: disaggregatedsetv1.DisaggregatedSetStatus{ObservedGeneration: observed, RoleStatuses: statuses, Conditions: conditions},
		}
	}
	status := func(replicas, ready, updated int32) []disaggregatedsetv1.RoleStatus {
		return []disaggregatedsetv1.RoleStatus{{Name: constants.LLMDRoleDecode, Replicas: replicas, ReadyReplicas: ready, UpdatedReplicas: updated}}
	}
	unavailable := metav1.Condition{
		Type:    string(disaggregatedsetv1.DisaggregatedSetAvailable),
		Status:  metav1.ConditionFalse,
		Reason:  "RolloutInProgress",
		Message: "rolling out",
	}
	two := role{replicas: 2}
	// Four replicas with the Deployment default maxUnavailable of 25% need three ready.
	four := role{replicas: 4, maxUnavailable: ptr.To(intstr.FromString("25%"))}

	tests := []struct {
		name      string
		multiNode bool
		ds        *disaggregatedsetv1.DisaggregatedSet
		// desired overrides the role's replicas as its target, as the
		// DisaggregatedSetRoleScaler of an autoscaled role does; noTarget leaves the
		// role without one, as before its scaler exists.
		desired     *int32
		noTarget    bool
		wantReady   bool
		wantReason  string
		wantMessage string
	}{
		{name: "no DisaggregatedSet", ds: nil, wantReason: "Progressing"},
		{name: "generation not observed", ds: dsWith(2, 1, four, status(4, 4, 4)), wantReason: "Progressing"},
		{name: "unavailable reason is surfaced", ds: dsWith(1, 1, four, nil, unavailable), wantReason: "RolloutInProgress", wantMessage: "rolling out"},

		// Single-node roles replace Deployments, so they are ready like a Deployment is
		// available: while ready replicas of any revision cover desired - maxUnavailable.
		{name: "single-node with all replicas ready and updated", ds: dsWith(1, 1, four, status(4, 4, 4)), wantReady: true},
		{name: "single-node starting a rollout", ds: dsWith(1, 1, four, status(5, 4, 1)), wantReady: true},
		{
			// Two old and two of four new replicas are ready: enough are serving.
			name:      "single-node mid-rollout with enough replicas serving",
			ds:        dsWith(1, 1, four, status(6, 4, 4)),
			wantReady: true,
		},
		{name: "single-node with exactly desired - maxUnavailable ready", ds: dsWith(1, 1, four, status(4, 3, 4)), wantReady: true},
		{
			name:        "single-node with too few replicas serving",
			ds:          dsWith(1, 1, four, status(4, 2, 4)),
			wantReason:  "Progressing",
			wantMessage: "decode role has 2 ready replicas, needs at least 3 of 4",
		},
		{
			// Scaling 4 -> 2: the extra replicas are still counted until they are gone.
			name:      "single-node scaling down",
			ds:        dsWith(1, 1, role{replicas: 2, maxUnavailable: ptr.To(intstr.FromString("25%"))}, status(4, 4, 4)),
			wantReady: true,
		},
		{
			// maxUnavailable: 1 is what a single-node role gets when its values round to 0/0.
			name:      "single-node allowed one replica down",
			ds:        dsWith(1, 1, role{replicas: 2, maxUnavailable: ptr.To(intstr.FromInt32(1))}, status(2, 1, 2)),
			wantReady: true,
		},
		{name: "single-node scaled to zero", ds: dsWith(1, 1, role{replicas: 0, maxUnavailable: ptr.To(intstr.FromString("25%"))}, status(0, 0, 0)), wantReady: true},

		// Multi-node roles replace LeaderWorkerSets, which are ready only once every
		// desired replica is ready and updated.
		{name: "multi-node with all replicas ready and updated", multiNode: true, ds: dsWith(1, 1, two, status(2, 2, 2)), wantReady: true},
		{name: "multi-node with replicas not updated", multiNode: true, ds: dsWith(1, 1, two, status(2, 2, 1)), wantReason: "Progressing", wantMessage: "2/2 replicas ready, 1 updated and 2 in total"},
		{
			// One old replica and one new replica are ready while the other new replica
			// starts: the ready old replica must not stand in for the unready new one.
			name:        "multi-node mid-rollout does not count ready old replicas",
			multiNode:   true,
			ds:          dsWith(1, 1, two, status(3, 2, 2)),
			wantReason:  "Progressing",
			wantMessage: "2/2 replicas ready, 2 updated and 3 in total",
		},
		{name: "multi-node with old replicas not yet removed", multiNode: true, ds: dsWith(1, 1, two, status(3, 3, 2)), wantReason: "Progressing", wantMessage: "3/2 replicas ready, 2 updated and 3 in total"},
		{name: "multi-node mid-rollout with enough replicas serving is not ready", multiNode: true, ds: dsWith(1, 1, four, status(6, 4, 4)), wantReason: "Progressing"},

		// An autoscaled role is ready against the replicas its DisaggregatedSetRoleScaler
		// asks for, not its own replicas.
		{
			name:        "autoscaled single-node scaling up is not ready until enough new replicas serve",
			ds:          dsWith(1, 1, four, status(8, 4, 8)),
			desired:     ptr.To[int32](8),
			wantReason:  "Progressing",
			wantMessage: "decode role has 4 ready replicas, needs at least 6 of 8",
		},
		{name: "autoscaled single-node scaled up", ds: dsWith(1, 1, four, status(8, 8, 8)), desired: ptr.To[int32](8), wantReady: true},
		{name: "autoscaled single-node scaled to zero", ds: dsWith(1, 1, four, status(0, 0, 0)), desired: ptr.To[int32](0), wantReady: true},
		{name: "autoscaled multi-node scaled up", multiNode: true, ds: dsWith(1, 1, two, status(3, 3, 3)), desired: ptr.To[int32](3), wantReady: true},
		{
			name:        "autoscaled role without its scaler",
			ds:          dsWith(1, 1, four, status(4, 4, 4)),
			noTarget:    true,
			wantReason:  "Progressing",
			wantMessage: "decode role is waiting for DisaggregatedSetRoleScaler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := tt.desired
			if desired == nil && !tt.noTarget {
				if role := findDisaggregatedRole(tt.ds, constants.LLMDRoleDecode); role != nil {
					desired = ptr.To(ptr.Deref(role.Spec.Replicas, 1))
				}
			}
			reason, message, ready := disaggregatedRoleReadiness(tt.ds, constants.LLMDRoleDecode, tt.multiNode, desired)
			assert.Equal(t, tt.wantReady, ready)
			assert.Equal(t, tt.wantReason, reason)
			assert.Contains(t, message, tt.wantMessage)
		})
	}
}

func TestSemanticDisaggregatedSetIsEqual(t *testing.T) {
	expected, err := disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), disaggTestService(t), disaggTestConfig(), nil)
	require.NoError(t, err)

	defaulted := expected.DeepCopy()
	defaulted.Spec.Slices = ptr.To[int32](1)
	defaulted.Spec.Roles[0].Spec.LeaderWorkerTemplate.SubGroupPolicy = nil
	defaulted.Labels["added-by-someone-else"] = "x"
	assert.True(t, semanticDisaggregatedSetIsEqual(expected, defaulted), "values added by the API server or others are ignored")

	removedEnv := expected.DeepCopy()
	removedEnv.Spec.Roles[1].Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].Image = "vllm:other"
	assert.False(t, semanticDisaggregatedSetIsEqual(expected, removedEnv), "pod spec differences are detected")

	reordered := expected.DeepCopy()
	reordered.Spec.Roles[0], reordered.Spec.Roles[1] = reordered.Spec.Roles[1], reordered.Spec.Roles[0]
	assert.False(t, semanticDisaggregatedSetIsEqual(expected, reordered))

	external := expected.DeepCopy()
	external.Spec.Roles[0].Scaling = &disaggregatedsetv1.RoleScaling{Mode: disaggregatedsetv1.RoleScalingExternal}
	assert.False(t, semanticDisaggregatedSetIsEqual(expected, external), "a static role is not equal to an External one")

	autoscaledSvc := disaggTestService(t)
	autoscaledSvc.Spec.Scaling = disaggTestKEDAScaling(1, 3)
	autoscaled, err := disaggTestReconciler(t).expectedDisaggregatedSet(context.Background(), autoscaledSvc, disaggTestConfig(), nil)
	require.NoError(t, err)
	assert.False(t, semanticDisaggregatedSetIsEqual(autoscaled, expected), "an External role is not equal to a static one")
	assert.True(t, semanticDisaggregatedSetIsEqual(autoscaled, autoscaled.DeepCopy()))
}

func TestEnqueueOnDisaggregatedSetRoleScalerChange(t *testing.T) {
	svc := disaggTestService(t)
	svc.UID = "svc-uid"
	ds := &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      disaggregatedSetName(svc),
			Namespace: svc.Namespace,
			UID:       "ds-uid",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(svc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}
	foreignDS := &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: svc.Namespace, UID: "foreign-uid"},
	}
	scalerOwnedBy := func(owner *disaggregatedsetv1.DisaggregatedSet) *disaggregatedsetv1.DisaggregatedSetRoleScaler {
		return &disaggregatedsetv1.DisaggregatedSetRoleScaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      owner.Name + "-" + constants.LLMDRoleDecode,
				Namespace: owner.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(owner, disaggregatedsetv1.GroupVersion.WithKind("DisaggregatedSet")),
				},
			},
		}
	}

	tests := []struct {
		name   string
		scaler *disaggregatedsetv1.DisaggregatedSetRoleScaler
		want   []reconcile.Request
	}{
		{
			name:   "a scaler of a service's DisaggregatedSet enqueues the service",
			scaler: scalerOwnedBy(ds),
			want:   []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}},
		},
		{
			name:   "a scaler of a DisaggregatedSet no service owns is ignored",
			scaler: scalerOwnedBy(foreignDS),
		},
		{
			name: "a scaler of a DisaggregatedSet that is gone is ignored",
			scaler: scalerOwnedBy(&disaggregatedsetv1.DisaggregatedSet{
				ObjectMeta: metav1.ObjectMeta{Name: "gone", Namespace: svc.Namespace, UID: "gone-uid"},
			}),
		},
		{
			name: "a scaler no DisaggregatedSet controls is ignored",
			scaler: &disaggregatedsetv1.DisaggregatedSetRoleScaler{
				ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: svc.Namespace},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := disaggTestReconciler(t)
			require.NoError(t, r.Create(t.Context(), ds.DeepCopy()))
			require.NoError(t, r.Create(t.Context(), foreignDS.DeepCopy()))

			got := r.enqueueOnDisaggregatedSetRoleScalerChange(logr.Discard())(t.Context(), tt.scaler)

			assert.Equal(t, tt.want, got)
		})
	}
}
