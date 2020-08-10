package shard

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

// Return a TrainedModel's shardId
func (v *ShardManager) GetShardIdForTrainedModel(trainedModel *v1beta1api.TrainedModel) int {
	//TODO to be implemented in another PR
	return 0
}
