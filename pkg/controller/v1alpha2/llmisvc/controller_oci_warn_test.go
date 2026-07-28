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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/version"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	kservetypes "github.com/kserve/kserve/pkg/types"
)

// staticLLMVersioner is a minimal serverVersioner stub for LLMInferenceService tests.
type staticLLMVersioner struct {
	v   *version.Info
	err error
}

func (s *staticLLMVersioner) ServerVersion() (*version.Info, error) { return s.v, s.err }

func makeLLMSvc() *v1alpha2.LLMInferenceService {
	svc := &v1alpha2.LLMInferenceService{}
	svc.GetConditionSet().Manage(svc.GetStatus()).InitializeConditions()
	return svc
}

func TestLLMWarnIfImageVolumeUnsupported(t *testing.T) {
	tests := []struct {
		name         string
		minor        string
		mode         string
		discoveryErr bool
		wantCond     bool
		wantReason   string
	}{
		// K8s ≥ 1.35: beta defaults-on — no warning needed.
		{
			name:     "k8s 1.35 native: beta defaults-on, no condition",
			minor:    "35",
			mode:     kservetypes.OciModelModeNative,
			wantCond: false,
		},
		// K8s 1.33–1.34: beta, feature gate required, subPath supported.
		{
			name:       "k8s 1.34 native: beta feature-gated, ImageVolumeAlpha condition",
			minor:      "34",
			mode:       kservetypes.OciModelModeNative,
			wantCond:   true,
			wantReason: "ImageVolumeAlpha",
		},
		{
			name:       "k8s 1.33 native: beta feature-gated, ImageVolumeAlpha condition",
			minor:      "33",
			mode:       kservetypes.OciModelModeNative,
			wantCond:   true,
			wantReason: "ImageVolumeAlpha",
		},
		// K8s 1.31–1.32: alpha, feature gate required, subPath forbidden.
		{
			name:       "k8s 1.32 native: alpha, subPath forbidden, ImageVolumeSubPathUnsupported condition",
			minor:      "32",
			mode:       kservetypes.OciModelModeNative,
			wantCond:   true,
			wantReason: "ImageVolumeSubPathUnsupported",
		},
		{
			name:       "k8s 1.31 native: alpha, subPath forbidden, ImageVolumeSubPathUnsupported condition",
			minor:      "31",
			mode:       kservetypes.OciModelModeNative,
			wantCond:   true,
			wantReason: "ImageVolumeSubPathUnsupported",
		},
		// K8s < 1.31: not supported.
		{
			name:       "k8s 1.30 native: unsupported, ImageVolumeUnsupported condition",
			minor:      "30",
			mode:       kservetypes.OciModelModeNative,
			wantCond:   true,
			wantReason: "ImageVolumeUnsupported",
		},
		// Non-native mode: no condition.
		{
			name:     "k8s 1.30 modelcar: mode is not native, no condition",
			minor:    "30",
			mode:     kservetypes.OciModelModeModelcar,
			wantCond: false,
		},
		// Discovery failure: skip silently.
		{
			name:         "discovery error: skip silently, no condition",
			discoveryErr: true,
			mode:         kservetypes.OciModelModeNative,
			wantCond:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := makeLLMSvc()

			var sv serverVersioner
			if tc.discoveryErr {
				sv = &staticLLMVersioner{err: errors.New("discovery unreachable")}
			} else {
				sv = &staticLLMVersioner{v: &version.Info{Major: "1", Minor: tc.minor}}
			}

			warnIfImageVolumeUnsupported(context.Background(), sv, svc, tc.mode)

			cond := svc.GetStatus().GetCondition(ociImageVolumeCompatible)
			if tc.wantCond {
				require.NotNil(t, cond, "expected OciImageVolumeCompatible condition to be set")
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, tc.wantReason, cond.Reason)
			} else {
				assert.Nil(t, cond, "expected no OciImageVolumeCompatible condition")
			}
		})
	}
}
