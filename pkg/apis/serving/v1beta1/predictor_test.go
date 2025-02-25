/*
Copyright 2022 The KServe Authors.

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

package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/constants"
)

func TestGetImplementations(t *testing.T) {
	tests := []struct {
		name           string
		predictorSpec  *PredictorSpec
		expectedLength int
	}{
		{
			name: "Single implementation - PyTorch",
			predictorSpec: &PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: ptr.To("0.4.1"),
					},
				},
			},
			expectedLength: 1,
		},
		{
			name: "Pytorch with transformer container",
			predictorSpec: &PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: ptr.To("0.4.1"),
					},
				},
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.TransformerContainerName,
						},
					},
				},
			},
			expectedLength: 1,
		},
		{
			name: "Multiple implementations - PyTorch and Tensorflow",
			predictorSpec: &PredictorSpec{
				PyTorch: &TorchServeSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: ptr.To("0.4.1"),
					},
				},
				Tensorflow: &TFServingSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						RuntimeVersion: ptr.To("2.0.0"),
					},
				},
			},
			expectedLength: 2,
		},
		{
			name: "Custom predictor with transformer container",
			predictorSpec: &PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: constants.TransformerContainerName,
						},
					},
				},
			},
			expectedLength: 1,
		},
		{
			name: "Custom predictor with containers",
			predictorSpec: &PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "another-container",
						},
					},
				},
			},
			expectedLength: 1,
		},
		{
			name: "Custom predictor",
			predictorSpec: &PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expectedLength: 1,
		},
		{ // If only one container is specified, it is assumed to be the predictor container
			name: "Custom predictor without container name",
			predictorSpec: &PredictorSpec{
				PodSpec: PodSpec{
					Containers: []corev1.Container{
						{
							Image: "image",
						},
					},
				},
			},
			expectedLength: 1,
		},
		{
			name:           "No implementations",
			predictorSpec:  &PredictorSpec{},
			expectedLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			implementations := tt.predictorSpec.GetImplementations()
			assert.Len(t, implementations, tt.expectedLength)
		})
	}
}
