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

package utils

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"knative.dev/serving/pkg/apis/autoscaling"

	"github.com/kserve/kserve/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
)

// CheckNodeAffinity returns true if the node matches the node affinity specified in the PV Spec
func CheckNodeAffinity(pvSpec *corev1.PersistentVolumeSpec, node corev1.Node) (bool, error) {
	if pvSpec.NodeAffinity == nil || pvSpec.NodeAffinity.Required == nil {
		return false, nil
	}

	terms := pvSpec.NodeAffinity.Required
	return corev1helpers.MatchNodeSelectorTerms(&node, terms)
}

// SetAutoScalingAnnotations validates the requested autoscaling configuration against the
// globally configured knative autoscaler configuration, then sets the resolved autoscaling annotations.
func SetAutoScalingAnnotations(
	annotations map[string]string,
	scaleTarget *int32,
	scaleMetric *string,
	minReplicas *int32,
	maxReplicas int32,
	log logr.Logger,
) {
	// User can pass down scaling class annotation to overwrite the default scaling KPA.
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	if scaleTarget != nil {
		annotations[autoscaling.TargetAnnotationKey] = strconv.Itoa(int(*scaleTarget))
	}

	if scaleMetric != nil {
		annotations[autoscaling.MetricAnnotationKey] = *scaleMetric
	}

	// If a min replicas value is not set, use the default min replicas value.
	if minReplicas == nil {
		annotations[autoscaling.MinScaleAnnotationKey] = strconv.Itoa(int(constants.DefaultMinReplicas))
	} else {
		annotations[autoscaling.MinScaleAnnotationKey] = strconv.Itoa(int(*minReplicas))
	}

	// Set the max scale equivalent to the max replicas unless max replicas is set to be unlimited, i.e. zero.
	// If unlimited max replicas is set, then we will allow scaling up to the globally configured knative max scale value.
	if maxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = strconv.Itoa(int(maxReplicas))
	}
}

// CheckZeroInitialScaleAllowed reads the global knative autoscaler configuration defined in the autoscaler
// configmap and returns a boolean value based on if knative is configured to allow zero initial scale.
// This configmap will always be defined with the name 'config-autoscaler'.
// The namespace the configmap exists within may vary. If the configmap is created in a namespace other than
// 'knative-serving' this value must be set using the KNATIVE_CONFIG_AUTOSCALER_NAMESPACE environmental variable.
func CheckZeroInitialScaleAllowed(ctx context.Context, clientset kubernetes.Interface) (bool, error) {
	// Set allow-zero-initial-scale to the default values to start.
	// If autoscaling values are not set in the configuration, then the defaults are used.
	allowZeroInitialScale := "false"

	configAutoscaler, err := clientset.CoreV1().ConfigMaps(constants.AutoscalerConfigmapNamespace).Get(ctx, constants.AutoscalerConfigmapName, metav1.GetOptions{})
	if err != nil {
		return false, errors.Wrapf(
			err,
			"fails to retrieve the %s configmap from the %s namespace.",
			constants.AutoscalerConfigmapName,
			constants.AutoscalerConfigmapNamespace,
		)
	}

	if configuredAllowZeroInitialScale, ok := configAutoscaler.Data[constants.AutoscalerAllowZeroScaleKey]; ok {
		allowZeroInitialScale = configuredAllowZeroInitialScale
	}

	return strconv.ParseBool(allowZeroInitialScale)
}

// ValidateInitialScaleAnnotation checks the annotations of a resource for the knative initial scale annotation.
// When the annotation is set validation is performed. If any of this validation fails, the annotation will
// be removed and the default initial scale behavior will be used.
func ValidateInitialScaleAnnotation(annotations map[string]string, allowZeroInitialScale bool, log logr.Logger) {
	// Check that the annoation is set.
	_, set := annotations[autoscaling.InitialScaleAnnotationKey]
	if !set {
		return
	}

	// If the initial scale annotation is set to an invalid non-integer then proceed with default initial scale behavior.
	initialScale, err := strconv.Atoi(annotations[autoscaling.InitialScaleAnnotationKey])
	if err != nil {
		log.Info(
			fmt.Sprintf(
				"Invalid value '%s' set for %s annotation, must be an integer. "+
					"This annotation will be ignored and the default initial scale behavior will be used.",
				annotations[autoscaling.InitialScaleAnnotationKey],
				autoscaling.InitialScaleAnnotationKey,
			),
		)
		delete(annotations, autoscaling.InitialScaleAnnotationKey)
		return
	}

	// If the initial scale annotation is set to zero when zero initial scale is not allowed then proceed with default initial scale behavior
	if !allowZeroInitialScale && initialScale == 0 {
		log.Info(
			fmt.Sprintf(
				"The %s annotation is explicitly set to 0 when the current knative autoscaler global configuration does not allow zero initial scale. "+
					"This annotation will be ignored and the default initial scale behavior will be used",
				autoscaling.InitialScaleAnnotationKey,
			),
		)
		delete(annotations, autoscaling.InitialScaleAnnotationKey)
		return
	}

	log.Info(
		fmt.Sprintf(
			"The %s annotation is explicitly set to an allowed integer. This value will override the default initial scale behavior",
			autoscaling.InitialScaleAnnotationKey,
		),
		"initial-scale", annotations[autoscaling.InitialScaleAnnotationKey],
	)
}
