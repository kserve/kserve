package servingruntime

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
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
		"When priority is same for model format in multi model serving runtime then it should return error": {
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
		"When model version is nil in both serving runtime and priority is same then it should return error": {
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
		"When model version is same in both serving runtime and priority is same then it should return error": {
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
