/*
Copyright 2023 The KServe Authors.

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

package servingruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	runtime "k8s.io/apimachinery/pkg/runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestValidateServingRuntimePriority(t *testing.T) {
	scenarios := map[string]struct {
		name                   string
		newServingRuntime      *v1alpha1.ServingRuntime
		existingServingRuntime *v1alpha1.ServingRuntime
		expected               gomega.OmegaMatcher
	}{
		"When both serving runtimes are not MMS it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(true),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When priority is same for model format in multi model serving runtime it should return error": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(true),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(true),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.3.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidPriorityError, "sklearn")),
		},
		"When existing serving runtime is disabled it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(true),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When new serving runtime and existing runtime are same it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When model format is same and supported protocol version is different it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When model format is different it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "lightgbm",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When autoselect is false in the new serving runtime it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(false),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When autoselect is not specified in the new serving runtime it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:     "sklearn",
							Version:  proto.String("1"),
							Priority: proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When autoselect is false in the existing serving runtime it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(false),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When model version is nil in both serving runtime and priority is same it should return error": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidPriorityError, "sklearn")),
		},
		"When model version is nil in both serving runtime and priority is not same then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(2),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When model version is nil in new serving runtime and priority is same then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When model version is nil in existing serving runtime and priority is same then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When model version is same in both serving runtime and priority is same it should return error": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidPriorityError, "sklearn")),
		},
		"When model version is different but priority is same then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1.3"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1.0"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When priority is nil in both serving runtime then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When priority is nil in new serving runtime and priority is specified in existing serving runtime then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
		"When priority is nil in existing serving runtime and priority is specified in new serving runtime then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "seldonio/mlserver:1.2.0",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			err := validateServingRuntimePriority(&scenario.newServingRuntime.Spec, &scenario.existingServingRuntime.Spec,
				scenario.newServingRuntime.Name, scenario.existingServingRuntime.Name)
			g.Expect(err).To(scenario.expected)
		})
	}
}

func TestValidateModelFormatPrioritySame(t *testing.T) {
	scenarios := map[string]struct {
		name              string
		newServingRuntime *v1alpha1.ServingRuntime
		expected          gomega.OmegaMatcher
	}{
		"When different priority assigned for the same model format in the runtime it should return error": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(1),
						},
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(2),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(ProrityIsNotSameError, "sklearn")),
		},
		"When same priority assigned for the same model format in the runtime then it should return nil": {
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(2),
						},
						{
							Name:       "sklearn",
							AutoSelect: proto.Bool(true),
							Priority:   proto.Int32(2),
						},
					},
					MultiModel: proto.Bool(false),
					Disabled:   proto.Bool(false),
					ProtocolVersions: []constants.InferenceServiceProtocol{
						constants.ProtocolV1,
						constants.ProtocolV2,
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.BeNil(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			err := validateModelFormatPrioritySame(&scenario.newServingRuntime.Spec)
			g.Expect(err).To(scenario.expected)
		})
	}
}

func TestValidateMultiNodeVariables(t *testing.T) {
	scenarios := map[string]struct {
		name                   string
		newServingRuntime      *v1alpha1.ServingRuntime
		existingServingRuntime *v1alpha1.ServingRuntime
		expected               gomega.OmegaMatcher
	}{
		"When pipeline-parallel-size set less than 1, it should return error": {
			existingServingRuntime: &v1alpha1.ServingRuntime{},
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						PipelineParallelSize: intPtr(0),
						TensorParallelSize:   intPtr(1),
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{
								{
									Name:    "worker-container",
									Image:   "kserve/huggingfaceserver:latest",
									Command: []string{"bash", "-c"},
									Args: []string{
										"ray start --address=$RAY_HEAD_ADDRESS --block",
									},
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidWorkerSpecPipelineParallelSizeValueError, "0")),
		},
		"When tensor-parallel-size set less than 1, it should return error": {
			existingServingRuntime: &v1alpha1.ServingRuntime{},
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-2",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						PipelineParallelSize: intPtr(2),
						TensorParallelSize:   intPtr(0),
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{
								{
									Name:    "worker-container",
									Image:   "kserve/huggingfaceserver:latest",
									Command: []string{"bash", "-c"},
									Args: []string{
										"ray start --address=$RAY_HEAD_ADDRESS --block",
									},
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(fmt.Errorf(InvalidWorkerSpecTensorParallelSizeValueError, "0")),
		},
		"When pipeline-parallel-size set in the environment, it should return error": {
			existingServingRuntime: &v1alpha1.ServingRuntime{},
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-3",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
								Env: []corev1.EnvVar{
									{Name: constants.PipelineParallelSizeEnvName, Value: "test"},
								},
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{
								{
									Name:    "worker-container",
									Image:   "kserve/huggingfaceserver:latest",
									Command: []string{"bash", "-c"},
									Args: []string{
										"ray start --address=$RAY_HEAD_ADDRESS --block",
									},
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(errors.New(DisallowedWorkerSpecPipelineParallelSizeEnvError)),
		},
		"When tensor-parallel-size set in the environment, it should return error": {
			existingServingRuntime: &v1alpha1.ServingRuntime{},
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-4",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
								Env: []corev1.EnvVar{
									{Name: constants.TensorParallelSizeEnvName, Value: "test"},
								},
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{
								{
									Name:    "worker-container",
									Image:   "kserve/huggingfaceserver:latest",
									Command: []string{"bash", "-c"},
									Args: []string{
										"ray start --address=$RAY_HEAD_ADDRESS --block",
									},
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(errors.New(DisallowedWorkerSpecTensorParallelSizeEnvError)),
		},
		"when the existing workerSpec is removed from the servingRuntime, it should return error": {
			existingServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-5",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{
								{
									Name:    "worker-container",
									Image:   "kserve/huggingfaceserver:latest",
									Command: []string{"bash", "-c"},
									Args: []string{
										"ray start --address=$RAY_HEAD_ADDRESS --block",
									},
								},
							},
						},
					},
				},
			},
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-1",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
							},
						},
					},
				},
			},
			expected: gomega.Equal(errors.New(DisallowedRemovingWorkerSpecFromServingRuntimeError)),
		},
		"When multiple containers set in WorkerSpec, it should return error": {
			existingServingRuntime: &v1alpha1.ServingRuntime{},
			newServingRuntime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-runtime-6",
					Namespace: "test",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "kserve/sklearnserver:latest",
								Args: []string{
									"--model_name={{.Name}}",
									"--model_dir=/mnt/models",
									"--http_port=8080",
								},
								Env: []corev1.EnvVar{
									{Name: constants.TensorParallelSizeEnvName, Value: "test"},
								},
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{
								{},
								{},
							},
						},
					},
				},
			},
			expected: gomega.Equal(errors.New(DisallowedMultipleContainersInWorkerSpecError)),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			err := validateMultiNodeSpec(&scenario.newServingRuntime.Spec, &scenario.existingServingRuntime.Spec)
			g.Expect(err).To(scenario.expected)
		})
	}
}

func TestServingRuntimeValidator_Handle(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	type fields struct {
		Client  client.Client
		Decoder admission.Decoder
	}
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name           string
		setupObjs      []client.Object
		runtime        *v1alpha1.ServingRuntime
		decoderErr     error
		listErr        error
		wantAllowed    bool
		wantDenied     bool
		wantStatusCode int
	}{
		{
			name: "allow when runtime is disabled",
			runtime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "sr1", Namespace: "ns1"},
				Spec: v1alpha1.ServingRuntimeSpec{
					Disabled: proto.Bool(true),
				},
			},
			wantAllowed: true,
		},
		{
			name: "deny when model format priorities are not the same",
			runtime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "sr2", Namespace: "ns2"},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{Name: "onnx", AutoSelect: proto.Bool(true), Priority: proto.Int32(1)},
						{Name: "onnx", AutoSelect: proto.Bool(true), Priority: proto.Int32(2)},
					},
				},
			},
			wantDenied: true,
		},
		{
			name: "deny when runtime priorities conflict with existing",
			setupObjs: []client.Object{
				&v1alpha1.ServingRuntime{
					ObjectMeta: metav1.ObjectMeta{Name: "sr3", Namespace: "ns3"},
					Spec: v1alpha1.ServingRuntimeSpec{
						MultiModel:       proto.Bool(true),
						ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1},
						SupportedModelFormats: []v1alpha1.SupportedModelFormat{
							{Name: "onnx", AutoSelect: proto.Bool(true), Priority: proto.Int32(1)},
						},
					},
				},
			},
			runtime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "sr4", Namespace: "ns3"},
				Spec: v1alpha1.ServingRuntimeSpec{
					MultiModel:       proto.Bool(true),
					ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1},
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{Name: "onnx", AutoSelect: proto.Bool(true), Priority: proto.Int32(1)},
					},
				},
			},
			wantDenied: true,
		},
		{
			name: "deny when multinode spec is invalid (removing workerSpec)",
			setupObjs: []client.Object{
				&v1alpha1.ServingRuntime{
					ObjectMeta: metav1.ObjectMeta{Name: "sr5", Namespace: "ns4"},
					Spec: v1alpha1.ServingRuntimeSpec{
						WorkerSpec: &v1alpha1.WorkerSpec{
							ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
								Containers: []corev1.Container{{Name: "worker"}},
							},
						},
					},
				},
			},
			runtime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "sr5", Namespace: "ns4"},
				Spec:       v1alpha1.ServingRuntimeSpec{},
			},
			wantDenied: true,
		},
		{
			name: "allow valid runtime",
			setupObjs: []client.Object{
				&v1alpha1.ServingRuntime{
					ObjectMeta: metav1.ObjectMeta{Name: "sr6", Namespace: "ns5"},
					Spec: v1alpha1.ServingRuntimeSpec{
						MultiModel:       proto.Bool(true),
						ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1},
						SupportedModelFormats: []v1alpha1.SupportedModelFormat{
							{Name: "onnx", AutoSelect: proto.Bool(true), Priority: proto.Int32(1)},
						},
					},
				},
			},
			runtime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "sr7", Namespace: "ns5"},
				Spec: v1alpha1.ServingRuntimeSpec{
					MultiModel:       proto.Bool(true),
					ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1},
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{Name: "onnx", AutoSelect: proto.Bool(true), Priority: proto.Int32(2)},
					},
				},
			},
			wantAllowed: true,
		},
		{
			name:           "error decoding request",
			runtime:        &v1alpha1.ServingRuntime{},
			decoderErr:     errors.New("decode error"),
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name: "error listing runtimes",
			runtime: &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "sr8", Namespace: "ns6"},
			},
			listErr:        errors.New("list error"),
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fake client and decoder
			fakeClient := &fakeClient{
				objs:    tt.setupObjs,
				listErr: tt.listErr,
			}
			fakeDecoder := &fakeDecoder{
				obj: tt.runtime,
				err: tt.decoderErr,
			}
			validator := &ServingRuntimeValidator{
				Client:  fakeClient,
				Decoder: fakeDecoder,
			}
			req := admission.Request{}
			resp := validator.Handle(context.TODO(), req)
			if tt.wantAllowed {
				g.Expect(resp.Allowed).To(gomega.BeTrue())
			}
			if tt.wantStatusCode != 0 {
				g.Expect(int(resp.Result.Code)).To(gomega.Equal(tt.wantStatusCode))
			}
		})
	}
}

type fakeClient struct {
	client.Client
	objs    []client.Object
	listErr error
}

func (f *fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	if l, ok := list.(*v1alpha1.ServingRuntimeList); ok {
		for _, obj := range f.objs {
			if sr, ok := obj.(*v1alpha1.ServingRuntime); ok {
				l.Items = append(l.Items, *sr)
			}
		}
	}
	return nil
}

// ...

type fakeDecoder struct {
	admission.Decoder
	obj interface{}
	err error
}

func (f *fakeDecoder) Decode(_ admission.Request, into runtime.Object) error {
	if f.err != nil {
		return f.err
	}
	if v, ok := into.(*v1alpha1.ServingRuntime); ok {
		*v = *(f.obj.(*v1alpha1.ServingRuntime))
	}
	return nil
}

func intPtr(i int) *int {
	return &i
}
