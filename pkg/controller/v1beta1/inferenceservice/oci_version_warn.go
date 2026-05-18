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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/version"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kservetypes "github.com/kserve/kserve/pkg/types"
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

// warnIfImageVolumeUnsupported sets an advisory condition on isvc when the
// resolved storage mode is "native" and the cluster K8s minor version is below
// the ImageVolume support threshold defined by KEP-4639:
//
//   - minor < 31  → ImageVolumeUnsupported  (no support at all)
//   - minor ∈ [31,32] → ImageVolumeAlpha (alpha, feature-gate required)
//   - minor ≥ 33  → condition cleared (beta, no warning needed)
//
// The function never blocks reconciliation: discovery and parse errors are
// logged at V(1) and silently skipped.
func warnIfImageVolumeUnsupported(ctx context.Context, sv serverVersioner, isvc *v1beta1.InferenceService, resolvedMode string) {
	if resolvedMode != kservetypes.OciModelModeNative {
		isvc.Status.ClearCondition(OciImageVolumeCompatible)
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
		isvc.Status.SetCondition(OciImageVolumeCompatible, &apis.Condition{
			Type:   OciImageVolumeCompatible,
			Status: corev1.ConditionFalse,
			Reason: "ImageVolumeUnsupported",
			Message: fmt.Sprintf(
				"Cluster K8s %s.%s does not support ImageVolume (introduced in 1.31). Falling back to modelcar may be required.",
				v.Major, v.Minor),
		})
		return
	}

	if minor < 33 {
		isvc.Status.SetCondition(OciImageVolumeCompatible, &apis.Condition{
			Type:   OciImageVolumeCompatible,
			Status: corev1.ConditionFalse,
			Reason: "ImageVolumeAlpha",
			Message: fmt.Sprintf(
				"Cluster K8s %s.%s has ImageVolume only in alpha (feature-gated). Ensure --feature-gates=ImageVolume=true is set on kube-apiserver and kubelet.",
				v.Major, v.Minor),
		})
		return
	}

	// minor >= 33: beta or later — clear any previously set warning.
	isvc.Status.ClearCondition(OciImageVolumeCompatible)
}
