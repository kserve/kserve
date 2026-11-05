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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"
	dsctrl "sigs.k8s.io/lws/pkg/controllers/disaggregatedset"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	apiGroupDisaggregatedSet = "disaggregatedset.x-k8s.io"

	disaggregatedSetNameSuffix = "-kserve-pd"

	// disaggregatedSetNameMaxLength keeps the names the DisaggregatedSet generates within
	// the 63-character limit its webhook enforces. A role's LeaderWorkerSet is named
	// <ds>-<slice>-<revision:8>-<role> and its worker pods carry the StatefulSet label
	// <lws>-<group>-<hash:10>. With one slice, the longest role name ("prefill") and up
	// to 10,000 groups per role, that leaves 63-35 characters for the DisaggregatedSet.
	disaggregatedSetNameMaxLength = 28

	// disaggregatedSetMigratingToReason and disaggregatedSetMigratingFromReason are the
	// event reasons emitted when a running service moves onto or off a DisaggregatedSet.
	// Both replace the workloads without waiting for the new pods to become ready.
	disaggregatedSetMigratingToReason   = "MigratingToDisaggregatedSet"
	disaggregatedSetMigratingFromReason = "MigratingFromDisaggregatedSet"
)

// Reasons for a False DisaggregatedSetUsed condition. They are part of the API.
const (
	reasonFeatureGateDisabled    = "FeatureGateDisabled"
	reasonCRDNotInstalled        = "CRDNotInstalled"
	reasonNoPrefillWorkload      = "NoPrefillWorkload"
	reasonRoleScalerNotInstalled = "RoleScalerNotInstalled"
	reasonReplicasMismatch       = "ReplicasMismatch"
)

// disaggregatedSetRoles lists the roles of a DisaggregatedSet in the order they appear
// in its spec.
var disaggregatedSetRoles = []string{constants.LLMDRoleDecode, constants.LLMDRolePrefill}

// disaggregatedSetDecision reports whether a service runs on the DisaggregatedSet
// backend. The zero value means the service did not ask for it. Reason and Message say
// why a service that asked for it cannot use it.
type disaggregatedSetDecision struct {
	Requested bool
	Use       bool
	Reason    string
	Message   string
}

// decideDisaggregatedSet decides whether a service runs on the DisaggregatedSet
// backend. It takes the spec after base configurations are merged, which is why these
// checks live here rather than in admission.
func (r *LLMISVCReconciler) decideDisaggregatedSet(llmSvc *v1alpha2.LLMInferenceService, config *Config) disaggregatedSetDecision {
	if !llmSvc.DisaggregatedSetRequested() {
		return disaggregatedSetDecision{}
	}

	notUsed := func(reason, message string) disaggregatedSetDecision {
		return disaggregatedSetDecision{Requested: true, Reason: reason, Message: message}
	}
	switch {
	case !config.FeatureGates.DisaggregatedSet:
		return notUsed(reasonFeatureGateDisabled, "the DisaggregatedSet feature gate is disabled")
	case !r.DisaggregatedSetAvailable:
		return notUsed(reasonCRDNotInstalled, "the DisaggregatedSet CRD is not installed")
	case llmSvc.Spec.Prefill == nil:
		return notUsed(reasonNoPrefillWorkload, "the service has no prefill workload")
	case (disaggregatedRoleAutoscaled(&llmSvc.Spec.WorkloadSpec) || disaggregatedRoleAutoscaled(llmSvc.Spec.Prefill)) && !r.DisaggregatedSetRoleScalerAvailable:
		return notUsed(reasonRoleScalerNotInstalled, "autoscaling on a DisaggregatedSet requires the DisaggregatedSetRoleScaler CRD from LWS v0.10.0 or later")
	// A DisaggregatedSet checks this only across roles with static replicas, so it
	// applies only when neither role is autoscaled.
	case !disaggregatedRoleAutoscaled(&llmSvc.Spec.WorkloadSpec) && !disaggregatedRoleAutoscaled(llmSvc.Spec.Prefill) &&
		(disaggregatedRoleReplicas(llmSvc.Spec.Replicas) == 0) != (disaggregatedRoleReplicas(llmSvc.Spec.Prefill.Replicas) == 0):
		return notUsed(reasonReplicasMismatch, "a DisaggregatedSet requires decode and prefill replicas to be both zero or both non-zero")
	}
	return disaggregatedSetDecision{Requested: true, Use: true}
}

