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

package autoscaling

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"knative.dev/pkg/apis"
)

func getIntGE0(m map[string]string, k string) (int64, *apis.FieldError) {
	v, ok := m[k]
	if !ok {
		return 0, nil
	}
	i, err := strconv.ParseInt(v, 10, 32)
	if err != nil || i < 0 {
		return 0, apis.ErrOutOfBoundsValue(v, 1, math.MaxInt32, k)
	}
	return i, nil
}

func ValidateAnnotations(anns map[string]string) *apis.FieldError {
	if len(anns) == 0 {
		return nil
	}
	return validateMinMaxScale(anns).Also(validateFloats(anns)).Also(validateWindows(anns).Also(validateMetric(anns)))
}

func validateFloats(annotations map[string]string) *apis.FieldError {
	var errs *apis.FieldError
	if v, ok := annotations[PanicWindowPercentageAnnotationKey]; ok {
		if fv, err := strconv.ParseFloat(v, 64); err != nil {
			errs = errs.Also(apis.ErrInvalidValue(v, PanicWindowPercentageAnnotationKey))
		} else if fv < PanicWindowPercentageMin || fv > PanicWindowPercentageMax {
			errs = apis.ErrOutOfBoundsValue(v, PanicWindowPercentageMin,
				PanicWindowPercentageMax, PanicWindowPercentageAnnotationKey)
		}
	}
	if v, ok := annotations[PanicThresholdPercentageAnnotationKey]; ok {
		if fv, err := strconv.ParseFloat(v, 64); err != nil {
			errs = errs.Also(apis.ErrInvalidValue(v, PanicThresholdPercentageAnnotationKey))
		} else if fv < PanicThresholdPercentageMin || fv > PanicThresholdPercentageMax {
			errs = errs.Also(apis.ErrOutOfBoundsValue(v, PanicThresholdPercentageMin, PanicThresholdPercentageMax,
				PanicThresholdPercentageAnnotationKey))
		}
	}

	if v, ok := annotations[TargetAnnotationKey]; ok {
		if fv, err := strconv.ParseFloat(v, 64); err != nil || fv < TargetMin {
			errs = errs.Also(apis.ErrInvalidValue(v, TargetAnnotationKey))
		}
	}

	if v, ok := annotations[TargetUtilizationPercentageKey]; ok {
		if fv, err := strconv.ParseFloat(v, 64); err != nil {
			errs = errs.Also(apis.ErrInvalidValue(v, TargetUtilizationPercentageKey))
		} else if fv < 1 || fv > 100 {
			errs = errs.Also(apis.ErrOutOfBoundsValue(v, 1, 100, TargetUtilizationPercentageKey))
		}
	}

	if v, ok := annotations[TargetBurstCapacityKey]; ok {
		if fv, err := strconv.ParseFloat(v, 64); err != nil || fv < 0 && fv != -1 {
			errs = errs.Also(apis.ErrInvalidValue(v, TargetBurstCapacityKey))
		}
	}
	return errs
}

func validateWindows(annotations map[string]string) *apis.FieldError {
	var errs *apis.FieldError
	if v, ok := annotations[WindowAnnotationKey]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = apis.ErrInvalidValue(v, WindowAnnotationKey)
		} else if d < WindowMin || d > WindowMax {
			errs = apis.ErrOutOfBoundsValue(v, WindowMin, WindowMax, WindowAnnotationKey)
		}
	}
	return errs
}

func validateMinMaxScale(annotations map[string]string) *apis.FieldError {
	var errs *apis.FieldError

	min, err := getIntGE0(annotations, MinScaleAnnotationKey)
	errs = errs.Also(err)

	max, err := getIntGE0(annotations, MaxScaleAnnotationKey)
	errs = errs.Also(err)

	if max != 0 && max < min {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("maxScale=%d is less than minScale=%d", max, min),
			Paths:   []string{MaxScaleAnnotationKey, MinScaleAnnotationKey},
		})
	}
	return errs
}

func validateMetric(annotations map[string]string) *apis.FieldError {
	if metric, ok := annotations[MetricAnnotationKey]; ok {
		classValue := KPA
		if c, ok := annotations[ClassAnnotationKey]; ok {
			classValue = c
		}
		switch classValue {
		case KPA:
			switch metric {
			case Concurrency, RPS:
				return nil
			}
		case HPA:
			switch metric {
			case CPU, Concurrency, RPS:
				return nil
			}
		default:
			// Leave other classes of PodAutoscaler alone.
			return nil
		}
		return apis.ErrInvalidValue(metric, MetricAnnotationKey)
	}
	return nil
}
