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
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// Scheduler generations (spec.router.scheduler.rolloutStrategy.type Generations).
//
// A generation is one scheduler Deployment, named after a hash of its pod template, so any
// change to the scheduler's template, config or presets makes a new one, and reverting the
// change gives back the name of the generation that ran it. Every generation belongs to
// the same InferencePool. The EPP Service, which the InferencePool's endpointPickerRef
// names, selects the pods of one generation by its label: moving the Service moves the
// gateway's ext_proc streams to that generation, while the other keeps running.
//
// The Service is also where the state lives: its selector holds the generation that
// serves, and an annotation the generations that served before it, most recent first, so
// the controller keeps the ones to roll back to.
const (
	// SchedulerGenerationLabelKey labels a generation's Deployment and pods with the
	// generation's name.
	SchedulerGenerationLabelKey = "serving.kserve.io/scheduler-generation"
	// schedulerGenerationHistoryAnnotationKey on the EPP Service lists the generations
	// that served before the one it selects, most recent first, comma-separated.
	schedulerGenerationHistoryAnnotationKey = "serving.kserve.io/scheduler-generation-history"
	// maxSchedulerGenerationHistory bounds the history: retainedGenerations is at most 10.
	maxSchedulerGenerationHistory = 10
	haLeaseNameFlag               = "--ha-lease-name"
)

// schedulerUsesGenerations reports whether KServe runs the scheduler as generations.
func schedulerUsesGenerations(llmSvc *v1alpha2.LLMInferenceService) bool {
	return hasManagedScheduler(llmSvc) &&
		!utils.GetForceStopRuntime(llmSvc) &&
		llmSvc.Spec.Router.Scheduler.RolloutStrategy.IsGenerations()
}

func schedulerGenerationsSpec(llmSvc *v1alpha2.LLMInferenceService) v1alpha2.SchedulerGenerationsSpec {
	if g := llmSvc.Spec.Router.Scheduler.RolloutStrategy.Generations; g != nil {
		return *g
	}
	return v1alpha2.SchedulerGenerationsSpec{}
}

func schedulerGenerationDeploymentName(llmSvc *v1alpha2.LLMInferenceService, generation string) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-router-scheduler-"+generation)
}

// schedulerGenerationLeaseName is the leader-election Lease of one generation. The endpoint
// picker derives its Lease from the pool by default, so without it two leader-elected
// generations of one pool would elect one leader between them and the other would have
// no ready pod.
func schedulerGenerationLeaseName(llmSvc *v1alpha2.LLMInferenceService, generation string) string {
	return kmeta.ChildName(llmSvc.GetName(), "-epp-"+generation) + ".llm-d.ai"
}

// schedulerGenerationHash names the generation of a pod template.
func schedulerGenerationHash(template *corev1.PodTemplateSpec) (string, error) {
	raw, err := json.Marshal(template)
	if err != nil {
		return "", fmt.Errorf("failed to hash the scheduler pod template: %w", err)
	}
	h := fnv.New32a()
	_, _ = h.Write(raw)
	return rand.SafeEncodeString(strconv.FormatUint(uint64(h.Sum32()), 10)), nil
}

// asSchedulerGeneration turns the expected scheduler Deployment into the Deployment of the
// generation: its name, the generation label on it, its selector and its pods, and its own
// Lease under leader election. Everything that is part of the hash is set before.
func asSchedulerGeneration(llmSvc *v1alpha2.LLMInferenceService, d *appsv1.Deployment, generation string, spec v1alpha2.SchedulerGenerationsSpec) {
	d.Name = schedulerGenerationDeploymentName(llmSvc, generation)
	d.Labels = maps.Clone(d.Labels)
	d.Labels[SchedulerGenerationLabelKey] = generation
	d.Spec.Selector = d.Spec.Selector.DeepCopy()
	d.Spec.Selector.MatchLabels[SchedulerGenerationLabelKey] = generation
	if d.Spec.Template.Labels == nil {
		d.Spec.Template.Labels = map[string]string{}
	}
	d.Spec.Template.Labels[SchedulerGenerationLabelKey] = generation
	d.Spec.MinReadySeconds = spec.MinReadySeconds
	// The template of a generation never changes, only its replicas do; Recreate would
	// stop a serving generation when it is scaled.
	d.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType}

	for i := range d.Spec.Template.Spec.Containers {
		c := &d.Spec.Template.Spec.Containers[i]
		if c.Name != "main" {
			continue
		}
		if slices.ContainsFunc(c.Args, schedulerArgEnablesLeaderElection) &&
			!slices.ContainsFunc(c.Args, isHALeaseNameArg) {
			c.Args = append(c.Args, haLeaseNameFlag+"="+schedulerGenerationLeaseName(llmSvc, generation))
		}
	}
}

