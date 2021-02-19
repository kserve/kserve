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
			for _, container := range pod.Spec.Containers {
				if strings.Compare(container.Name, TargetImageName) == 0 {
					if strings.Contains(container.Image, "/") && strings.Contains(strings.SplitN(container.Image, "/", 2)[1], ":") {
						container.Image += "-arm64"
					} else {
						container.Image += ":latest-arm64"
					}

					break
				}
			}

			// modify storage init container
			for _, container := range pod.Spec.InitContainers {
				if strings.Compare(container.Name, StorageInitializerContainerName) == 0 {
					if strings.Contains(container.Image, "/") && strings.Contains(strings.SplitN(container.Image, "/", 2)[1], ":") {
						container.Image += "-arm64"
					} else {
						container.Image += ":latest-arm64"
					}
					break
				}
			}
		}
	}
	return nil
}
