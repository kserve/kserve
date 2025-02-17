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

package memory

import (
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// TODO MemoryStrategy will be implemented in another PR
type MemoryStrategy struct{}

// Return a TrainedModel's shardId
func (v *MemoryStrategy) GetOrAssignShard(tm *v1alpha1.TrainedModel) int {
	// TODO to be implemented in another PR
	// Currently each InferenceService only has one shard with id=0
	return 0
}

func (v *MemoryStrategy) GetShard(isvc *v1beta1.InferenceService) []int {
	// TODO to be implemented in another PR
	// Currently each InferenceService only has one shard with id=0
	return []int{0}
}
