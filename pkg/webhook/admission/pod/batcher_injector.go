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

package pod

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	BatcherContainerName        = "batcher"
	BatcherConfigMapKeyName     = "batcher"
	BatcherEnableFlag           = "--enable-batcher"
	BatcherArgumentMaxBatchSize = "--max-batchsize"
	BatcherArgumentMaxLatency   = "--max-latency"
	BatcherArgumentTimeout      = "--timeout"
)

type BatcherConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

type BatcherInjector struct {
	config *BatcherConfig
}

func getBatcherConfigs(configMap *v1.ConfigMap) (*BatcherConfig, error) {

	batcherConfig := &BatcherConfig{}
	if batcherConfigValue, ok := configMap.Data[BatcherConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(batcherConfigValue), &batcherConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall batcher json string due to %v ", err))
		}
	}

	//Ensure that we set proper values for CPU/Memory Limit/Request
	resourceDefaults := []string{batcherConfig.MemoryRequest,
		batcherConfig.MemoryLimit,
		batcherConfig.CpuRequest,
		batcherConfig.CpuLimit}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return batcherConfig, fmt.Errorf("Failed to parse resource configuration for %q: %q",
				BatcherConfigMapKeyName, err.Error())
		}
	}

	return batcherConfig, nil
}

func (il *BatcherInjector) InjectBatcher(pod *v1.Pod) error {
	// Only inject if the required annotations are set
	_, ok := pod.ObjectMeta.Annotations[constants.BatcherInternalAnnotationKey]
	if !ok {
		return nil
	}

	var args []string

	maxBatchSize, ok := pod.ObjectMeta.Annotations[constants.BatcherMaxBatchSizeInternalAnnotationKey]
	if ok {
		args = append(args, BatcherArgumentMaxBatchSize)
		args = append(args, maxBatchSize)
	}

	maxLatency, ok := pod.ObjectMeta.Annotations[constants.BatcherMaxLatencyInternalAnnotationKey]
	if ok {
		args = append(args, BatcherArgumentMaxLatency)
		args = append(args, maxLatency)
	}

	timeout, ok := pod.ObjectMeta.Annotations[constants.BatcherTimeoutInternalAnnotationKey]
	if ok {
		args = append(args, BatcherArgumentTimeout)
		args = append(args, timeout)
	}

	// Don't inject if Container already injected
	for _, container := range pod.Spec.Containers {
		if strings.Compare(container.Name, BatcherContainerName) == 0 {
			return nil
		}
	}

	// Make sure securityContext is initialized and valid
	securityContext := pod.Spec.Containers[0].SecurityContext.DeepCopy()

	batcherContainer := &v1.Container{
		Name:  BatcherContainerName,
		Image: il.config.Image,
		Args:  args,
		Resources: v1.ResourceRequirements{
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(il.config.CpuLimit),
				v1.ResourceMemory: resource.MustParse(il.config.MemoryLimit),
			},
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(il.config.CpuRequest),
				v1.ResourceMemory: resource.MustParse(il.config.MemoryRequest),
			},
		},
		SecurityContext: securityContext,
	}

	// Add container to the spec
	pod.Spec.Containers = append(pod.Spec.Containers, *batcherContainer)

	return nil
}