// schedulerArgEnablesLeaderElection reports whether an endpoint picker argument turns leader
// election on (--ha-enable-leader-election, optionally with =true).
func schedulerArgEnablesLeaderElection(arg string) bool {
	name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
	if name != "ha-enable-leader-election" || !strings.HasPrefix(arg, "-") {
		return false
	}
	if !hasValue {
		return true
	}
	enabled, err := strconv.ParseBool(value)
	return err != nil || enabled
}

func isHALeaseNameArg(arg string) bool {
	name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
	return name == "ha-lease-name"
}

// schedulerGenerationAvailable reports whether a generation can take the gateway's traffic:
// every replica available for minReadySeconds, or with leader election its leader, since
// standbys never pass the readiness probe.
func schedulerGenerationAvailable(d *appsv1.Deployment) bool {
	if d.Status.ObservedGeneration < d.Generation {
		return false
	}
	if deploymentRunsLeaderElection(d) {
		return d.Status.AvailableReplicas >= 1
	}
	want := ptr.Deref(d.Spec.Replicas, 1)
	return d.Status.UpdatedReplicas >= want && d.Status.AvailableReplicas >= want
}

func deploymentRunsLeaderElection(d *appsv1.Deployment) bool {
	for _, c := range d.Spec.Template.Spec.Containers {
		if c.Name == "main" && slices.ContainsFunc(c.Args, schedulerArgEnablesLeaderElection) {
			return true
		}
	}
	return false
}

// schedulerGenerationChoice is what decides the generation the EPP Service selects.
type schedulerGenerationChoice struct {
	// current is the generation of the current spec; currentAvailable whether it is.
	current          string
	currentAvailable bool
	// serving is the generation the Service selects; "" when it selects the scheduler
	// Deployment of the other strategies (legacyRunning: that Deployment exists).
	serving       string
	legacyRunning bool
	// running are the generations that have a Deployment.
	running sets.Set[string]
	spec    v1alpha2.SchedulerGenerationsSpec
}

// target returns the generation the Service should select ("" for the scheduler
// Deployment of the other strategies) and, when generations.serving names a generation
// that does not run, false.
func (c schedulerGenerationChoice) target() (string, bool) {
	servingRuns := (c.serving != "" && c.running.Has(c.serving)) || (c.serving == "" && c.legacyRunning)
	stay := func() string {
		if servingRuns {
			return c.serving
		}
		// Nothing serves that could be kept: start with the current generation.
		return c.current
	}

	if pinned := c.spec.Serving; pinned != "" {
		if c.running.Has(pinned) {
			return pinned, true
		}
		return stay(), false
	}
	if !servingRuns || c.serving == c.current {
		return c.current, true
	}
	if c.spec.Promotion == v1alpha2.SchedulerGenerationPromotionManual {
		return c.serving, true
	}
	if c.currentAvailable {
		return c.current, true
	}
	return c.serving, true
}

// nextSchedulerGenerationHistory returns the generations that served before target, most
// recent first, after the Service moves from serving to target.
func nextSchedulerGenerationHistory(history []string, serving, target string) []string {
	next := make([]string, 0, len(history)+1)
	if serving != "" && serving != target {
		next = append(next, serving)
	}
	for _, g := range history {
		if g != target && !slices.Contains(next, g) {
			next = append(next, g)
		}
	}
	if len(next) > maxSchedulerGenerationHistory {
		next = next[:maxSchedulerGenerationHistory]
	}
	return next
}

// retainedSchedulerGenerations returns the generations to keep running: the current one,
// the one that serves, a pinned one, and the retainedGenerations most recent ones of the
// history that still run.
func retainedSchedulerGenerations(c schedulerGenerationChoice, target string, history []string) sets.Set[string] {
	keep := sets.New(c.current)
	if target != "" {
		keep.Insert(target)
	}
	if c.spec.Serving != "" {
		keep.Insert(c.spec.Serving)
	}
	retained := ptr.Deref(c.spec.RetainedGenerations, 1)
	for _, g := range history {
		if retained <= 0 {
			break
		}
		if keep.Has(g) || !c.running.Has(g) {
			continue
		}
		keep.Insert(g)
		retained--
	}
	return keep
}

func parseSchedulerGenerationHistory(svc *corev1.Service) []string {
	var history []string
	for _, g := range strings.Split(svc.GetAnnotations()[schedulerGenerationHistoryAnnotationKey], ",") {
		if g = strings.TrimSpace(g); g != "" {
			history = append(history, g)
		}
	}
	return history
}

