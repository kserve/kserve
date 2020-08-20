package main


import v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"

type Strategy interface {
	GetOrAssignShard(trainedModel *v1beta1api.TrainedModel) int
}
