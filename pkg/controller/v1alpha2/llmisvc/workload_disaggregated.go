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

	// disaggregatedSetNotUsedReason is the event reason emitted when a service asks for
	// the DisaggregatedSet backend but keeps its current workloads.
	disaggregatedSetNotUsedReason = "DisaggregatedSetNotUsed"

	// disaggregatedSetMigratingToReason and disaggregatedSetMigratingFromReason are the
	// event reasons emitted when a running service moves onto or off a DisaggregatedSet.
	// Both replace the workloads without waiting for the new pods to become ready.
	disaggregatedSetMigratingToReason   = "MigratingToDisaggregatedSet"
	disaggregatedSetMigratingFromReason = "MigratingFromDisaggregatedSet"
)

// Reasons for a False DisaggregatedSetUsed condition. They are part of the API.
const (
	reasonFeatureGateDisabled     = "FeatureGateDisabled"
	reasonCRDNotInstalled         = "CRDNotInstalled"
	reasonNoPrefillWorkload       = "NoPrefillWorkload"
	reasonAutoscalingNotSupported = "AutoscalingNotSupported"
	reasonReplicasMismatch        = "ReplicasMismatch"
)

// disaggregatedSetRoles lists the roles of a DisaggregatedSet in the order they appear
// in its spec.
var disaggregatedSetRoles = []string{constants.LLMDRoleDecode, constants.LLMDRolePrefill}

// disaggregatedSetDecision reports whether a service runs on the DisaggregatedSet
// backend. The zero value means the service did not ask for it. Explicit says the
// request came from the service's own spec rather than from its presets. Reason and
// Message say why a service that asked for it cannot use it.
type disaggregatedSetDecision struct {
	Requested bool
	Explicit  bool
	Use       bool
	Reason    string
	Message   string
}

// decideDisaggregatedSet decides whether a service runs on the DisaggregatedSet
// backend. It takes the spec after base configurations are merged, which is why these
// checks live here rather than in admission. explicit reports whether the service asked
// for the backend in its own spec, before the presets that turn it on by default were
// merged in.
func (r *LLMISVCReconciler) decideDisaggregatedSet(llmSvc *v1alpha2.LLMInferenceService, config *Config, explicit bool) disaggregatedSetDecision {
	if !llmSvc.DisaggregatedSetRequested() {
		return disaggregatedSetDecision{}
	}

	notUsed := func(reason, message string) disaggregatedSetDecision {
		return disaggregatedSetDecision{Requested: true, Explicit: explicit, Reason: reason, Message: message}
	}
	switch {
	case !config.FeatureGates.DisaggregatedSet:
		return notUsed(reasonFeatureGateDisabled, "the DisaggregatedSet feature gate is disabled")
	case !r.DisaggregatedSetAvailable:
		return notUsed(reasonCRDNotInstalled, "the DisaggregatedSet CRD is not installed")
	case llmSvc.Spec.Prefill == nil:
		return notUsed(reasonNoPrefillWorkload, "the service has no prefill workload")
	case llmSvc.Spec.Scaling != nil || llmSvc.Spec.Prefill.Scaling != nil:
		return notUsed(reasonAutoscalingNotSupported, "autoscaling is not supported with a DisaggregatedSet yet")
	case (disaggregatedRoleReplicas(llmSvc.Spec.Replicas) == 0) != (disaggregatedRoleReplicas(llmSvc.Spec.Prefill.Replicas) == 0):
		return notUsed(reasonReplicasMismatch, "a DisaggregatedSet requires decode and prefill replicas to be both zero or both non-zero")
	}
	return disaggregatedSetDecision{Requested: true, Explicit: explicit, Use: true}
}