// markDisaggregatedSetDecision records the decision in the DisaggregatedSetUsed
// condition, whose reason and message say why a service that requests the backend keeps
// its current workloads. It decides from the decision and the workloads that exist,
// never from the service's previous status, which can be stale or lost.
//
// A running service that moves onto or off a DisaggregatedSet also gets a migration
// warning on the reconcile that replaces its workloads: the switch deletes the old
// workloads without waiting for the new pods, so the service is unavailable until they
// are ready. Stopping a service deletes its workloads too, but is not a migration.
func (r *LLMISVCReconciler) markDisaggregatedSetDecision(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, decision disaggregatedSetDecision) {
	running := !utils.GetForceStopRuntime(llmSvc)

	if decision.Use {
		if running && r.hasNonDisaggregatedWorkloads(ctx, llmSvc) {
			r.Eventf(llmSvc, corev1.EventTypeWarning, disaggregatedSetMigratingToReason,
				"moving the prefill and decode workloads from Deployments or LeaderWorkerSets to DisaggregatedSet %s; "+
					"the old workloads are deleted without waiting for the new pods, so the service is unavailable until they are ready",
				disaggregatedSetName(llmSvc))
		}
		llmSvc.MarkDisaggregatedSetUsed()
		return
	}

	if running && r.hasDisaggregatedSet(ctx, llmSvc) {
		why := decision.Message
		if !decision.Requested {
			why = constants.LLMDisaggregatedSetAnnotationKey + " is no longer \"true\""
		}
		r.Eventf(llmSvc, corev1.EventTypeWarning, disaggregatedSetMigratingFromReason,
			"moving the prefill and decode workloads from DisaggregatedSet %s to Deployments or LeaderWorkerSets because %s; "+
				"the DisaggregatedSet is deleted without waiting for the new pods, so the service is unavailable until they are ready",
			disaggregatedSetName(llmSvc), why)
	}

	if decision.Requested {
		llmSvc.MarkDisaggregatedSetNotUsed(decision.Reason, "%s", decision.Message)
	} else {
		llmSvc.MarkDisaggregatedSetUsedUnset()
	}
}

// hasNonDisaggregatedWorkloads reports whether the service still runs Deployments or
// LeaderWorkerSets, which moving onto a DisaggregatedSet replaces.
func (r *LLMISVCReconciler) hasNonDisaggregatedWorkloads(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) bool {
	return r.anyExists(ctx,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: mainDeploymentName(llmSvc), Namespace: llmSvc.GetNamespace()}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: prefillDeploymentName(llmSvc), Namespace: llmSvc.GetNamespace()}},
		&lwsapi.LeaderWorkerSet{ObjectMeta: metav1.ObjectMeta{Name: mainLWSName(llmSvc), Namespace: llmSvc.GetNamespace()}},
		&lwsapi.LeaderWorkerSet{ObjectMeta: metav1.ObjectMeta{Name: prefillLWSName(llmSvc), Namespace: llmSvc.GetNamespace()}},
	)
}

// hasDisaggregatedSet reports whether the service still runs a DisaggregatedSet, which
// moving off it deletes.
func (r *LLMISVCReconciler) hasDisaggregatedSet(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) bool {
	return r.anyExists(ctx,
		&disaggregatedsetv1.DisaggregatedSet{ObjectMeta: metav1.ObjectMeta{Name: disaggregatedSetName(llmSvc), Namespace: llmSvc.GetNamespace()}},
	)
}

// anyExists reports whether any of the objects exists, reading the cache. It only
// decides whether to emit an event, so a failed read is logged and counts as missing.
func (r *LLMISVCReconciler) anyExists(ctx context.Context, objs ...client.Object) bool {
	for _, obj := range objs {
		err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		switch {
		case err == nil:
			return true
		case apierrors.IsNotFound(err) || meta.IsNoMatchError(err):
		default:
			log.FromContext(ctx).Error(err, "Failed to check whether a workload exists", "kind", fmt.Sprintf("%T", obj), "name", obj.GetName())
		}
	}
	return false
}

