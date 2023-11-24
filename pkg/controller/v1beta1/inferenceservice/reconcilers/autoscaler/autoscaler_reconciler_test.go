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

package autoscaler

import (
	"github.com/google/go-cmp/cmp"
	"github.com/kserve/kserve/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"testing"
)

func TestGetAutoscalerClass(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"

	testCases := []struct {
		name                   string
		isvcMetaData           *metav1.ObjectMeta
		expectedAutoScalerType constants.AutoscalerClassType
	}{
		{
			name: "Return default AutoScaler,if the autoscalerClass annotation is not set",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{},
			},

			expectedAutoScalerType: constants.AutoscalerClassHPA,
		},
		{
			name: "Return default AutoScaler,if the autoscalerClass annotation set hpa",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "hpa"},
			},

			expectedAutoScalerType: constants.AutoscalerClassHPA,
		},
		{
			name: "Return external AutoScaler,if the autoscalerClass annotation set external",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "external"},
			},
			expectedAutoScalerType: constants.AutoscalerClassExternal,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getAutoscalerClass(*tt.isvcMetaData)
			if diff := cmp.Diff(tt.expectedAutoScalerType, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}
