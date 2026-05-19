/*
Copyright 2025 The KServe Authors.

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

package kernelcachecommon

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// LoadKernelCacheConfig loads and parses the KernelCache configuration from the ConfigMap.
// It applies default values for any unset fields and returns the updated configuration.
func LoadKernelCacheConfig(ctx context.Context, clientset kubernetes.Interface) (*v1beta1.KernelCacheConfig, error) {
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, clientset)
	if err != nil {
		return nil, err
	}

	kernelCacheConfig, err := v1beta1.NewKernelCacheConfig(isvcConfigMap)
	if err != nil {
		// Return defaults on parse error
		kernelCacheConfig = &v1beta1.KernelCacheConfig{}
	}

	// Apply defaults for unset fields
	if kernelCacheConfig.JobNamespace == "" {
		kernelCacheConfig.JobNamespace = DefaultJobNamespace
	}
	if kernelCacheConfig.ExtractImage == "" {
		kernelCacheConfig.ExtractImage = DefaultExtractImage
	}
	if kernelCacheConfig.JobTTLSecondsAfterFinished == nil {
		kernelCacheConfig.JobTTLSecondsAfterFinished = ptr.To(DefaultJobTTLSecondsAfterFinished)
	}
	if kernelCacheConfig.ReconcileIntervalSeconds == nil {
		kernelCacheConfig.ReconcileIntervalSeconds = ptr.To(DefaultReconcileIntervalSeconds)
	}

	return kernelCacheConfig, nil
}