// disaggregatedRoleReplicas returns the replicas of a role. A DisaggregatedSet rejects
// a mix of set and unset replicas, so an unset value is written as the default of 1.
func disaggregatedRoleReplicas(replicas *int32) int32 {
	return ptr.Deref(replicas, 1)
}

// disaggregatedRoleAutoscaled reports whether an autoscaler sets the replicas of a role.
// Only KEDA does: WVA is no longer supported, so scaling that only configures WVA
// creates no autoscaler and the role keeps its own replicas.
func disaggregatedRoleAutoscaled(workload *v1alpha2.WorkloadSpec) bool {
	return workload.Scaling != nil && workload.Scaling.KEDA != nil
}

// disaggregatedRoleSizingReplicas returns the replicas the rollout settings of a role
// are resolved against: its replicas, or for an autoscaled role, which sets none, its
// minimum replicas and at least one.
func disaggregatedRoleSizingReplicas(workload *v1alpha2.WorkloadSpec) int32 {
	if disaggregatedRoleAutoscaled(workload) {
		return max(ptr.Deref(workload.Scaling.MinReplicas, 1), 1)
	}
	return disaggregatedRoleReplicas(workload.Replicas)
}

func disaggregatedSetName(llmSvc *v1alpha2.LLMInferenceService) string {
	return boundedChildName(llmSvc.GetName(), disaggregatedSetNameSuffix, disaggregatedSetNameMaxLength)
}

// disaggregatedRoleScalerName returns the name of the DisaggregatedSetRoleScaler the
// DisaggregatedSet creates for an autoscaled role.
func disaggregatedRoleScalerName(llmSvc *v1alpha2.LLMInferenceService, role string) string {
	return dsctrl.ScalerName(disaggregatedSetName(llmSvc), role)
}

// boundedChildName returns parent+suffix, or when that exceeds maxLength, a truncated
// parent followed by a hash of the full parent and the suffix, so distinct parents keep
// distinct names.
func boundedChildName(parent, suffix string, maxLength int) string {
	if len(parent)+len(suffix) <= maxLength {
		return parent + suffix
	}
	sum := sha256.Sum256([]byte(parent))
	hash := hex.EncodeToString(sum[:])[:8]
	head := strings.TrimRight(parent[:maxLength-len(suffix)-len(hash)-1], "-")
	return head + "-" + hash + suffix
}

// reconcileDisaggregatedSet creates or updates the DisaggregatedSet of a service that
// runs on the DisaggregatedSet backend, and returns it as the API server returned it.
func (r *LLMISVCReconciler) reconcileDisaggregatedSet(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, useDisaggregatedSet bool) (*disaggregatedsetv1.DisaggregatedSet, error) {
	if !useDisaggregatedSet {
		return nil, r.deleteDisaggregatedSet(ctx, llmSvc)
	}

	current, err := r.currentDisaggregatedSet(ctx, llmSvc)
	if err != nil {
		return nil, err
	}
	expected, err := r.expectedDisaggregatedSet(ctx, llmSvc, config, current)
	if err != nil {
		return nil, fmt.Errorf("failed to build the expected DisaggregatedSet: %w", err)
	}
	// current was just read, so create or update directly rather than through Reconcile,
	// which would read it again.
	if current == nil {
		err = Create(ctx, r, llmSvc, expected)
	} else {
		err = Update(ctx, r, llmSvc, current, expected, semanticDisaggregatedSetIsEqual, PreserveDisaggregatedSetControllerAnnotations())
	}
	if err != nil {
		return nil, err
	}
	// Read the stored object back, as the Deployment and LeaderWorkerSet paths do, rather
	// than returning expected: when nothing changed, Update leaves expected holding its
	// dry-run response, whose generation is ahead of the stored one if the stored object
	// has a field the controller does not set, so readiness would never catch up.
	return r.currentDisaggregatedSet(ctx, llmSvc)
}

