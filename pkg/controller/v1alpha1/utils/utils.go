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
func SetAutoScalingAnnotations(ctx context.Context,
	clientset kubernetes.Interface,
	annotations map[string]string,
	scaleTarget *int32,
	scaleMetric *string,
	minReplicas *int32,
	maxReplicas int32,
	log logr.Logger,
) error {
	// User can pass down scaling class annotation to overwrite the default scaling KPA
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
	var revisionMinScale int32
	if minReplicas == nil {
		annotations[autoscaling.MinScaleAnnotationKey] = strconv.Itoa(int(constants.DefaultMinReplicas))
		revisionMinScale = constants.DefaultMinReplicas
	} else {
		annotations[autoscaling.MinScaleAnnotationKey] = strconv.Itoa(int(*minReplicas))
		revisionMinScale = *minReplicas
	}

	annotations[autoscaling.MaxScaleAnnotationKey] = strconv.Itoa(int(maxReplicas))

	log.Info("kserve will always set the lower and upper scale bounds for knative revisions equal to the min and max replicas requested",
		"min-scale", annotations[autoscaling.MinScaleAnnotationKey],
		"max-scale", annotations[autoscaling.MaxScaleAnnotationKey],
	)

	annotations[autoscaling.InitialScaleAnnotationKey] = annotations[autoscaling.MinScaleAnnotationKey]

	log.Info("kserve will always set the initial scale value for knative revisions equal to the min replicas requested",
		"initial-scale", annotations[autoscaling.InitialScaleAnnotationKey],
	)

	// Retrieve the allow-zero-initial-scale value from the knative autoscaler configuration.
	allowZeroInitialScale, err := CheckZeroInitialScaleAllowed(ctx, clientset)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve the knative autoscaler configuration")
	}

	// If knative is configured to not allow zero initial scale when 0 min replicas are requested,
	// set the initial scale to 1 for the created knative revision. This represents the closest supported value
	// to the requested min replicas. After initialization, the knative revision will be scaled to 0.
	if !allowZeroInitialScale && revisionMinScale == 0 {
		log.Info("The current knative autoscaler global configuration does not allow zero initial scale. The knative revision will be initialized with 1 replica then scaled down to 0",
			"allow-zero-initial-scale", allowZeroInitialScale,
			"initial-scale", 1)
		annotations[autoscaling.InitialScaleAnnotationKey] = "1"
	}

	return nil
}

// CheckZeroInitialScaleAllowed reads the global knative autoscaler configuration defined in the autoscaler
// configmap and returns a boolean value based on if knative is configured to allow zero initial scale.
// This configmap will always be defined with the name 'config-autoscaler'.
// The namespace the configmap exists within may vary. If the configmap is created in a namespace other than
// 'knative-serving' this value must be set using the CONFIG_AUTOSCALER_NAMESPACE environmental variable.
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
