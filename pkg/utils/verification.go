package utils

import (
	"github.com/kserve/kserve/pkg/constants"
	corev1 "k8s.io/api/core/v1"
)

// ApplyVerificationContainerConfig configures a container for model verification
// by injecting the required environment variables.
func ApplyVerificationContainerConfig(
	container *corev1.Container,
	digest *string,
) {
	if digest != nil {
		AddOrReplaceEnv(container, constants.VerificationDigestEnvVar, *digest)
	}
}