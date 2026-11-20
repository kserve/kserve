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

package reconcilers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// reconcileDownloadVolumes attempts every group's PV and PVC so a failure in one
// group does not stop other downloads. The caller returns the errors to the work queue.
func reconcileDownloadVolumes(
	ctx context.Context, clientset *kubernetes.Clientset, scheme *runtime.Scheme, log logr.Logger,
	cache *v1alpha1.LocalModelCache, namespaceCache *v1alpha1.LocalModelNamespaceCache,
	nodeGroups map[string]*v1alpha1.LocalModelNodeGroup, jobNamespace string,
) error {
	model := ExtractLocalModelParams(cache, namespaceCache)
	var errs []error
	for _, group := range nodeGroups {
		claimName := model.Name + "-" + group.Name
		pvName := claimName + "-download"
		if model.IsNamespaceScoped {
			pvName = claimName + "-" + model.Namespace + "-download"
			claimName = pvName
		}

		pv := corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: pvName},
			Spec:       group.Spec.PersistentVolumeSpec,
		}
		if err := CreatePV(ctx, clientset, scheme, log, pv, cache, namespaceCache); err != nil {
			errs = append(errs, fmt.Errorf("download PV %s: %w", pvName, err))
		}
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: claimName},
			Spec:       group.Spec.PersistentVolumeClaimSpec,
		}
		pvc.Spec.VolumeName = pvName
		if err := CreatePVC(ctx, clientset, scheme, log, pvc, jobNamespace, cache, namespaceCache); err != nil {
			errs = append(errs, fmt.Errorf("download PVC %s/%s: %w", jobNamespace, claimName, err))
		}
	}
	return errors.Join(errs...)
}