// PreserveDisaggregatedSetControllerAnnotations returns an UpdateOption that copies the
// annotations in the DisaggregatedSet API group from the stored DisaggregatedSet to the
// one being written, unless the latter sets them. Those annotations are set by the
// DisaggregatedSet controller.
func PreserveDisaggregatedSetControllerAnnotations() UpdateOption[*disaggregatedsetv1.DisaggregatedSet] {
	return AfterDryRun(func(expected, expectedGiven, curr *disaggregatedsetv1.DisaggregatedSet) {
		for k, v := range curr.Annotations {
			if !strings.HasPrefix(k, apiGroupDisaggregatedSet+"/") {
				continue
			}
			if _, set := expectedGiven.Annotations[k]; set {
				continue
			}
			if expected.Annotations == nil {
				expected.Annotations = map[string]string{}
			}
			expected.Annotations[k] = v
		}
	})
}

// deleteDisaggregatedSet removes the DisaggregatedSet of a service that does not run on
// the DisaggregatedSet backend. A missing object or CRD is not an error.
func (r *LLMISVCReconciler) deleteDisaggregatedSet(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	return Delete(ctx, r, llmSvc, &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      disaggregatedSetName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
		},
	})
}

// currentDisaggregatedSet returns the DisaggregatedSet of a service, or nil when it does
// not exist.
func (r *LLMISVCReconciler) currentDisaggregatedSet(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*disaggregatedsetv1.DisaggregatedSet, error) {
	current := &disaggregatedsetv1.DisaggregatedSet{}
	key := types.NamespacedName{Name: disaggregatedSetName(llmSvc), Namespace: llmSvc.GetNamespace()}
	if err := r.Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get DisaggregatedSet %s: %w", key, err)
	}
	return current, nil
}

// expectedDisaggregatedSet builds the DisaggregatedSet of a service. current is the
// DisaggregatedSet as it exists, or nil; its role templates are the deployed pod specs
// that keep storage-initializer settings stable across controller upgrades.
func (r *LLMISVCReconciler) expectedDisaggregatedSet(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, current *disaggregatedsetv1.DisaggregatedSet) (*disaggregatedsetv1.DisaggregatedSet, error) {
	// The workload revision is computed below from the rendered roles, not by
	// reconcileWorkloadRevision, which renders the Deployment and LeaderWorkerSet builders.
	renderConfig := *config
	renderConfig.WorkloadRevision = ""

	decode, err := r.expectedDisaggregatedDecodeRole(ctx, llmSvc, &renderConfig, deployedDisaggregatedRole(current, constants.LLMDRoleDecode))
	if err != nil {
		return nil, err
	}
	prefill, err := r.expectedDisaggregatedPrefillRole(ctx, llmSvc, &renderConfig, deployedDisaggregatedRole(current, constants.LLMDRolePrefill))
	if err != nil {
		return nil, err
	}

	if workloadRevisionEnabled(llmSvc) {
		revision, err := computeWorkloadRevision([]workloadRevisionRole{
			leaderWorkerRevisionRole(constants.LLMDRoleDecode, &decode.Template),
			leaderWorkerRevisionRole(constants.LLMDRolePrefill, &prefill.Template),
		})
		if err != nil {
			return nil, err
		}
		revisionConfig := &Config{WorkloadRevision: revision}
		applyLeaderWorkerSetWorkloadRevision(&decode.Template, revisionConfig)
		applyLeaderWorkerSetWorkloadRevision(&prefill.Template, revisionConfig)
	}

	decodeRollout, err := disaggregatedRollingUpdateConfig(&llmSvc.Spec.WorkloadSpec)
	if err != nil {
		return nil, fmt.Errorf("invalid rollout strategy for the %s role: %w", constants.LLMDRoleDecode, err)
	}
	prefillRollout, err := disaggregatedRollingUpdateConfig(llmSvc.Spec.Prefill)
	if err != nil {
		return nil, fmt.Errorf("invalid rollout strategy for the %s role: %w", constants.LLMDRolePrefill, err)
	}

	ds := &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      disaggregatedSetName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
				constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: disaggregatedsetv1.DisaggregatedSetSpec{
			Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
				disaggregatedRoleSpec(constants.LLMDRoleDecode, decode, &llmSvc.Spec.WorkloadSpec, decodeRollout),
				disaggregatedRoleSpec(constants.LLMDRolePrefill, prefill, llmSvc.Spec.Prefill, prefillRollout),
			},
		},
	}

	log.FromContext(ctx).V(2).Info("Expected DisaggregatedSet", "disaggregatedset", ds)

	return ds, nil
}