// markDisaggregatedSetDecision records the decision in the DisaggregatedSetUsed
// condition and warns when the service changes backend or cannot get the one it asked
// for. It decides from the decision and the workloads that exist, never from the
// service's previous status, which can be stale or lost.
//
// A running service that moves onto or off a DisaggregatedSet gets a migration warning
// on the reconcile that replaces its workloads: the switch deletes the old workloads
// without waiting for the new pods, so the service is unavailable until they are ready.
// Moving off carries the reason, so it is the only warning for that change. Stopping a
// service deletes its workloads too, but is not a migration.
//
// Otherwise, a service that asked for the backend in its own spec gets a warning on
// every reconcile while it falls back; the event recorder folds the repeats into one
// event with a count. A service that only takes the default from its presets falls back
// quietly: it did nothing to act on, and with the feature gate off by default every P/D
// service would otherwise warn.
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

	switch {
	case running && r.hasDisaggregatedSet(ctx, llmSvc):
		why := decision.Message
		if !decision.Requested {
			why = constants.LLMDisaggregatedSetAnnotationKey + " is no longer \"true\""
		}
		r.Eventf(llmSvc, corev1.EventTypeWarning, disaggregatedSetMigratingFromReason,
			"moving the prefill and decode workloads from DisaggregatedSet %s to Deployments or LeaderWorkerSets because %s; "+
				"the DisaggregatedSet is deleted without waiting for the new pods, so the service is unavailable until they are ready",
			disaggregatedSetName(llmSvc), why)
	case decision.Requested && decision.Explicit:
		r.Eventf(llmSvc, corev1.EventTypeWarning, disaggregatedSetNotUsedReason,
			"%s is set but the service keeps its current workloads: %s", constants.LLMDisaggregatedSetAnnotationKey, decision.Message)
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

func disaggregatedSetName(llmSvc *v1alpha2.LLMInferenceService) string {
	return boundedChildName(llmSvc.GetName(), disaggregatedSetNameSuffix, disaggregatedSetNameMaxLength)
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
		err = Update(ctx, r, llmSvc, current, expected, semanticDisaggregatedSetIsEqual)
	}
	if err != nil {
		return nil, err
	}
	// Create, Update and the dry-run Update that Update makes when nothing changed all
	// fill expected from the API server response, status and generation included.
	return expected, nil
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
				disaggregatedRoleSpec(constants.LLMDRoleDecode, decode, llmSvc.Spec.Replicas, decodeRollout),
				disaggregatedRoleSpec(constants.LLMDRolePrefill, prefill, llmSvc.Spec.Prefill.Replicas, prefillRollout),
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
// A single-node role also resolves the values the way the Deployment controller does
// (ResolveFenceposts): when both round down to zero for the role's replicas, for
// example maxSurge: 0 with the default maxUnavailable of 25% on three replicas or
// fewer, it removes one replica at a time. A Deployment accepts such values, but a
// DisaggregatedSet rejects them. A multi-node role keeps them, as a LeaderWorkerSet
// rejects them too.
func disaggregatedRollingUpdateConfig(workload *v1alpha2.WorkloadSpec) (*lwsapi.RollingUpdateConfiguration, error) {
	if workload.Worker != nil {
		return rollingUpdateConfigFromWorkloadSpec(workload), nil
	}
	config := &lwsapi.RollingUpdateConfiguration{
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

	// A DisaggregatedSet does not check a role scaled to zero.
	replicas := int(disaggregatedRoleReplicas(workload.Replicas))
	if replicas == 0 {
		return config, nil
	}
	surge, err := intstr.GetScaledValueFromIntOrPercent(&config.MaxSurge, replicas, true)
	if err != nil {
		return nil, fmt.Errorf("invalid maxSurge: %w", err)
	}
	unavailable, err := intstr.GetScaledValueFromIntOrPercent(&config.MaxUnavailable, replicas, false)
	if err != nil {
		return nil, fmt.Errorf("invalid maxUnavailable: %w", err)
	}
	if surge == 0 && unavailable == 0 {
		config.MaxUnavailable = intstr.FromInt32(1)
	}
	return config, nil
}

func disaggregatedRoleSpec(name string, role *disaggregatedRoleTemplate, replicas *int32, rollingUpdate *lwsapi.RollingUpdateConfiguration) disaggregatedsetv1.DisaggregatedRoleSpec {
	return disaggregatedsetv1.DisaggregatedRoleSpec{
		Name: name,
		LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      role.Labels,
				Annotations: role.Annotations,
			},
			Spec: lwsapi.LeaderWorkerSetSpec{
				Replicas:             ptr.To(disaggregatedRoleReplicas(replicas)),
				LeaderWorkerTemplate: role.Template,
				RolloutStrategy: lwsapi.RolloutStrategy{
					Type:                       lwsapi.RollingUpdateStrategyType,
					RollingUpdateConfiguration: rollingUpdate,
				},
				StartupPolicy: lwsapi.LeaderCreatedStartupPolicy,
			},
		},
	}
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

// semanticDisaggregatedSetIsEqual compares the fields KServe manages. Pod specs are
// compared exactly so that removed fields are detected; the rest is compared with
// DeepDerivative so that values defaulted by the API server are ignored.
func semanticDisaggregatedSetIsEqual(expected *disaggregatedsetv1.DisaggregatedSet, curr *disaggregatedsetv1.DisaggregatedSet) bool {
	if len(expected.Spec.Roles) != len(curr.Spec.Roles) {
		return false
	}
	for i := range expected.Spec.Roles {
		want, got := expected.Spec.Roles[i], curr.Spec.Roles[i]
		if want.Name != got.Name || !podSpecsEqual(&want.Spec.LeaderWorkerTemplate, &got.Spec.LeaderWorkerTemplate) {
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

// propagateDisaggregatedSetStatus maps the readiness of each role onto the workload
// condition the role would have without a DisaggregatedSet.
func propagateDisaggregatedSetStatus(llmSvc *v1alpha2.LLMInferenceService, ds *disaggregatedsetv1.DisaggregatedSet) {
	for _, role := range disaggregatedSetRoles {
		ready, notReady := disaggregatedRoleConditionMarkers(llmSvc, role)
		reason, message, isReady := disaggregatedRoleReadiness(ds, role)
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

// disaggregatedRoleReadiness reports whether a role has finished rolling out: exactly
// its desired replicas exist, are ready and run the latest revision of the
// DisaggregatedSet. The status counts replicas of every revision in Replicas and
// ReadyReplicas, so ready old replicas could otherwise stand in for unready new ones.
// This matches the check the DisaggregatedSet controller uses for its own status.
func disaggregatedRoleReadiness(ds *disaggregatedsetv1.DisaggregatedSet, role string) (reason, message string, ready bool) {
	if ds == nil || ds.Status.ObservedGeneration < ds.Generation {
		return "Progressing", "DisaggregatedSet is progressing", false
	}
	spec := findDisaggregatedRole(ds, role)
	if spec == nil {
		return "Progressing", fmt.Sprintf("DisaggregatedSet has no %s role yet", role), false
	}
	desired := ptr.Deref(spec.Spec.Replicas, 1)

	status := disaggregatedRoleStatus(ds, role)
	if status.Replicas == desired && status.ReadyReplicas == desired && status.UpdatedReplicas == desired {
		return "", "", true
	}

	reason, message = "Progressing", fmt.Sprintf("%s role has %d/%d replicas ready, %d updated and %d in total", role, status.ReadyReplicas, desired, status.UpdatedReplicas, status.Replicas)
	if available := meta.FindStatusCondition(ds.Status.Conditions, string(disaggregatedsetv1.DisaggregatedSetAvailable)); available != nil &&
		available.Status == metav1.ConditionFalse && available.Reason != "" {
		reason = available.Reason
		if available.Message != "" {
			message = available.Message
		}
	}
	return reason, message, false
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