// listSchedulerGenerations returns the generation Deployments of the service by name.
func (r *LLMISVCReconciler) listSchedulerGenerations(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (map[string]*appsv1.Deployment, error) {
	hasGeneration, err := labels.NewRequirement(SchedulerGenerationLabelKey, selection.Exists, nil)
	if err != nil {
		return nil, err
	}
	selector := labels.SelectorFromSet(SchedulerLabels(llmSvc)).Add(*hasGeneration)
	list := &appsv1.DeploymentList{}
	if err := r.List(ctx, list, client.InNamespace(llmSvc.GetNamespace()), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("failed to list scheduler generations: %w", err)
	}
	generations := make(map[string]*appsv1.Deployment, len(list.Items))
	for i := range list.Items {
		d := &list.Items[i]
		if !metav1.IsControlledBy(d, llmSvc) || !d.GetDeletionTimestamp().IsZero() {
			continue
		}
		generations[d.Labels[SchedulerGenerationLabelKey]] = d
	}
	return generations, nil
}

// reconcileSchedulerGenerations runs the scheduler as generations: it creates the
// generation of the current spec next to the one that serves, points the EPP Service at
// the generation that should serve, and then deletes the generations it no longer keeps.
func (r *LLMISVCReconciler) reconcileSchedulerGenerations(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	spec := schedulerGenerationsSpec(llmSvc)

	generations, err := r.listSchedulerGenerations(ctx, llmSvc)
	if err != nil {
		return err
	}

	svcKey := types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: llmSvc.Spec.Router.EPPServiceName(llmSvc)}
	currSvc := &corev1.Service{}
	var serving string
	var history []string
	if err := r.Get(ctx, svcKey, currSvc); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get scheduler service %s: %w", svcKey, err)
		}
	} else {
		serving = currSvc.Spec.Selector[SchedulerGenerationLabelKey]
		history = parseSchedulerGenerationHistory(currSvc)
	}

	legacy := &appsv1.Deployment{}
	legacyKey := types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: schedulerDeploymentName(llmSvc)}
	legacyRunning := false
	if err := r.Get(ctx, legacyKey, legacy); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get scheduler deployment %s: %w", legacyKey, err)
		}
	} else {
		legacyRunning = metav1.IsControlledBy(legacy, llmSvc) && legacy.GetDeletionTimestamp().IsZero()
	}

	// What carries over from a running scheduler is read from the one that serves.
	from := &appsv1.Deployment{}
	switch {
	case serving != "" && generations[serving] != nil:
		from = generations[serving]
	case legacyRunning:
		from = legacy
	}
	expected, err := r.expectedSchedulerDeploymentFrom(ctx, llmSvc, from)
	if err != nil {
		return fmt.Errorf("failed to build expected scheduler deployment: %w", err)
	}
	current, err := schedulerGenerationHash(&expected.Spec.Template)
	if err != nil {
		return err
	}
	asSchedulerGeneration(llmSvc, expected, current, spec)
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, expected, semanticDeploymentIsEqual, PreserveDeploymentReplicas(), PreserveDeploymentSelector()); err != nil {
		return fmt.Errorf("failed to reconcile scheduler generation %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	// Read back for its status; a Deployment just created may not be in the cache yet.
	currentDeployment := &appsv1.Deployment{}
	if err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, client.ObjectKeyFromObject(expected), currentDeployment)
	}); err != nil {
		return fmt.Errorf("failed to get scheduler generation %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	generations[current] = currentDeployment

	choice := schedulerGenerationChoice{
		current:          current,
		currentAvailable: schedulerGenerationAvailable(currentDeployment),
		serving:          serving,
		legacyRunning:    legacyRunning,
		running:          sets.KeySet(generations),
		spec:             spec,
	}
	target, pinnedRuns := choice.target()
	if !pinnedRuns {
		r.Eventf(llmSvc, corev1.EventTypeWarning, "SchedulerGenerationNotFound",
			"rolloutStrategy.generations.serving names scheduler generation %q, which does not run; the EPP Service stays on %q", spec.Serving, target)
	}

	// A generation that serves while the spec has moved on keeps the scheduler's
	// replicas and minReadySeconds; its template stays the one it was started with.
	if d := generations[target]; target != current && d != nil {
		if err := r.scaleSchedulerGeneration(ctx, llmSvc, d, expected); err != nil {
			return err
		}
	}

	nextHistory := nextSchedulerGenerationHistory(history, serving, target)
	svc := r.expectedSchedulerService(ctx, llmSvc)
	if target != "" {
		svc.Spec.Selector[SchedulerGenerationLabelKey] = target
	}
	if len(nextHistory) > 0 {
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations[schedulerGenerationHistoryAnnotationKey] = strings.Join(nextHistory, ",")
	}
	if err := Reconcile(ctx, r, llmSvc, &corev1.Service{}, svc, semanticSchedulerServiceIsEqual); err != nil {
		return err
	}
	if target != serving {
		logger.Info("EPP Service moved to another scheduler generation", "from", serving, "to", target, "current", current)
		r.Eventf(llmSvc, corev1.EventTypeNormal, "SchedulerGenerationServing",
			"EPP Service %s selects scheduler generation %q (was %q)", svc.GetName(), target, serving)
	}

	// The Service has moved: delete what is no longer kept.
	keep := retainedSchedulerGenerations(choice, target, nextHistory)
	for name, d := range generations {
		if keep.Has(name) {
			continue
		}
		if err := Delete(ctx, r, llmSvc, d); err != nil {
			return err
		}
	}
	if legacyRunning && target != "" {
		if err := Delete(ctx, r, llmSvc, legacy); err != nil {
			return err
		}
	}

	servingDeployment := legacy
	if target != "" {
		servingDeployment = generations[target]
	}
	if err := r.propagateComponentDeploymentStatus(ctx, servingDeployment, llmSvc.MarkSchedulerWorkloadReady, llmSvc.MarkSchedulerWorkloadNotReady); err != nil {
		return err
	}
	setObservedSchedulerGenerations(llmSvc, servingDeployment.GetName(), generations, keep, current, target)
	return nil
}

// scaleSchedulerGeneration sets the replicas and minReadySeconds of a generation other
// than the current one, which is reconciled in full.
func (r *LLMISVCReconciler) scaleSchedulerGeneration(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, d, expected *appsv1.Deployment) error {
	if (expected.Spec.Replicas == nil || ptr.Equal(d.Spec.Replicas, expected.Spec.Replicas)) && d.Spec.MinReadySeconds == expected.Spec.MinReadySeconds {
		return nil
	}
	patch := client.MergeFrom(d.DeepCopy())
	if expected.Spec.Replicas != nil {
		d.Spec.Replicas = ptr.To(*expected.Spec.Replicas)
	}
	d.Spec.MinReadySeconds = expected.Spec.MinReadySeconds
	if err := r.Patch(ctx, d, patch); err != nil {
		return fmt.Errorf("failed to scale scheduler generation %s/%s: %w", d.GetNamespace(), d.GetName(), err)
	}
	r.Eventf(llmSvc, corev1.EventTypeNormal, "Updated", "Updated Deployment %s/%s", d.GetNamespace(), d.GetName())
	return nil
}

// setObservedSchedulerGenerations records the generations in status.workloads, where
// observeWorkloadStatus, which runs after the router is reconciled, carries them over.
func setObservedSchedulerGenerations(llmSvc *v1alpha2.LLMInferenceService, servingDeployment string, generations map[string]*appsv1.Deployment, keep sets.Set[string], current, target string) {
	observed := make([]v1alpha2.ObservedSchedulerGeneration, 0, keep.Len())
	for name, d := range generations {
		if !keep.Has(name) {
			continue
		}
		observed = append(observed, v1alpha2.ObservedSchedulerGeneration{
			Name:              name,
			Deployment:        d.GetName(),
			Serving:           name == target,
			Current:           name == current,
			ReadyReplicas:     d.Status.AvailableReplicas,
			CreationTimestamp: d.GetCreationTimestamp(),
		})
	}
	slices.SortFunc(observed, func(a, b v1alpha2.ObservedSchedulerGeneration) int {
		if c := b.CreationTimestamp.Compare(a.CreationTimestamp.Time); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	if llmSvc.Status.Workloads == nil {
		llmSvc.Status.Workloads = &v1alpha2.WorkloadStatus{}
	}
	llmSvc.Status.Workloads.Scheduler = observedDeployment(servingDeployment)
	llmSvc.Status.Workloads.SchedulerGenerations = observed
}

// deleteSchedulerGenerations deletes the generations once they are not used: when the
// scheduler is stopped or not managed, or, after a switch from type Generations to another
// strategy, once the scheduler Deployment the Service selects again is available.
func (r *LLMISVCReconciler) deleteSchedulerGenerations(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	generations, err := r.listSchedulerGenerations(ctx, llmSvc)
	if err != nil || len(generations) == 0 {
		return err
	}
	if hasManagedScheduler(llmSvc) && !utils.GetForceStopRuntime(llmSvc) {
		d := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: schedulerDeploymentName(llmSvc)}, d); err != nil {
			return client.IgnoreNotFound(err)
		}
		if d.Status.AvailableReplicas < 1 {
			log.FromContext(ctx).Info("Keeping the scheduler generations until the scheduler Deployment is available",
				"deployment", d.GetName(), "generations", len(generations))
			return nil
		}
	}
	for _, d := range generations {
		if err := Delete(ctx, r, llmSvc, d); err != nil {
			return err
		}
	}
	return nil
}
