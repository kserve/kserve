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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// CreatePV creates a PersistentVolume and optionally sets the owner reference
// PVs are cluster-scoped, so owner can only be cluster-scoped resources (or nil)
func CreatePV(
	ctx context.Context,
	clientset kubernetes.Interface,
	scheme *runtime.Scheme,
	log logr.Logger,
	spec corev1.PersistentVolume,
	kc *v1alpha1.KernelCache,
) error {
	persistentVolumes := clientset.CoreV1().PersistentVolumes()
	if _, err := persistentVolumes.Get(ctx, spec.Name, metav1.GetOptions{}); err != nil {
		if !apierr.IsNotFound(err) {
			log.Error(err, "Failed to get PV")
			return err
		}
		log.Info("Create PV", "name", spec.Name)

		// Only set controller reference if owner is provided and cluster-scoped
		// For namespace-scoped KernelCache, use labels instead (kc will be nil)
		if kc != nil {
			if err := controllerutil.SetControllerReference(kc, &spec, scheme); err != nil {
				log.Error(err, "Failed to set controller reference")
				return err
			}
		}

		if _, err := persistentVolumes.Create(ctx, &spec, metav1.CreateOptions{}); err != nil {
			log.Error(err, "Failed to create PV", "name", spec.Name)
			return err
		}
	}
	return nil
}

// CreatePVC creates a PersistentVolumeClaim and optionally sets the owner reference to the KernelCache
func CreatePVC(
	ctx context.Context,
	clientset kubernetes.Interface,
	scheme *runtime.Scheme,
	log logr.Logger,
	spec corev1.PersistentVolumeClaim,
	namespace string,
	kc *v1alpha1.KernelCache,
) error {
	persistentVolumeClaims := clientset.CoreV1().PersistentVolumeClaims(namespace)
	if _, err := persistentVolumeClaims.Get(ctx, spec.Name, metav1.GetOptions{}); err != nil {
		if !apierr.IsNotFound(err) {
			log.Error(err, "Failed to get PVC")
			return err
		}
		log.Info("Create PVC", "name", spec.Name, "namespace", namespace)

		spec.Namespace = namespace

		// Set controller reference only if in same namespace
		if kc != nil && kc.Namespace == namespace {
			if err := controllerutil.SetControllerReference(kc, &spec, scheme); err != nil {
				log.Error(err, "Set controller reference")
				return err
			}
		}

		if _, err := persistentVolumeClaims.Create(ctx, &spec, metav1.CreateOptions{}); err != nil {
			log.Error(err, "Failed to create PVC", "name", spec.Name)
			return err
		}
	}
	return nil
}