func (r *LLMISVCReconciler) expectedDisaggregatedDecodeRole(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	if llmSvc.Spec.Worker != nil {
		return r.disaggregatedMultiNodeDecodeTemplate(ctx, llmSvc, config, deployed)
	}
	return r.disaggregatedSingleNodeDecodeTemplate(ctx, llmSvc, config, deployed)
}

func (r *LLMISVCReconciler) expectedDisaggregatedPrefillRole(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	if llmSvc.Spec.Prefill.Worker != nil {
		return r.disaggregatedMultiNodePrefillTemplate(ctx, llmSvc, config, deployed)
	}
	return r.disaggregatedSingleNodePrefillTemplate(ctx, llmSvc, config, deployed)
}

// disaggregatedRollingUpdateConfig returns the rolling update settings of a role. A
// multi-node role keeps the LeaderWorkerSet defaults it would get as a LeaderWorkerSet.
// A single-node role would otherwise run as a Deployment, so the settings it leaves out
// take the Deployment defaults: the LeaderWorkerSet default maxSurge of 0 would make
// maxUnavailable: 0 invalid and remove a lone replica before its replacement is ready.
//
// The DisaggregatedSet copies these settings into the LeaderWorkerSet it creates for
// each revision, and resizes those LeaderWorkerSets one batch at a time during a
// rollout, down to a single replica, or as an autoscaler asks. The LeaderWorkerSet
// webhook resolves the settings against the replicas of each LeaderWorkerSet at every
// step, and rejects maxSurge and maxUnavailable that both resolve to zero, which stalls
// the rollout: maxSurge: 0 with the default maxUnavailable of 25% is rejected as soon
// as a LeaderWorkerSet has three replicas or fewer. So when maxSurge resolves to zero,
// maxUnavailable is resolved once against the role's replicas, rounding down the way a
// Deployment does, and written as a fixed number of at least one. An autoscaled role is
// resolved against its minimum replicas.
//
// A role scaled to zero is resolved as if it had one replica. The DisaggregatedSet
// creates its LeaderWorkerSets with these settings even at zero replicas, which the
// webhook accepts, and afterwards only changes their replicas, never their settings: a
// value that resolves to zero would be rejected as soon as the role scales up, leaving
// it at zero replicas.
func disaggregatedRollingUpdateConfig(workload *v1alpha2.WorkloadSpec) (*lwsapi.RollingUpdateConfiguration, error) {
	var config *lwsapi.RollingUpdateConfiguration
	if workload.Worker != nil {
		config = rollingUpdateConfigFromWorkloadSpec(workload)
		if config == nil {
			// The LeaderWorkerSet defaults, maxUnavailable: 1 and maxSurge: 0, are valid
			// at every size.
			return nil, nil
		}
	} else {
		config = &lwsapi.RollingUpdateConfiguration{
			MaxUnavailable: intstr.FromString("25%"),
			MaxSurge:       intstr.FromString("25%"),
		}
		if rs := workload.RolloutStrategy; rs != nil {
			if rs.MaxUnavailable != nil {
				config.MaxUnavailable = *rs.MaxUnavailable
			}
			if rs.MaxSurge != nil {
				config.MaxSurge = *rs.MaxSurge
			}
		}
	}

	replicas := max(int(disaggregatedRoleSizingReplicas(workload)), 1)
	surge, err := intstr.GetScaledValueFromIntOrPercent(&config.MaxSurge, replicas, true)
	if err != nil {
		return nil, fmt.Errorf("invalid maxSurge: %w", err)
	}
	unavailable, err := intstr.GetScaledValueFromIntOrPercent(&config.MaxUnavailable, replicas, false)
	if err != nil {
		return nil, fmt.Errorf("invalid maxUnavailable: %w", err)
	}
	// A maxSurge that resolves to more than zero is a positive number, or a positive
	// percentage that rounds up to at least one at every size, so it is valid as set.
	if surge == 0 && (config.MaxUnavailable.Type == intstr.String || unavailable == 0) {
		if unavailable < 0 || unavailable > math.MaxInt32 {
			return nil, fmt.Errorf("invalid maxUnavailable: %s resolves to %d", config.MaxUnavailable.String(), unavailable)
		}
		config.MaxUnavailable = intstr.FromInt32(max(int32(unavailable), 1))
	}
	return config, nil
}

