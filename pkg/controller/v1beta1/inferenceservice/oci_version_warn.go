/*
Copyright 2025 The KServe Authors.

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

package inferenceservice

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/version"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kservetypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

// OciImageVolumeCompatible is an advisory condition surfaced on an InferenceService
// when native OCI ImageVolume mode is in use and the cluster Kubernetes version may
// not support it. It never affects the Ready condition (not in conditionSet).
const OciImageVolumeCompatible apis.ConditionType = "OciImageVolumeCompatible"

// serverVersioner is the subset of discovery.DiscoveryInterface required by
// warnIfImageVolumeUnsupported. Using a minimal interface enables injection of
// a lightweight fake in unit tests without implementing all ~30 discovery methods.
type serverVersioner interface {
	ServerVersion() (*version.Info, error)
}

// warnIfImageVolumeUnsupported sets an advisory condition on isvc when the resolved
// storage mode is "native" and the cluster does not have ImageVolume enabled by default.
// The compatibility thresholds and version discovery are handled by the shared helper
// utils.CheckImageVolumeCompatibility; this function translates the result into the
// ISVC condition format.
func warnIfImageVolumeUnsupported(ctx context.Context, sv serverVersioner, isvc *v1beta1.InferenceService, resolvedMode string) {
	if resolvedMode != kservetypes.OciModelModeNative {
		isvc.Status.ClearCondition(OciImageVolumeCompatible)
		return
	}

	result := utils.CheckImageVolumeCompatibility(ctx, sv)

	switch result.Status {
	case utils.ImageVolumeUnsupported:
		isvc.Status.SetCondition(OciImageVolumeCompatible, &apis.Condition{
			Type:   OciImageVolumeCompatible,
			Status: corev1.ConditionFalse,
			Reason: "ImageVolumeUnsupported",
			Message: fmt.Sprintf(
				"Cluster K8s %s.%s does not support ImageVolume (introduced in 1.31 as alpha). Falling back to modelcar may be required.",
				result.Major, result.Minor),
		})
	case utils.ImageVolumeNeedsGate:
		isvc.Status.SetCondition(OciImageVolumeCompatible, &apis.Condition{
			Type:   OciImageVolumeCompatible,
			Status: corev1.ConditionFalse,
			Reason: "ImageVolumeAlpha",
			Message: fmt.Sprintf(
				"Cluster K8s %s.%s has ImageVolume feature-gated (K8s 1.31–1.34). Ensure --feature-gates=ImageVolume=true is set on kube-apiserver and kubelet.",
				result.Major, result.Minor),
		})
	default:
		// ImageVolumeOK (≥ 1.35) or ImageVolumeUnknown — clear any previously set warning.
		isvc.Status.ClearCondition(OciImageVolumeCompatible)
	}
}
