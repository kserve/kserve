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
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/version"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	kservetypes "github.com/kserve/kserve/pkg/types"
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
// LLMInferenceService. See that package for full threshold documentation.
func warnIfImageVolumeUnsupported(ctx context.Context, sv serverVersioner, llmSvc *v1alpha2.LLMInferenceService, resolvedMode string) {
	mgr := llmSvc.GetConditionSet().Manage(llmSvc.GetStatus())

	if resolvedMode != kservetypes.OciModelModeNative {
		_ = mgr.ClearCondition(ociImageVolumeCompatible)
		return
	}

	logger := log.FromContext(ctx)
	v, err := sv.ServerVersion()
	if err != nil {
		logger.V(1).Info("Skipping ImageVolume cluster version check: discovery failed", "error", err)
		return
	}

	minor, err := strconv.Atoi(strings.TrimSuffix(v.Minor, "+"))
	if err != nil {
		logger.V(1).Info("Skipping ImageVolume cluster version check: cannot parse minor version", "minor", v.Minor)
		return
	}

	if minor < 31 {
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeUnsupported",
			"Cluster K8s %s.%s does not support ImageVolume (introduced in 1.31). Falling back to modelcar may be required.",
			v.Major, v.Minor)
		return
	}

	if minor < 33 {
		mgr.MarkFalse(ociImageVolumeCompatible, "ImageVolumeAlpha",
			"Cluster K8s %s.%s has ImageVolume only in alpha (feature-gated). Ensure --feature-gates=ImageVolume=true is set on kube-apiserver and kubelet.",
			v.Major, v.Minor)
		return
	}

	// minor >= 33: beta or later — clear any previous warning.
	_ = mgr.ClearCondition(ociImageVolumeCompatible)
}
