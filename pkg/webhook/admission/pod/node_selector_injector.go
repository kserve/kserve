package pod

import (
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

const (
	NodeSelectorLabel = "gpuType"
)

func InjectNodeSelector(pod *v1.Pod) error {
	if selector, ok := pod.ObjectMeta.Labels[NodeSelectorLabel]; ok {
		pod.Spec.NodeSelector = utils.Union(
			pod.Spec.NodeSelector,
			map[string]string{NodeSelectorLabel: selector},
		)
	}
	return nil
}
