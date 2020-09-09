package memory

import (
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
)

//TODO MemoryStrategy will be implemented in another PR
type MemoryStrategy struct {
}

// Return a TrainedModel's shardId
func (v *MemoryStrategy) GetOrAssignShard(trainedModel *v1beta1api.TrainedModel) int {
	//TODO to be implemented in another PR
	//Currently each InferenceService only has one shard with id=0
	return 0
}
