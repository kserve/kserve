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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestJobFailed(t *testing.T) {
	reconciler := &KernelCacheReconciler{
		Log: logr.Discard(),
	}

	t.Run("returns true when job has failed condition", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if !reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return true for failed job")
		}
	})

	t.Run("returns false when job has not failed", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return false for completed job")
		}
	})

	t.Run("returns false when job has no conditions", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		if reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return false for job with no conditions")
		}
	})

	t.Run("returns false when job failed condition is false", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		if reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return false when failed condition status is false")
		}
	})
}

func TestJobCompleted(t *testing.T) {
	reconciler := &KernelCacheReconciler{
		Log: logr.Discard(),
	}

	t.Run("returns true when job has complete condition", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if !reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return true for completed job")
		}
	})

	t.Run("returns false when job has not completed", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false for failed job")
		}
	})

	t.Run("returns false when job has no conditions", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false for job with no conditions")
		}
	})

	t.Run("returns false when job complete condition is false", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false when complete condition status is false")
		}
	})
}

func TestExtractionComplete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	t.Run("returns true when extraction job is completed", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extract-test-cache-abc123",
				Namespace: "kserve",
				Labels: map[string]string{
					"cache":           "test-cache",
					"cache-namespace": "default",
					"app":             "kernel-cache-extract",
				},
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, job).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		if !reconciler.extractionComplete(context.TODO(), kc, "kserve") {
			t.Error("expected extractionComplete to return true for completed job")
		}
	})

	t.Run("returns false when extraction job is not completed", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extract-test-cache-abc123",
				Namespace: "kserve",
				Labels: map[string]string{
					"cache":           "test-cache",
					"cache-namespace": "default",
					"app":             "kernel-cache-extract",
				},
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, job).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		if reconciler.extractionComplete(context.TODO(), kc, "kserve") {
			t.Error("expected extractionComplete to return false for non-completed job")
		}
	})

	t.Run("returns false when extraction job does not exist", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		if reconciler.extractionComplete(context.TODO(), kc, "kserve") {
			t.Error("expected extractionComplete to return false when job does not exist")
		}
	})
}
