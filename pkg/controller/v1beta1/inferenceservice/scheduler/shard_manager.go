package scheduler

import (
	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
)

type ShardingStrategy string

const (
	Memory ShardingStrategy = "Memory"
)

type ShardManager struct {
	Strategy ShardingStrategy
}

// Return all the shard ids for an InferenceService
func (v *ShardManager) GetShardIdsForInferenceService(isvc *v1beta1api.InferenceService) []int {
	//TODO implement sharding logic for InferenceService
	//Currently each InfereceService only has one shard with id 0
	ids := []int{0}
	return ids
}

// Return a TrainedModel's shardId
func (v *ShardManager) GetShardIdForTrainedMode(trainedModel *v1beta1api.TrainedModel) int {
	//TODO to be implemented in another PR
	return 0
}