// disaggregatedRoleSpec builds a role of the DisaggregatedSet. An autoscaled role is set
// to External scaling, taking its replicas from the DisaggregatedSetRoleScaler the
// DisaggregatedSet creates for it, and leaves its own replicas unset.
func disaggregatedRoleSpec(name string, role *disaggregatedRoleTemplate, workload *v1alpha2.WorkloadSpec, rollingUpdate *lwsapi.RollingUpdateConfiguration) disaggregatedsetv1.DisaggregatedRoleSpec {
	spec := disaggregatedsetv1.DisaggregatedRoleSpec{
		Name: name,
		LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      role.Labels,
				Annotations: role.Annotations,
			},
			Spec: lwsapi.LeaderWorkerSetSpec{
				Replicas:             ptr.To(disaggregatedRoleReplicas(workload.Replicas)),
				LeaderWorkerTemplate: role.Template,
				RolloutStrategy: lwsapi.RolloutStrategy{
					Type:                       lwsapi.RollingUpdateStrategyType,
					RollingUpdateConfiguration: rollingUpdate,
				},
				StartupPolicy: lwsapi.LeaderCreatedStartupPolicy,
			},
		},
	}
	if disaggregatedRoleAutoscaled(workload) {
		spec.Scaling = &disaggregatedsetv1.RoleScaling{Mode: disaggregatedsetv1.RoleScalingExternal}
		spec.Spec.Replicas = nil
	}
	return spec
}

// deployedDisaggregatedRole returns the pod specs a role currently runs, or empty specs
// when there is no DisaggregatedSet or no such role yet.
func deployedDisaggregatedRole(ds *disaggregatedsetv1.DisaggregatedSet, role string) deployedRoleSpecs {
	spec := findDisaggregatedRole(ds, role)
	if spec == nil {
		return deployedRoleSpecs{}
	}
	template := spec.Spec.LeaderWorkerTemplate
	deployed := deployedRoleSpecs{Worker: template.WorkerTemplate.Spec}
	if template.LeaderTemplate != nil {
		deployed.Leader = &template.LeaderTemplate.Spec
	}
	return deployed
}

func findDisaggregatedRole(ds *disaggregatedsetv1.DisaggregatedSet, role string) *disaggregatedsetv1.DisaggregatedRoleSpec {
	if ds == nil {
		return nil
	}
	for i := range ds.Spec.Roles {
		if ds.Spec.Roles[i].Name == role {
			return &ds.Spec.Roles[i]
		}
	}
	return nil
}

// semanticDisaggregatedSetIsEqual compares the fields KServe manages. Pod specs and role
// scaling are compared exactly so that removed fields are detected; the rest is compared
// with DeepDerivative so that values defaulted by the API server are ignored.
func semanticDisaggregatedSetIsEqual(expected *disaggregatedsetv1.DisaggregatedSet, curr *disaggregatedsetv1.DisaggregatedSet) bool {
	if len(expected.Spec.Roles) != len(curr.Spec.Roles) {
		return false
	}
	for i := range expected.Spec.Roles {
		want, got := expected.Spec.Roles[i], curr.Spec.Roles[i]
		if want.Name != got.Name ||
			!equality.Semantic.DeepEqual(want.Scaling, got.Scaling) ||
			!podSpecsEqual(&want.Spec.LeaderWorkerTemplate, &got.Spec.LeaderWorkerTemplate) {
			return false
		}
	}
	return equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func podSpecsEqual(expected, curr *lwsapi.LeaderWorkerTemplate) bool {
	if (expected.LeaderTemplate != nil) != (curr.LeaderTemplate != nil) {
		return false
	}
	if expected.LeaderTemplate != nil && !equality.Semantic.DeepEqual(expected.LeaderTemplate.Spec, curr.LeaderTemplate.Spec) {
		return false
	}
	return equality.Semantic.DeepEqual(expected.WorkerTemplate.Spec, curr.WorkerTemplate.Spec)
}

// disaggregatedRoleTargets returns the replicas each role of a DisaggregatedSet should
// run: the role's own replicas, or for a role an autoscaler scales, the replicas its
// DisaggregatedSetRoleScaler asks for. A role whose scaler the DisaggregatedSet has not
// created yet, or does not control, has no entry.
func (r *LLMISVCReconciler) disaggregatedRoleTargets(ctx context.Context, ds *disaggregatedsetv1.DisaggregatedSet) (map[string]int32, error) {
	if ds == nil {
		return nil, nil
	}
	targets := make(map[string]int32, len(ds.Spec.Roles))
	for _, role := range ds.Spec.Roles {
		if role.Scaling == nil || role.Scaling.Mode != disaggregatedsetv1.RoleScalingExternal {
			targets[role.Name] = ptr.Deref(role.Spec.Replicas, 1)
			continue
		}
		scaler := &disaggregatedsetv1.DisaggregatedSetRoleScaler{}
		key := types.NamespacedName{Name: dsctrl.ScalerName(ds.Name, role.Name), Namespace: ds.Namespace}
		if err := r.Get(ctx, key, scaler); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get DisaggregatedSetRoleScaler %s: %w", key, err)
		}
		if metav1.IsControlledBy(scaler, ds) {
			targets[role.Name] = scaler.Spec.Replicas
		}
	}
	return targets, nil
}

