/*
Copyright 2021 The KServe Authors.

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

package knative

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmp"
	knserving "knative.dev/serving/pkg/apis/serving"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	knutils "github.com/kserve/kserve/pkg/controller/v1alpha1/utils"
	"github.com/kserve/kserve/pkg/utils"
)

var log = logf.Log.WithName("KsvcReconciler")

var managedKsvcAnnotations = map[string]bool{
	constants.RollOutDurationAnnotationKey: true,
	// Required for the integration of Openshift Serverless with Openshift Service Mesh
	constants.KnativeOpenshiftEnablePassthroughKey: true,
}

type KsvcReconciler struct {
	client          client.Client
	scheme          *runtime.Scheme
	Service         *knservingv1.Service
	componentExt    *v1beta1.ComponentExtensionSpec
	componentStatus v1beta1.ComponentStatusSpec
}

func NewKsvcReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec,
	disallowedLabelList []string,
) *KsvcReconciler {
	return &KsvcReconciler{
		client:          client,
		scheme:          scheme,
		Service:         createKnativeService(componentMeta, componentExt, podSpec, componentStatus, disallowedLabelList),
		componentExt:    componentExt,
		componentStatus: componentStatus,
	}
}

func createKnativeService(
	componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec,
	disallowedLabelList []string,
) *knservingv1.Service {
	annotations := componentMeta.GetAnnotations()

	knutils.SetAutoScalingAnnotations(
		annotations,
		componentExtension.ScaleTarget,
		(*string)(componentExtension.ScaleMetric),
		componentExtension.MinReplicas,
		componentExtension.MaxReplicas,
		log,
	)

	// ksvc metadata.annotations
	// rollout-duration must be put under metadata.annotations
	ksvcAnnotations := make(map[string]string)
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		if value, ok := annotations[ksvcAnnotationKey]; ok {
			ksvcAnnotations[ksvcAnnotationKey] = value
			delete(annotations, ksvcAnnotationKey)
		}
	}

	lastRolledoutRevision := componentStatus.LatestRolledoutRevision

	// Log component status and canary traffic percent
	log.Info("revision status:", "LatestRolledoutRevision", componentStatus.LatestRolledoutRevision,
		"LatestReadyRevision", componentStatus.LatestReadyRevision,
		"LatestCreatedRevision", componentStatus.LatestCreatedRevision,
		"PreviousRolledoutRevision", componentStatus.PreviousRolledoutRevision,
		"CanaryTrafficPercent", componentExtension.CanaryTrafficPercent)

	trafficTargets := []knservingv1.TrafficTarget{}
	// Split traffic when canary traffic percent is specified
	if componentExtension.CanaryTrafficPercent != nil && lastRolledoutRevision != "" {
		latestTarget := knservingv1.TrafficTarget{
			LatestRevision: proto.Bool(true),
			Percent:        proto.Int64(*componentExtension.CanaryTrafficPercent),
		}
		if value, ok := annotations[constants.EnableRoutingTagAnnotationKey]; ok && value == "true" {
			latestTarget.Tag = "latest"
		}
		trafficTargets = append(trafficTargets, latestTarget)

		if *componentExtension.CanaryTrafficPercent < 100 {
			remainingTraffic := 100 - *componentExtension.CanaryTrafficPercent
			canaryTarget := knservingv1.TrafficTarget{
				RevisionName:   lastRolledoutRevision,
				LatestRevision: proto.Bool(false),
				Percent:        proto.Int64(remainingTraffic),
				Tag:            "prev",
			}
			trafficTargets = append(trafficTargets, canaryTarget)
		}
	} else {
		// blue-green rollout
		latestTarget := knservingv1.TrafficTarget{
			LatestRevision: proto.Bool(true),
			Percent:        proto.Int64(100),
		}
		if value, ok := annotations[constants.EnableRoutingTagAnnotationKey]; ok && value == "true" {
			latestTarget.Tag = "latest"
		}
		trafficTargets = append(trafficTargets, latestTarget)
	}

	labels := utils.Filter(componentMeta.Labels, func(key string) bool {
		return !utils.Includes(disallowedLabelList, key)
	})

	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: ksvcAnnotations,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      labels,
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						// If timeoutSeconds is not set by isvc(componentExtension.TimeoutSeconds is nil), Knative
						// Serving will set timeoutSeconds to the default value.
						TimeoutSeconds: componentExtension.TimeoutSeconds,
						// If timeoutSeconds is set by isvc, set ResponseStartTimeoutSeconds to the same value.
						// If timeoutSeconds is not set by isvc, set ResponseStartTimeoutSeconds to empty.
						ResponseStartTimeoutSeconds: componentExtension.TimeoutSeconds,
						ContainerConcurrency:        componentExtension.ContainerConcurrency,
						PodSpec:                     *podSpec,
					},
				},
			},
			RouteSpec: knservingv1.RouteSpec{
				Traffic: trafficTargets,
			},
		},
	}
	return service
}

func reconcileKsvc(desired *knservingv1.Service, existing *knservingv1.Service) error {
	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return nil
	}

	// Reconcile differences and update
	// knative mutator defaults the enableServiceLinks to false which would generate a diff despite no changes on desired knative service
	// https://github.com/knative/serving/blob/main/pkg/apis/serving/v1/revision_defaults.go#L134
	if desired.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks == nil &&
		existing.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks != nil &&
		!*existing.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks {
		desired.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks = proto.Bool(false)
	}
	diff, err := kmp.SafeDiff(desired.Spec.ConfigurationSpec, existing.Spec.ConfigurationSpec)
	if err != nil {
		return errors.Wrapf(err, "failed to diff knative service configuration spec")
	}
	log.Info("knative service configuration diff (-desired, +observed):", "diff", diff)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.Spec.Traffic = desired.Spec.Traffic
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		if desiredValue, ok := desired.ObjectMeta.Annotations[ksvcAnnotationKey]; ok {
			existing.ObjectMeta.Annotations[ksvcAnnotationKey] = desiredValue
		} else {
			delete(existing.ObjectMeta.Annotations, ksvcAnnotationKey)
		}
	}
	return nil
}

func (r *KsvcReconciler) Reconcile(ctx context.Context) (*knservingv1.ServiceStatus, error) {
	desired := r.Service
	existing := &knservingv1.Service{}

	forceStopRuntime := false
	if val, exist := desired.Spec.Template.Annotations[constants.StopAnnotationKey]; exist {
		forceStopRuntime = strings.EqualFold(val, "true")
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		log.Info("Updating knative service", "namespace", desired.Namespace, "name", desired.Name)
		if err := r.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing); err != nil {
			return err
		}

		if forceStopRuntime {
			log.Info("Deleting knative service", "namespace", existing.Namespace, "name", existing.Name)
			if existing.GetDeletionTimestamp() == nil { // check if the ksvc was already deleted
				err := r.client.Delete(ctx, existing)
				if err != nil {
					return err
				}
			}
			return nil
		}

		// Set ResourceVersion which is required for update operation.
		desired.ResourceVersion = existing.ResourceVersion
		// Add immutable annotations to avoid validation error during dry-run update.
		desired.Annotations[knserving.CreatorAnnotation] = existing.Annotations[knserving.CreatorAnnotation]
		desired.Annotations[knserving.UpdaterAnnotation] = existing.Annotations[knserving.UpdaterAnnotation]

		// Do a dry-run update to avoid diffs generated by default values introduced by knative's defaulter webhook.
		// This will populate our local knative service object with any default values
		// that are present on the remote version.
		if err := r.client.Update(ctx, desired, client.DryRunAll); err != nil {
			// log only if it is not resource conflict error to avoid spamming
			if !apierr.IsConflict(err) {
				log.Error(err, "Failed to perform dry-run update of knative service", "service", desired.Name)
			}
			return err
		}
		if err := reconcileKsvc(desired, existing); err != nil {
			return err
		}
		return r.client.Update(ctx, existing)
	})
	if err != nil {
		// Create service if it does not exist
		if apierr.IsNotFound(err) {
			if !forceStopRuntime {
				log.Info("Creating knative service", "namespace", desired.Namespace, "name", desired.Name)
				return &desired.Status, r.client.Create(ctx, desired)
			}

			return &desired.Status, nil
		}
		return &existing.Status, errors.Wrapf(err, "fails to reconcile knative service")
	}
	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1.Service) bool {
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		existingValue, ok1 := service.ObjectMeta.Annotations[ksvcAnnotationKey]
		desiredValue, ok2 := desiredService.ObjectMeta.Annotations[ksvcAnnotationKey]
		if ok1 != ok2 || existingValue != desiredValue {
			return false
		}
	}
	return equality.Semantic.DeepEqual(desiredService.Spec.ConfigurationSpec, service.Spec.ConfigurationSpec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.Spec.RouteSpec, service.Spec.RouteSpec)
}
