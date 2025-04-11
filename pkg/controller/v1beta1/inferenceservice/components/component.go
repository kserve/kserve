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

package components

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/sharding/memory"
	v1beta1utils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/credentials"
)

// Component can be reconciled to create underlying resources for an InferenceService
type Component interface {
	Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) (ctrl.Result, error)
}

func addStorageSpecAnnotations(storageSpec *v1beta1.StorageSpec, annotations map[string]string) bool {
	if storageSpec == nil {
		return false
	}
	annotations[constants.StorageSpecAnnotationKey] = "true"
	if storageSpec.Parameters != nil {
		if jsonParam, err := json.Marshal(storageSpec.Parameters); err == nil {
			annotations[constants.StorageSpecParamAnnotationKey] = string(jsonParam)
		}
	}
	if storageSpec.StorageKey != nil {
		annotations[constants.StorageSpecKeyAnnotationKey] = *storageSpec.StorageKey
	}
	if storageSpec.Path != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = fmt.Sprintf("%s://%s", credentials.UriSchemePlaceholder,
			strings.TrimPrefix(*storageSpec.Path, "/"))
	}
	return true
}

func addLoggerAnnotations(logger *v1beta1.LoggerSpec, annotations map[string]string) {
	if logger != nil {
		annotations[constants.LoggerInternalAnnotationKey] = "true"
		if logger.URL != nil {
			annotations[constants.LoggerSinkUrlInternalAnnotationKey] = *logger.URL
		}
		annotations[constants.LoggerModeInternalAnnotationKey] = string(logger.Mode)

		if logger.MetadataHeaders != nil {
			annotations[constants.LoggerMetadataHeadersInternalAnnotationKey] = strings.Join(logger.MetadataHeaders, ",")
		}
		if logger.MetadataAnnotations != nil {
			annotations[constants.LoggerMetadataAnnotationsInternalAnnotationKey] = strings.Join(logger.MetadataAnnotations, ",")
		}
	}
}

func addBatcherAnnotations(batcher *v1beta1.Batcher, annotations map[string]string) {
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
	}
}

func addAgentAnnotations(isvc *v1beta1.InferenceService, annotations map[string]string) bool {
	if v1beta1utils.IsMMSPredictor(&isvc.Spec.Predictor) {
		annotations[constants.AgentShouldInjectAnnotationKey] = "true"
		shardStrategy := memory.MemoryStrategy{}
		for _, id := range shardStrategy.GetShard(isvc) {
			multiModelConfigMapName := constants.ModelConfigName(isvc.Name, id)
			annotations[constants.AgentModelConfigVolumeNameAnnotationKey] = multiModelConfigMapName
			annotations[constants.AgentModelConfigMountPathAnnotationKey] = constants.ModelConfigDir
			annotations[constants.AgentModelDirAnnotationKey] = constants.ModelDir
		}
		return true
	}
	return false
}
