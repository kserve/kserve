/*
Copyright 2018 The Knative Authors

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

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/kmp"
	"github.com/knative/serving/pkg/apis/autoscaling"
	"github.com/knative/serving/pkg/apis/serving"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (pa *PodAutoscaler) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(pa.GetObjectMeta()).ViaField("metadata")
	errs = errs.Also(pa.validateMetric())
	errs = errs.Also(pa.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
	if apis.IsInUpdate(ctx) {
		original := apis.GetBaseline(ctx).(*PodAutoscaler)
		errs = errs.Also(pa.checkImmutableFields(ctx, original))
	}
	return errs
}

func (current *PodAutoscaler) checkImmutableFields(ctx context.Context, original *PodAutoscaler) *apis.FieldError {
	if diff, err := compareSpec(original, current); err != nil {
		return &apis.FieldError{
			Message: "Failed to diff PodAutoscaler",
			Paths:   []string{"spec"},
			Details: err.Error(),
		}
	} else if diff != "" {
		return &apis.FieldError{
			Message: "Immutable fields changed (-old +new)",
			Paths:   []string{"spec"},
			Details: diff,
		}
	}
	// Verify the PA class does not change.
	// For backward compatibility, we allow a new class where there was none before.
	if oldClass, newClass, annotationChanged := classAnnotationChanged(original, current); annotationChanged {
		return &apis.FieldError{
			Message: fmt.Sprintf("Immutable class annotation changed (-%q +%q)", oldClass, newClass),
			Paths:   []string{"annotations[autoscaling.knative.dev/class]"},
		}
	}
	return nil
}

// Validate validates PodAutoscaler Spec.
func (rs *PodAutoscalerSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(rs, &PodAutoscalerSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}
	errs := validateReference(rs.ScaleTargetRef).ViaField("scaleTargetRef")
	if rs.ServiceName == "" {
		errs = errs.Also(apis.ErrMissingField("serviceName"))
	}
	errs = errs.Also(rs.ContainerConcurrency.Validate(ctx).
		ViaField("containerConcurrency"))
	return errs.Also(validateSKSFields(ctx, rs))
}

func validateSKSFields(ctx context.Context, rs *PodAutoscalerSpec) *apis.FieldError {
	var all *apis.FieldError
	// TODO(vagababov) stop permitting empty protocol type, once SKS controller is live.
	if string(rs.ProtocolType) != "" {
		all = all.Also(rs.ProtocolType.Validate(ctx)).ViaField("protocolType")
	}
	return all
}

func validateReference(ref autoscalingv1.CrossVersionObjectReference) *apis.FieldError {
	if equality.Semantic.DeepEqual(ref, autoscalingv1.CrossVersionObjectReference{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}
	var errs *apis.FieldError
	if ref.Kind == "" {
		errs = errs.Also(apis.ErrMissingField("kind"))
	}
	if ref.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	}
	if ref.APIVersion == "" {
		errs = errs.Also(apis.ErrMissingField("apiVersion"))
	}
	return errs
}

func (pa *PodAutoscaler) validateMetric() *apis.FieldError {
	if metric, ok := pa.Annotations[autoscaling.MetricAnnotationKey]; ok {
		switch pa.Class() {
		case autoscaling.KPA:
			switch metric {
			case autoscaling.Concurrency:
				return nil
			}
		case autoscaling.HPA:
			switch metric {
			case autoscaling.CPU:
				return nil
			}
			// TODO: implement OPS autoscaling.
		default:
			// Leave other classes of PodAutoscaler alone.
			return nil
		}
		return &apis.FieldError{
			Message: fmt.Sprintf("Unsupported metric %q for PodAutoscaler class %q",
				metric, pa.Class()),
			Paths: []string{"annotations[autoscaling.knative.dev/metric]"},
		}
	}
	return nil
}

func compareSpec(original *PodAutoscaler, current *PodAutoscaler) (string, error) {
	// TODO(vagababov): remove after 0.6. This is temporary plug for backwards compatibility.
	opt := cmp.FilterPath(
		func(p cmp.Path) bool {
			return p.String() == "ProtocolType"
		},
		cmp.Ignore(),
	)

	return kmp.ShortDiff(original.Spec, current.Spec, opt)
}

func classAnnotationChanged(original *PodAutoscaler, current *PodAutoscaler) (string, string, bool) {
	oldClass, ok := original.Annotations[autoscaling.ClassAnnotationKey]
	if !ok {
		return "", "", false
	}
	newClass, ok := current.Annotations[autoscaling.ClassAnnotationKey]
	if ok && oldClass == newClass {
		return "", "", false
	}
	return oldClass, newClass, true
}
