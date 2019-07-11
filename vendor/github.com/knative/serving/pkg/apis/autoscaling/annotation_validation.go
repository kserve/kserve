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
	"strconv"

	"github.com/knative/pkg/apis"
)

func getIntGE0(m map[string]string, k string) (int64, *apis.FieldError) {
	v, ok := m[k]
	if !ok {
		return 0, nil
	}
	i, err := strconv.ParseInt(v, 10, 32)
	if err != nil || i < 0 {
		return 0, &apis.FieldError{
			Message: fmt.Sprintf("Invalid %s annotation value: must be an integer equal or greater than 0", k),
			Paths:   []string{k},
		}
	}
	return i, nil
}

func ValidateAnnotations(annotations map[string]string) *apis.FieldError {
	if len(annotations) == 0 {
		return nil
	}

	min, err := getIntGE0(annotations, MinScaleAnnotationKey)
	if err != nil {
		return err
	}
	max, err := getIntGE0(annotations, MaxScaleAnnotationKey)
	if err != nil {
		return err
	}

	if max != 0 && max < min {
		return &apis.FieldError{
			Message: fmt.Sprintf("%s=%v is less than %s=%v", MaxScaleAnnotationKey, max, MinScaleAnnotationKey, min),
			Paths:   []string{MaxScaleAnnotationKey, MinScaleAnnotationKey},
		}
	}

	return nil
}
