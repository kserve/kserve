/*
Copyright 2019 The Knative Authors

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

package serving

import (
	"context"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/config"
)

// ValidateObjectMetadata validates that `metadata` stanza of the
// resources is correct.
func ValidateObjectMetadata(meta metav1.Object) *apis.FieldError {
	return apis.ValidateObjectMetadata(meta).
		Also(autoscaling.ValidateAnnotations(meta.GetAnnotations()).ViaField("annotations"))
}

// ValidateQueueSidecarAnnotation validates QueueSideCarResourcePercentageAnnotation
func ValidateQueueSidecarAnnotation(annotations map[string]string) *apis.FieldError {
	if len(annotations) == 0 {
		return nil
	}
	v, ok := annotations[QueueSideCarResourcePercentageAnnotation]
	if !ok {
		return nil
	}
	value, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return apis.ErrInvalidValue(v, apis.CurrentField).ViaKey(QueueSideCarResourcePercentageAnnotation)
	}
	if value <= 0.1 || value > 100 {
		return apis.ErrOutOfBoundsValue(value, 0.1, 100.0, QueueSideCarResourcePercentageAnnotation)
	}
	return nil
}

// ValidateTimeoutSeconds validates timeout by comparing MaxRevisionTimeoutSeconds
func ValidateTimeoutSeconds(ctx context.Context, timeoutSeconds int64) *apis.FieldError {
	if timeoutSeconds != 0 {
		cfg := config.FromContextOrDefaults(ctx)
		if timeoutSeconds > cfg.Defaults.MaxRevisionTimeoutSeconds || timeoutSeconds < 0 {
			return apis.ErrOutOfBoundsValue(timeoutSeconds, 0,
				cfg.Defaults.MaxRevisionTimeoutSeconds,
				"timeoutSeconds")
		}
	}
	return nil
}
