//go:build distro

/*
Copyright 2026 The KServe Authors.

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

package localmodelnode

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

const MountPath = "/var/lib/kserve"

func enhanceDownloadJob(_ *batchv1.Job, _ string) error { return nil }

func ensureModelRootFolderExistsAndIsWritable(_ context.Context, _ *LocalModelNodeReconciler,
	_ *v1beta1.LocalModelConfig,
) (*ensureModelRootFolderResult, error) {
	if err := fsHelper.ensureModelRootFolderExists(); err != nil {
		return nil, fmt.Errorf("failed to ensure model root folder: %w", err)
	}
	return &ensureModelRootFolderResult{Continue: true}, nil
}
