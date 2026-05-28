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

package reconcilers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestCreatePV(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	t.Run("creates PV when it doesn't exist", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		ctx := context.Background()
		log := logr.Discard()

		pvSpec := corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp/test",
					},
				},
			},
		}

		err := CreatePV(ctx, clientset, scheme, log, pvSpec, nil)
		if err != nil {
			t.Fatalf("CreatePV failed: %v", err)
		}

		pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, "test-pv", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get created PV: %v", err)
		}
		if pv.Name != "test-pv" {
			t.Errorf("expected PV name test-pv, got %s", pv.Name)
		}
	})

	t.Run("does not fail when PV already exists", func(t *testing.T) {
		existingPV := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp/existing",
					},
				},
			},
		}

		clientset := fake.NewSimpleClientset(existingPV)
		ctx := context.Background()
		log := logr.Discard()

		pvSpec := *existingPV

		err := CreatePV(ctx, clientset, scheme, log, pvSpec, nil)
		if err != nil {
			t.Fatalf("CreatePV should not fail when PV exists: %v", err)
		}
	})
}

func TestCreatePVC(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	t.Run("creates PVC when it doesn't exist", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		ctx := context.Background()
		log := logr.Discard()

		pvcSpec := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pvc",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		}

		err := CreatePVC(ctx, clientset, scheme, log, pvcSpec, "default", nil)
		if err != nil {
			t.Fatalf("CreatePVC failed: %v", err)
		}

		pvc, err := clientset.CoreV1().PersistentVolumeClaims("default").Get(ctx, "test-pvc", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get created PVC: %v", err)
		}
		if pvc.Name != "test-pvc" {
			t.Errorf("expected PVC name test-pvc, got %s", pvc.Name)
		}
		if pvc.Namespace != "default" {
			t.Errorf("expected PVC namespace default, got %s", pvc.Namespace)
		}
	})

	t.Run("does not fail when PVC already exists", func(t *testing.T) {
		existingPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		}

		clientset := fake.NewSimpleClientset(existingPVC)
		ctx := context.Background()
		log := logr.Discard()

		pvcSpec := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-pvc",
			},
			Spec: existingPVC.Spec,
		}

		err := CreatePVC(ctx, clientset, scheme, log, pvcSpec, "default", nil)
		if err != nil {
			t.Fatalf("CreatePVC should not fail when PVC exists: %v", err)
		}
	})

	t.Run("sets namespace on PVC spec", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		ctx := context.Background()
		log := logr.Discard()

		pvcSpec := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "namespaced-pvc",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		}

		err := CreatePVC(ctx, clientset, scheme, log, pvcSpec, "test-namespace", nil)
		if err != nil {
			t.Fatalf("CreatePVC failed: %v", err)
		}

		pvc, err := clientset.CoreV1().PersistentVolumeClaims("test-namespace").Get(ctx, "namespaced-pvc", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get created PVC: %v", err)
		}
		if pvc.Namespace != "test-namespace" {
			t.Errorf("expected PVC namespace test-namespace, got %s", pvc.Namespace)
		}
	})
}
