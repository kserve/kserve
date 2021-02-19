package pod

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"strings"
)

const (
	ArchTypeForARM64 = "arm64"
	TargetImageName  = "queue-proxy"
	ArchTypeLabel    = "archType"
	VersionLabel     = "imageVersion"
	ChangeImageName  = "kfserving-container"
)

func MuteImageTag(pod *v1.Pod) error {
	if version, ok := pod.ObjectMeta.Labels[VersionLabel]; ok {
		klog.Info("version is", version)
		for _, container := range pod.Spec.Containers {
			klog.Info("container is", container.Name, container.Image)
			if strings.Compare(container.Name, ChangeImageName) == 0 {
				if strings.Contains(container.Image, "@") {
					klog.Info("current image name is", container.Image)
					container.Image = strings.SplitN(container.Image, "@", 2)[0] + ":" + version
				}
				break
			}
		}
	}
	//if selector, ok := pod.ObjectMeta.Labels[ArchTypeLabel]; ok {

	//if strings.Contains(selector, ArchTypeForARM64) {
	//	for _, container := range pod.Spec.Containers {
	//		if strings.Compare(container.Name, TargetImageName) == 0 {
	//			if strings.Contains(container.Image, "/") {
	//				if strings.Contains(strings.SplitN(container.Image, "/", 2)[1], ":") {
	//					container.Image += "-arm64"
	//				} else {
	//					container.Image += ":latest-arm64"
	//				}
	//			}
	//
	//			break
	//		}
	//	}
	//
	//	// modify storage init container
	//	for _, container := range pod.Spec.InitContainers {
	//		if strings.Compare(container.Name, StorageInitializerContainerName) == 0 {
	//			if strings.Contains(container.Image, "/") {
	//				if strings.Contains(strings.SplitN(container.Image, "/", 2)[1], ":") {
	//					container.Image += "-arm64"
	//				} else {
	//					container.Image += ":latest-arm64"
	//				}
	//			}
	//			break
	//		}
	//	}
	//}
	//}
	return nil
}