// propagateDisaggregatedSetStatus maps the readiness of each role onto the workload
// condition the role would have without a DisaggregatedSet. targets holds the replicas
// each role should run, from disaggregatedRoleTargets.
func propagateDisaggregatedSetStatus(llmSvc *v1alpha2.LLMInferenceService, ds *disaggregatedsetv1.DisaggregatedSet, targets map[string]int32) {
	for _, role := range disaggregatedSetRoles {
		ready, notReady := disaggregatedRoleConditionMarkers(llmSvc, role)
		var desired *int32
		if target, ok := targets[role]; ok {
			desired = &target
		}
		reason, message, isReady := disaggregatedRoleReadiness(ds, role, disaggregatedRoleIsMultiNode(llmSvc, role), desired)
		if isReady {
			ready()
		} else {
			notReady(reason, message)
		}
	}
}

func disaggregatedRoleConditionMarkers(llmSvc *v1alpha2.LLMInferenceService, role string) (func(), func(reason, messageFormat string, messageA ...interface{})) {
	if role == constants.LLMDRolePrefill {
		if llmSvc.Spec.Prefill.Worker != nil {
			return llmSvc.MarkPrefillWorkerWorkloadReady, llmSvc.MarkPrefillWorkerWorkloadNotReady
		}
		return llmSvc.MarkPrefillWorkloadReady, llmSvc.MarkPrefillWorkloadNotReady
	}
	if llmSvc.Spec.Worker != nil {
		return llmSvc.MarkWorkerWorkloadReady, llmSvc.MarkWorkerWorkloadNotReady
	}
	return llmSvc.MarkMainWorkloadReady, llmSvc.MarkMainWorkloadNotReady
}

// disaggregatedRoleIsMultiNode reports whether a role runs leader/worker groups.
func disaggregatedRoleIsMultiNode(llmSvc *v1alpha2.LLMInferenceService, role string) bool {
	if role == constants.LLMDRolePrefill {
		return llmSvc.Spec.Prefill.Worker != nil
	}
	return llmSvc.Spec.Worker != nil
}

