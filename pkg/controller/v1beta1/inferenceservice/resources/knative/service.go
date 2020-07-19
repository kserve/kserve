/*
Copyright 2020 kubeflow.org.

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

package knative

import (
	"fmt"
	"strconv"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
)

var serviceAnnotationDisallowedList = []string{
	autoscaling.MinScaleAnnotationKey,
	autoscaling.MaxScaleAnnotationKey,
	constants.StorageInitializerSourceUriInternalAnnotationKey,
	"kubectl.kubernetes.io/last-applied-configuration",
}

func AddLoggerAnnotations(logger *v1alpha2.Logger, annotations map[string]string) bool {
	if logger != nil {
		annotations[constants.LoggerInternalAnnotationKey] = "true"
		if logger.Url != nil {
			annotations[constants.LoggerSinkUrlInternalAnnotationKey] = *logger.Url
		}
		annotations[constants.LoggerModeInternalAnnotationKey] = string(logger.Mode)
		return true
	}
	return false
}

func AddLoggerContainerPort(container *v1.Container) {
	if container != nil {
		if container.Ports == nil {
			port, _ := strconv.Atoi(constants.InferenceServiceDefaultLoggerPort)
			container.Ports = []v1.ContainerPort{
				v1.ContainerPort{
					ContainerPort: int32(port),
				},
			}
		}
	}
}

func AddBatcherAnnotations(batcher *v1alpha2.Batcher, annotations map[string]string) bool {
	if batcher != nil {
		annotations[constants.BatcherInternalAnnotationKey] = "true"

		if batcher.MaxBatchSize != nil {
			s := strconv.Itoa(*batcher.MaxBatchSize)
			annotations[constants.BatcherMaxBatchSizeInternalAnnotationKey] = s
		}
		if batcher.MaxLatency != nil {
			s := strconv.Itoa(*batcher.MaxLatency)
			annotations[constants.BatcherMaxLatencyInternalAnnotationKey] = s
		}
		if batcher.Timeout != nil {
			s := strconv.Itoa(*batcher.Timeout)
			annotations[constants.BatcherTimeoutInternalAnnotationKey] = s
		}
		return true
	}
	return false
}

func AddBatcherContainerPort(container *v1.Container) {
	if container != nil {
		if container.Ports == nil {
			port, _ := strconv.Atoi(constants.InferenceServiceDefaultBatcherPort)
			container.Ports = []v1.ContainerPort{
				{
					ContainerPort: int32(port),
				},
			}
		}
	}
}

func BuildAnnotations(metadata metav1.ObjectMeta, minReplicas *int, maxReplicas int, parallelism int) (map[string]string, error) {
	annotations := utils.Filter(metadata.Annotations, func(key string) bool {
		return !utils.Includes(serviceAnnotationDisallowedList, key)
	})

	if minReplicas == nil {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(constants.DefaultMinReplicas)
	} else if *minReplicas != 0 {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(*minReplicas)
	}

	if maxReplicas != 0 {
		annotations[autoscaling.MaxScaleAnnotationKey] = fmt.Sprint(maxReplicas)
	}

	// User can pass down scaling target annotation to overwrite the target default 1
	if _, ok := annotations[autoscaling.TargetAnnotationKey]; !ok {
		if parallelism == 0 {
			annotations[autoscaling.TargetAnnotationKey] = constants.DefaultScalingTarget
		} else {
			annotations[autoscaling.TargetAnnotationKey] = strconv.Itoa(parallelism)
		}
	}
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}
	return annotations, nil
}
