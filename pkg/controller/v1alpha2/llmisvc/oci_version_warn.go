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

package llmisvc

import (
	"context"

	"k8s.io/apimachinery/pkg/version"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	kservetypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

// ociImageVolumeCompatible is the advisory condition type surfaced on an
// LLMInferenceService when native OCI ImageVolume mode is in use and the cluster
// Kubernetes version may not support it.
const ociImageVolumeCompatible apis.ConditionType = "OciImageVolumeCompatible"

// serverVersioner is a single-method interface for K8s version discovery.
type serverVersioner interface {
	ServerVersion() (*version.Info, error)
}

// warnIfImageVolumeUnsupported mirrors the ISVC-controller helper for
// LLMInferenceService. Compatibility thresholds are handled by the shared helper
// utils.CheckImageVolumeCompatibility; this function translates the result into
// the LLMInferenceService condition format (MarkFalse path on its conditionSet).
func warnIfImageVolumeUnsupported(ctx context.Context, sv serverVersioner, llmSvc *v1alpha2.LLMInferenceService, resolvedMode string) {
	mgr := llmSvc.GetConditionSet().Manage(llmSvc.GetStatus())

	if resolvedMode != kservetypes.OciModelModeNative {
		_ = mgr.ClearCondition(ociImageVolumeCompatible)
		return
	}

	result := utils.CheckImageVolumeCompatibility(ctx, sv)

	switch result.Status {
	case utils.ImageVolumeUnsupported:
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeUnsupported",
			"Cluster K8s %s.%s does not support ImageVolume (introduced in 1.31 as alpha). Falling back to modelcar may be required.",
			result.Major, result.Minor)
	case utils.ImageVolumeNeedsGate:
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeAlpha",
			"Cluster K8s %s.%s has ImageVolume feature-gated (K8s 1.31–1.34). Ensure --feature-gates=ImageVolume=true is set on kube-apiserver and kubelet.",
			result.Major, result.Minor)
	default:
		// ImageVolumeOK (≥ 1.35) or ImageVolumeUnknown — clear any previous warning.
		_ = mgr.ClearCondition(ociImageVolumeCompatible)
	}
}
