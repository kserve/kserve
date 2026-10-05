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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
		Client:                    fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Clientset:                 k8sfake.NewClientset(),
		DisaggregatedSetAvailable: true,
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
			name: "decode autoscaling",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Scaling = &v1alpha2.ScalingSpec{MaxReplicas: 3}
			},
			wantReason: reasonAutoscalingNotSupported,
		},
		{
			name: "prefill autoscaling",
			mutate: func(svc *v1alpha2.LLMInferenceService, _ *Config, _ *LLMISVCReconciler) {
				svc.Spec.Prefill.Scaling = &v1alpha2.ScalingSpec{MaxReplicas: 3}
			},
			wantReason: reasonAutoscalingNotSupported,
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
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/explicit=%t", tt.name, explicit), func(t *testing.T) {
				svc, config, r := disaggTestService(t), disaggTestConfig(), &LLMISVCReconciler{DisaggregatedSetAvailable: true}
				tt.mutate(svc, config, r)

				got := r.decideDisaggregatedSet(svc, config, explicit)
				assert.Equal(t, tt.wantUse, got.Use)
				assert.Equal(t, !tt.wantNoReason, got.Requested)
				assert.Equal(t, explicit && got.Requested, got.Explicit, "only a request records who made it")
				assert.Equal(t, tt.wantReason, got.Reason)
				if tt.wantReason != "" {
					assert.NotEmpty(t, got.Message)
				}
			})
		}
	}
}

func TestMarkDisaggregatedSetDecision(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	r := disaggTestReconciler(t)
	r.EventRecorder = recorder
	svc := disaggTestService(t)
	gateOff := disaggregatedSetDecision{Requested: true, Explicit: true, Reason: reasonFeatureGateDisabled, Message: "gate off"}
	noCRD := disaggregatedSetDecision{Requested: true, Explicit: true, Reason: reasonCRDNotInstalled, Message: "no CRD"}

	steps := []struct {
		name       string
		decision   disaggregatedSetDecision
		wantStatus corev1.ConditionStatus
		wantReason string
		wantEvent  string
	}{
		{name: "first fallback emits an event", decision: gateOff, wantStatus: corev1.ConditionFalse, wantReason: reasonFeatureGateDisabled, wantEvent: disaggregatedSetNotUsedReason},
		{name: "same fallback is quiet", decision: gateOff, wantStatus: corev1.ConditionFalse, wantReason: reasonFeatureGateDisabled},
		{name: "new fallback reason emits an event", decision: noCRD, wantStatus: corev1.ConditionFalse, wantReason: reasonCRDNotInstalled, wantEvent: disaggregatedSetNotUsedReason},
		{name: "used", decision: disaggregatedSetDecision{Requested: true, Explicit: true, Use: true}, wantStatus: corev1.ConditionTrue},
		// Leaving the backend is reported once, as a migration that carries the reason.
		{name: "fallback after use reports the migration", decision: noCRD, wantStatus: corev1.ConditionFalse, wantReason: reasonCRDNotInstalled, wantEvent: disaggregatedSetMigratingFromReason},
		{name: "not requested clears the condition", decision: disaggregatedSetDecision{}},
	}
	for _, step := range steps {
		r.markDisaggregatedSetDecision(context.Background(), svc, step.decision)

		cond := svc.Status.GetCondition(v1alpha2.DisaggregatedSetUsed)
		if step.wantStatus == "" {
			assert.Nil(t, cond, step.name)
		} else if assert.NotNil(t, cond, step.name) {
			assert.Equal(t, step.wantStatus, cond.Status, step.name)
			assert.Equal(t, step.wantReason, cond.Reason, step.name)
		}
		assertEvents(t, recorder, step.name, step.wantEvent)
	}
}

