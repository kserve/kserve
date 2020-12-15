package pod

import (
	v1 "k8s.io/api/core/v1"
	"strings"
)

const (
	ArchTypeForARM64 = "arm64"
	TargetImageName  = "queue-proxy"
	ArchTypeLabel    = "archType"
)

func MuteImageTag(pod *v1.Pod) error {
	if selector, ok := pod.ObjectMeta.Labels[ArchTypeLabel]; ok {
		if strings.Contains(selector, ArchTypeForARM64) {
			for idx, container := range pod.Spec.Containers {
				if strings.Compare(container.Name, TargetImageName) == 0 {
					pod.Spec.Containers[idx].Image += "-arm64"
					break
				}
			}
		}
	}
	return nil
}