// disaggregatedRoleReadiness reports whether a role is ready the way the workload it
// replaces would be.
//
// A single-node role replaces a Deployment, which is available while its ready
// replicas cover desired - maxUnavailable, so the service stays ready through rollouts
// and scaling. It counts ReadyReplicas, which spans every revision the way a
// Deployment's available replicas span its ReplicaSets, against the role's
// maxUnavailable resolved for its replicas and rounded down as a Deployment does.
//
// A multi-node role replaces a LeaderWorkerSet, which is available only once every
// desired replica is ready and updated. It requires exactly its desired replicas to
// exist, be ready and run the latest revision, the check the DisaggregatedSet
// controller uses for its own status; ReadyReplicas alone would let ready old replicas
// stand in for unready new ones.
//
// desiredReplicas is the replicas the role should run, which for an autoscaled role is
// what its DisaggregatedSetRoleScaler asks for rather than the role's own replicas. A nil
// value means that scaler does not exist yet, so the role has no target to be ready against.
func disaggregatedRoleReadiness(ds *disaggregatedsetv1.DisaggregatedSet, role string, multiNode bool, desiredReplicas *int32) (reason, message string, ready bool) {
	if ds == nil || ds.Status.ObservedGeneration < ds.Generation {
		return "Progressing", "DisaggregatedSet is progressing", false
	}
	spec := findDisaggregatedRole(ds, role)
	if spec == nil {
		return "Progressing", fmt.Sprintf("DisaggregatedSet has no %s role yet", role), false
	}
	if desiredReplicas == nil {
		return "Progressing", fmt.Sprintf("%s role is waiting for DisaggregatedSetRoleScaler %s", role, dsctrl.ScalerName(ds.Name, role)), false
	}
	desired := *desiredReplicas
	status := disaggregatedRoleStatus(ds, role)

	if multiNode {
		if status.Replicas == desired && status.ReadyReplicas == desired && status.UpdatedReplicas == desired {
			return "", "", true
		}
		message = fmt.Sprintf("%s role has %d/%d replicas ready, %d updated and %d in total", role, status.ReadyReplicas, desired, status.UpdatedReplicas, status.Replicas)
	} else {
		minReady := int(desired) - disaggregatedRoleMaxUnavailable(spec, desired)
		if int(status.ReadyReplicas) >= minReady {
			return "", "", true
		}
		message = fmt.Sprintf("%s role has %d ready replicas, needs at least %d of %d", role, status.ReadyReplicas, minReady, desired)
	}

	reason = "Progressing"
	if available := meta.FindStatusCondition(ds.Status.Conditions, string(disaggregatedsetv1.DisaggregatedSetAvailable)); available != nil &&
		available.Status == metav1.ConditionFalse && available.Reason != "" {
		reason = available.Reason
		if available.Message != "" {
			message = available.Message
		}
	}
	return reason, message, false
}

// disaggregatedRoleMaxUnavailable returns how many of a role's desired replicas may be
// unavailable, resolving its maxUnavailable for those replicas and rounding down. A role
// without a rolling update configuration, or with one that does not resolve, allows none.
func disaggregatedRoleMaxUnavailable(spec *disaggregatedsetv1.DisaggregatedRoleSpec, desired int32) int {
	config := spec.Spec.RolloutStrategy.RollingUpdateConfiguration
	if config == nil {
		return 0
	}
	maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(&config.MaxUnavailable, int(desired), false)
	if err != nil {
		return 0
	}
	return maxUnavailable
}

func observedDisaggregatedSet(name string) *v1alpha2.ObservedWorkloadStatus {
	return &v1alpha2.ObservedWorkloadStatus{
		TypedLocalObjectReference: corev1.TypedLocalObjectReference{
			APIGroup: ptr.To(apiGroupDisaggregatedSet), Kind: "DisaggregatedSet", Name: name,
		},
	}
}

// observeDisaggregatedSetReplicas sets the ready replicas of the decode (primary) and
// prefill workloads from the role statuses of the DisaggregatedSet.
func (r *LLMISVCReconciler) observeDisaggregatedSetReplicas(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, primary, prefill *v1alpha2.ObservedWorkloadStatus) error {
	ds, err := r.currentDisaggregatedSet(ctx, llmSvc)
	if err != nil {
		return err
	}
	primary.ReadyReplicas = ptr.To(disaggregatedRoleStatus(ds, constants.LLMDRoleDecode).ReadyReplicas)
	prefill.ReadyReplicas = ptr.To(disaggregatedRoleStatus(ds, constants.LLMDRolePrefill).ReadyReplicas)
	return nil
}

// disaggregatedRoleStatus returns the status of a role, or an empty status when there
// is no DisaggregatedSet or the role has none yet.
func disaggregatedRoleStatus(ds *disaggregatedsetv1.DisaggregatedSet, role string) disaggregatedsetv1.RoleStatus {
	if ds != nil {
		for _, s := range ds.Status.RoleStatuses {
			if s.Name == role {
				return s
			}
		}
	}
	return disaggregatedsetv1.RoleStatus{}
}

// useDisaggregatedSetWorkload reports whether the DisaggregatedSet should exist: the
// service runs on the backend and is not stopped.
func useDisaggregatedSetWorkload(llmSvc *v1alpha2.LLMInferenceService, decision disaggregatedSetDecision) bool {
	return decision.Use && !utils.GetForceStopRuntime(llmSvc)
}