// TestDisaggregatedSetMigrationEvents covers the warning a running service gets when it
// moves onto or off a DisaggregatedSet, which replaces its workloads without waiting for
// the new pods to become ready.
func TestDisaggregatedSetMigrationEvents(t *testing.T) {
	use := disaggregatedSetDecision{Requested: true, Use: true}
	deployment := func(name string) client.Object {
		return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: disaggTestNamespace}}
	}
	lws := func(name string) client.Object {
		return &lwsapi.LeaderWorkerSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: disaggTestNamespace}}
	}

	tests := []struct {
		name          string
		prev          func(svc *v1alpha2.LLMInferenceService)
		existing      func(svc *v1alpha2.LLMInferenceService) []client.Object
		decision      disaggregatedSetDecision
		wantEvent     string
		wantInMessage string
	}{
		{
			name:     "a new service starts on the DisaggregatedSet without a warning",
			decision: use,
		},
		{
			name: "a service on Deployments moves onto the DisaggregatedSet",
			existing: func(svc *v1alpha2.LLMInferenceService) []client.Object {
				return []client.Object{deployment(mainDeploymentName(svc)), deployment(prefillDeploymentName(svc))}
			},
			decision:      use,
			wantEvent:     disaggregatedSetMigratingToReason,
			wantInMessage: "unavailable",
		},
		{
			name: "a service on LeaderWorkerSets moves onto the DisaggregatedSet",
			existing: func(svc *v1alpha2.LLMInferenceService) []client.Object {
				return []client.Object{lws(mainLWSName(svc)), lws(prefillLWSName(svc))}
			},
			decision:  use,
			wantEvent: disaggregatedSetMigratingToReason,
		},
		{
			name: "a service that fell back moves onto the DisaggregatedSet once it can",
			prev: func(svc *v1alpha2.LLMInferenceService) {
				svc.MarkDisaggregatedSetNotUsed(reasonFeatureGateDisabled, "gate off")
			},
			existing: func(svc *v1alpha2.LLMInferenceService) []client.Object {
				return []client.Object{deployment(mainDeploymentName(svc))}
			},
			decision:  use,
			wantEvent: disaggregatedSetMigratingToReason,
		},
		{
			name:     "a service already on the DisaggregatedSet stays quiet",
			prev:     func(svc *v1alpha2.LLMInferenceService) { svc.MarkDisaggregatedSetUsed() },
			decision: use,
		},
		{
			name:          "opting out moves the service off the DisaggregatedSet",
			prev:          func(svc *v1alpha2.LLMInferenceService) { svc.MarkDisaggregatedSetUsed() },
			decision:      disaggregatedSetDecision{},
			wantEvent:     disaggregatedSetMigratingFromReason,
			wantInMessage: constants.LLMDisaggregatedSetAnnotationKey,
		},
		{
			name: "falling back moves the service off the DisaggregatedSet and says why",
			prev: func(svc *v1alpha2.LLMInferenceService) { svc.MarkDisaggregatedSetUsed() },
			decision: disaggregatedSetDecision{
				Requested: true, Reason: reasonAutoscalingNotSupported, Message: "autoscaling is not supported",
			},
			wantEvent:     disaggregatedSetMigratingFromReason,
			wantInMessage: "autoscaling is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := disaggTestService(t)
			if tt.prev != nil {
				tt.prev(svc)
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

// assertEvents checks that the recorder holds exactly the wanted event reason, or none
// when want is empty, and drains it.
func assertEvents(t *testing.T, recorder *record.FakeRecorder, step, want string) {
	t.Helper()
	if want == "" {
		assert.Empty(t, recorder.Events, step)
		return
	}
	if assert.Len(t, recorder.Events, 1, step) {
		assert.Contains(t, <-recorder.Events, " "+want+" ", step)
	}
}

// TestMarkDisaggregatedSetDecisionFromPreset covers a service that takes the
// DisaggregatedSet default from its presets rather than asking for it. Falling back is
// recorded in the condition, but the service did nothing to act on, so no warning is
// emitted.
func TestMarkDisaggregatedSetDecisionFromPreset(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	r := disaggTestReconciler(t)
	r.EventRecorder = recorder
	svc := disaggTestService(t)

	steps := []struct {
		name       string
		decision   disaggregatedSetDecision
		wantStatus corev1.ConditionStatus
		wantReason string
	}{
		{name: "fallback", decision: disaggregatedSetDecision{Requested: true, Reason: reasonFeatureGateDisabled, Message: "gate off"}, wantStatus: corev1.ConditionFalse, wantReason: reasonFeatureGateDisabled},
		{name: "new fallback reason", decision: disaggregatedSetDecision{Requested: true, Reason: reasonAutoscalingNotSupported, Message: "scaling"}, wantStatus: corev1.ConditionFalse, wantReason: reasonAutoscalingNotSupported},
		{name: "used", decision: disaggregatedSetDecision{Requested: true, Use: true}, wantStatus: corev1.ConditionTrue},
	}
	for _, step := range steps {
		r.markDisaggregatedSetDecision(context.Background(), svc, step.decision)

		cond := svc.Status.GetCondition(v1alpha2.DisaggregatedSetUsed)
		if assert.NotNil(t, cond, step.name) {
			assert.Equal(t, step.wantStatus, cond.Status, step.name)
			assert.Equal(t, step.wantReason, cond.Reason, step.name)
		}
		assert.Empty(t, recorder.Events, step.name)
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
				svc.Spec.Worker = disaggTestPod()
				svc.Spec.Parallelism = disaggTestParallelism()
				svc.Spec.Prefill.Worker = disaggTestPod()
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc := disaggTestService(t)
			tt.mutate(svc)
			r := disaggTestReconciler(t)

			ds, err := r.expectedDisaggregatedSet(ctx, svc.DeepCopy(), disaggTestConfig(), nil)
			require.NoError(t, err)
			require.Len(t, ds.Spec.Roles, 2)
			decode, prefill := ds.Spec.Roles[0], ds.Spec.Roles[1]
			require.Equal(t, constants.LLMDRoleDecode, decode.Name)
			require.Equal(t, constants.LLMDRolePrefill, prefill.Name)

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
		assert.Equal(t, lwsapi.RecreateGroupOnPodRestart, role.LeaderWorkerTemplate.RestartPolicy)
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

func TestDisaggregatedRollingUpdateConfig(t *testing.T) {
	deploymentDefaults := &lwsapi.RollingUpdateConfiguration{
		MaxUnavailable: intstr.FromString("25%"),
		MaxSurge:       intstr.FromString("25%"),
	}
	tests := []struct {
		name     string
		workload *v1alpha2.WorkloadSpec
		want     *lwsapi.RollingUpdateConfiguration
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
		{
			name:     "multi-node without a rollout strategy leaves the LeaderWorkerSet defaults",
			workload: &v1alpha2.WorkloadSpec{Template: disaggTestPod(), Worker: disaggTestPod()},
			want:     nil,
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
			assert.Equal(t, tt.want, disaggregatedRollingUpdateConfig(tt.workload))
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
	dsWith := func(generation, observed int64, statuses []disaggregatedsetv1.RoleStatus, conditions ...metav1.Condition) *disaggregatedsetv1.DisaggregatedSet {
		return &disaggregatedsetv1.DisaggregatedSet{
			ObjectMeta: metav1.ObjectMeta{Generation: generation},
			Spec: disaggregatedsetv1.DisaggregatedSetSpec{Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
				{Name: constants.LLMDRoleDecode, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{Spec: lwsapi.LeaderWorkerSetSpec{Replicas: ptr.To[int32](2)}}},
			}},
			Status: disaggregatedsetv1.DisaggregatedSetStatus{ObservedGeneration: observed, RoleStatuses: statuses, Conditions: conditions},
		}
	}
	unavailable := metav1.Condition{
		Type:    string(disaggregatedsetv1.DisaggregatedSetAvailable),
		Status:  metav1.ConditionFalse,
		Reason:  "RolloutInProgress",
		Message: "rolling out",
	}

	tests := []struct {
		name        string
		ds          *disaggregatedsetv1.DisaggregatedSet
		wantReady   bool
		wantReason  string
		wantMessage string
	}{
		{name: "no DisaggregatedSet", ds: nil, wantReason: "Progressing"},
		{name: "generation not observed", ds: dsWith(2, 1, []disaggregatedsetv1.RoleStatus{{Name: constants.LLMDRoleDecode, Replicas: 2, ReadyReplicas: 2, UpdatedReplicas: 2}}), wantReason: "Progressing"},
		{name: "all replicas ready and updated", ds: dsWith(1, 1, []disaggregatedsetv1.RoleStatus{{Name: constants.LLMDRoleDecode, Replicas: 2, ReadyReplicas: 2, UpdatedReplicas: 2}}), wantReady: true},
		{name: "replicas not updated", ds: dsWith(1, 1, []disaggregatedsetv1.RoleStatus{{Name: constants.LLMDRoleDecode, Replicas: 2, ReadyReplicas: 2, UpdatedReplicas: 1}}), wantReason: "Progressing", wantMessage: "2/2 replicas ready, 1 updated and 2 in total"},
		{
			// One old replica and one new replica are ready while the other new replica
			// starts: the ready old replica must not stand in for the unready new one.
			name:        "ready old replicas count towards ready during a rollout",
			ds:          dsWith(1, 1, []disaggregatedsetv1.RoleStatus{{Name: constants.LLMDRoleDecode, Replicas: 3, ReadyReplicas: 2, UpdatedReplicas: 2}}),
			wantReason:  "Progressing",
			wantMessage: "2/2 replicas ready, 2 updated and 3 in total",
		},
		{
			name:        "old replicas not yet removed",
			ds:          dsWith(1, 1, []disaggregatedsetv1.RoleStatus{{Name: constants.LLMDRoleDecode, Replicas: 3, ReadyReplicas: 3, UpdatedReplicas: 2}}),
			wantReason:  "Progressing",
			wantMessage: "3/2 replicas ready, 2 updated and 3 in total",
		},
		{name: "unavailable reason is surfaced", ds: dsWith(1, 1, nil, unavailable), wantReason: "RolloutInProgress", wantMessage: "rolling out"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, message, ready := disaggregatedRoleReadiness(tt.ds, constants.LLMDRoleDecode)
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
}
