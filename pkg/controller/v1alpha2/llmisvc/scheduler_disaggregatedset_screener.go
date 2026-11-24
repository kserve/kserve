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
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	// disaggregatedSetRolloutScreenerType is the endpoint picker plugin that keeps each
	// request's prefill and decode on one DisaggregatedSet revision.
	disaggregatedSetRolloutScreenerType = "disaggregatedset-rollout-screener"

	reasonScreenerMissing             = "ScreenerMissing"
	reasonExperimentalPluginsDisabled = "ExperimentalPluginsDisabled"
	reasonScreenerScopeMismatch       = "ScreenerScopeMismatch"
	reasonConfigNotInspectable        = "ConfigNotInspectable"
)

// screenerCheck is the outcome of checking a scheduler config for the rollout screener.
// An empty reason means the screener runs and covers the service's pods.
type screenerCheck struct {
	reason  string
	message string
}

// checkDisaggregatedSetRolloutScreener records in the DisaggregatedSetRolloutScreened
// condition whether the scheduler of a service on a DisaggregatedSet runs the
// disaggregatedset-rollout-screener, and emits a Warning event when it does not.
//
// A DisaggregatedSet rolls its prefill and decode roles together, and a release controller
// can hold it part-way with canaryPercent, so two revisions serve at once. Without the
// screener the endpoint picker pairs a prefill pod of one revision with a decode pod of
// the other. The check never blocks the scheduler: requests are still served.
func (r *LLMISVCReconciler) checkDisaggregatedSetRolloutScreener(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, scheduler *appsv1.Deployment) error {
	if !schedulerScreensDisaggregatedSet(llmSvc) {
		llmSvc.MarkDisaggregatedSetRolloutScreenedUnset()
		return nil
	}

	ds, err := r.currentDisaggregatedSet(ctx, llmSvc)
	if err != nil {
		return err
	}
	check := disaggregatedSetRolloutScreenerCheck(scheduler, disaggregatedSetPodLabels(ds))
	switch check.reason {
	case "":
		llmSvc.MarkDisaggregatedSetRolloutScreened()
	case reasonConfigNotInspectable:
		llmSvc.MarkDisaggregatedSetRolloutScreenedUnknown(check.reason, "%s", check.message)
	default:
		// Once per reason, not on every reconcile.
		if prev := llmSvc.Status.GetCondition(v1alpha2.DisaggregatedSetRolloutScreened); prev == nil || !prev.IsFalse() || prev.Reason != check.reason {
			r.Eventf(llmSvc, corev1.EventTypeWarning, check.reason, "%s", check.message)
		}
		llmSvc.MarkDisaggregatedSetRolloutNotScreened(check.reason, "%s", check.message)
	}
	return nil
}

// schedulerScreensDisaggregatedSet reports whether the check applies: the service runs on
// a DisaggregatedSet, and KServe runs its scheduler.
func schedulerScreensDisaggregatedSet(llmSvc *v1alpha2.LLMInferenceService) bool {
	used := llmSvc.Status.GetCondition(v1alpha2.DisaggregatedSetUsed)
	return used != nil && used.IsTrue() &&
		!utils.GetForceStopRuntime(llmSvc) &&
		llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Template != nil &&
		!llmSvc.Spec.Router.Scheduler.Pool.HasRef()
}

// disaggregatedSetPodLabels returns the pod labels of each role of the DisaggregatedSet.
func disaggregatedSetPodLabels(ds *disaggregatedsetv1.DisaggregatedSet) map[string]labels.Set {
	if ds == nil {
		return nil
	}
	podLabels := make(map[string]labels.Set, len(ds.Spec.Roles))
	for _, role := range ds.Spec.Roles {
		podLabels[role.Name] = labels.Set(role.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels)
	}
	return podLabels
}

// disaggregatedSetRolloutScreenerCheck checks the scheduler Deployment's inline config for
// the screener, the flag the endpoint picker needs to load it, and whether the screener's
// scope selects the pods of every role in rolePodLabels.
func disaggregatedSetRolloutScreenerCheck(d *appsv1.Deployment, rolePodLabels map[string]labels.Set) screenerCheck {
	main := utils.GetContainerWithName(&d.Spec.Template.Spec, "main")
	if main == nil || !hasWritableConfigText(d) {
		return screenerCheck{
			reasonConfigNotInspectable,
			"the scheduler config is not inline (--config-text), so KServe cannot check it for the " + disaggregatedSetRolloutScreenerType,
		}
	}

	screeners := schedulerConfigPlugins(main, disaggregatedSetRolloutScreenerType)
	if len(screeners) == 0 {
		return screenerCheck{reasonScreenerMissing, fmt.Sprintf(
			"the scheduler config has no %s: while the DisaggregatedSet rolls or holds a canary, a request can be prefilled on one revision and decoded on another; "+
				"add the plugin to the scheduler config, with --allow-experimental-plugins", disaggregatedSetRolloutScreenerType)}
	}
	if !experimentalPluginsAllowed(main.Args) {
		return screenerCheck{reasonExperimentalPluginsDisabled, fmt.Sprintf(
			"the scheduler config has the alpha plugin %s, which the endpoint picker refuses to load without --allow-experimental-plugins; add the flag to the scheduler's main container",
			disaggregatedSetRolloutScreenerType)}
	}

	for _, screener := range screeners {
		selectorText, _, _ := unstructured.NestedString(screener, "parameters", "scope", "labelSelector")
		selector, err := labels.Parse(selectorText)
		if err != nil {
			return screenerCheck{reasonScreenerScopeMismatch, fmt.Sprintf(
				"the %s scope.labelSelector %q does not parse: %v", disaggregatedSetRolloutScreenerType, selectorText, err)}
		}
		var unselected []string
		for role, podLabels := range rolePodLabels {
			if !selector.Matches(podLabels) {
				unselected = append(unselected, role)
			}
		}
		if len(unselected) > 0 {
			slices.Sort(unselected)
			return screenerCheck{reasonScreenerScopeMismatch, fmt.Sprintf(
				"the %s scope.labelSelector %q does not select the pods of the %s role(s), so it cannot gate their revisions",
				disaggregatedSetRolloutScreenerType, selectorText, strings.Join(unselected, ", "))}
		}
	}
	return screenerCheck{}
}

// schedulerConfigPlugins returns the plugins of a type declared in the container's inline
// scheduler config. Request-control plugins such as screeners run when declared.
func schedulerConfigPlugins(c *corev1.Container, pluginType string) []map[string]interface{} {
	var found []map[string]interface{}
	for i := range len(c.Args) - 1 {
		if _, ok := inlineConfigTextFlags[c.Args[i]]; !ok {
			continue
		}
		u := unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(c.Args[i+1]), &u.Object); err != nil {
			continue
		}
		plugins, _, _ := unstructured.NestedSlice(u.Object, "plugins")
		for _, plugin := range plugins {
			if pm, ok := plugin.(map[string]interface{}); ok && pm["type"] == pluginType {
				found = append(found, pm)
			}
		}
	}
	return found
}

// experimentalPluginsAllowed reports whether the endpoint picker arguments turn on
// --allow-experimental-plugins (optionally with =true). The last occurrence wins.
func experimentalPluginsAllowed(args []string) bool {
	allowed := false
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name != "allow-experimental-plugins" {
			continue
		}
		if !hasValue {
			allowed = true
			continue
		}
		enabled, err := strconv.ParseBool(value)
		allowed = err == nil && enabled
	}
	return allowed
}
