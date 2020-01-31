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
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/config"
	routeconfig "knative.dev/serving/pkg/reconciler/route/config"
)

var (
	allowedAnnotations = map[string]struct{}{
		UpdaterAnnotation:                {},
		CreatorAnnotation:                {},
		RevisionLastPinnedAnnotationKey:  {},
		GroupNamePrefix + "forceUpgrade": {},
	}
)

// ValidateObjectMetadata validates that `metadata` stanza of the
// resources is correct.
func ValidateObjectMetadata(meta metav1.Object) *apis.FieldError {
	return apis.ValidateObjectMetadata(meta).
		Also(autoscaling.ValidateAnnotations(meta.GetAnnotations()).
			Also(validateKnativeAnnotations(meta.GetAnnotations())).
			ViaField("annotations"))
}

func validateKnativeAnnotations(annotations map[string]string) (errs *apis.FieldError) {
	for key := range annotations {
		if _, ok := allowedAnnotations[key]; !ok && strings.HasPrefix(key, GroupNamePrefix) {
			errs = errs.Also(apis.ErrInvalidKeyName(key, apis.CurrentField))
		}
	}
	return
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

// ValidateContainerConcurrency function validates the ContainerConcurrency field
// TODO(#5007): Move this to autoscaling.
func ValidateContainerConcurrency(containerConcurrency *int64) *apis.FieldError {
	if containerConcurrency != nil {
		if *containerConcurrency < 0 || *containerConcurrency > config.DefaultMaxRevisionContainerConcurrency {
			return apis.ErrOutOfBoundsValue(
				*containerConcurrency, 0, config.DefaultMaxRevisionContainerConcurrency, apis.CurrentField)
		}
	}
	return nil
}

// ValidateClusterVisibilityLabel function validates the visibility label on a Route
func ValidateClusterVisibilityLabel(label string) (errs *apis.FieldError) {
	if label != routeconfig.VisibilityClusterLocal {
		errs = apis.ErrInvalidValue(label, routeconfig.VisibilityLabelKey)
	}
	return
}

// SetUserInfo sets creator and updater annotations
func SetUserInfo(ctx context.Context, oldSpec, newSpec, resource interface{}) {
	if ui := apis.GetUserInfo(ctx); ui != nil {
		objectMetaAccessor, ok := resource.(metav1.ObjectMetaAccessor)
		if !ok {
			return
		}
		ans := objectMetaAccessor.GetObjectMeta().GetAnnotations()
		if ans == nil {
			ans = map[string]string{}
			objectMetaAccessor.GetObjectMeta().SetAnnotations(ans)
		}

		if apis.IsInUpdate(ctx) {
			if equality.Semantic.DeepEqual(oldSpec, newSpec) {
				return
			}
			ans[UpdaterAnnotation] = ui.Username
		} else {
			ans[CreatorAnnotation] = ui.Username
			ans[UpdaterAnnotation] = ui.Username
		}
	}
}

// ValidateRevisionName validates name and generateName for the revisionTemplate
func ValidateRevisionName(ctx context.Context, name, generateName string) *apis.FieldError {
	if generateName != "" {
		if msgs := validation.NameIsDNS1035Label(generateName, true); len(msgs) > 0 {
			return apis.ErrInvalidValue(
				fmt.Sprintf("not a DNS 1035 label prefix: %v", msgs),
				"metadata.generateName")
		}
	}
	if name != "" {
		if msgs := validation.NameIsDNS1035Label(name, false); len(msgs) > 0 {
			return apis.ErrInvalidValue(
				fmt.Sprintf("not a DNS 1035 label: %v", msgs),
				"metadata.name")
		}
		om := apis.ParentMeta(ctx)
		prefix := om.Name + "-"
		if om.Name != "" {
			// Even if there is GenerateName, allow the use
			// of Name post-creation.
		} else if om.GenerateName != "" {
			// We disallow bringing your own name when the parent
			// resource uses generateName (at creation).
			return apis.ErrDisallowedFields("metadata.name")
		}

		if !strings.HasPrefix(name, prefix) {
			return apis.ErrInvalidValue(
				fmt.Sprintf("%q must have prefix %q", name, prefix),
				"metadata.name")
		}
	}
	return nil
}
