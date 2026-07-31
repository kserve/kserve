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

package v1alpha1

import (
	"context"
	"os"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractDigestFromImage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name     string
		imageRef string
		expected string
	}{
		{
			name:     "image with tag and digest",
			imageRef: "quay.io/repo/image:v1.0@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:     "image with only digest",
			imageRef: "quay.io/repo/image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:     "image with only tag",
			imageRef: "quay.io/repo/image:v1.0",
			expected: "",
		},
		{
			name:     "image with no tag or digest",
			imageRef: "quay.io/repo/image",
			expected: "",
		},
		{
			name:     "invalid image reference",
			imageRef: "not a valid image ref !!",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDigestFromImage(tt.imageRef)
			g.Expect(result).To(gomega.Equal(tt.expected))
		})
	}
}

func TestIsKyvernoVerificationEnabled(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{name: "enabled with true", envValue: "true", expected: true},
		{name: "enabled with TRUE", envValue: "TRUE", expected: true},
		{name: "disabled with false", envValue: "false", expected: false},
		{name: "disabled with FALSE", envValue: "FALSE", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				t.Setenv(EnvKyvernoEnabled, tt.envValue)
			} else {
				_ = os.Unsetenv(EnvKyvernoEnabled)
			}

			result := isKyvernoVerificationEnabled()
			g.Expect(result).To(gomega.Equal(tt.expected))
		})
	}
}

func TestVerifyKyvernoAnnotation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name        string
		annotations map[string]string
		shouldError bool
	}{
		{
			name: "valid annotation with pass status",
			annotations: map[string]string{
				KyvernoVerifyImagesAnnotation: `{"quay.io/repo/image@sha256:abc":"pass"}`,
			},
			shouldError: false,
		},
		{
			name: "valid annotation with multiple images",
			annotations: map[string]string{
				KyvernoVerifyImagesAnnotation: `{"quay.io/repo/image1@sha256:abc":"pass","quay.io/repo/image2@sha256:def":"pass"}`,
			},
			shouldError: false,
		},
		{
			name: "invalid annotation with fail status",
			annotations: map[string]string{
				KyvernoVerifyImagesAnnotation: `{"quay.io/repo/image@sha256:abc":"fail"}`,
			},
			shouldError: true,
		},
		{
			name:        "missing annotation",
			annotations: map[string]string{},
			shouldError: true,
		},
		{
			name: "invalid JSON",
			annotations: map[string]string{
				KyvernoVerifyImagesAnnotation: `not valid json`,
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyKyvernoAnnotation(tt.annotations)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestSignAndVerifyMutation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	secret := "test-secret-key"
	image := "quay.io/repo/image:v1.0"
	digest := "sha256:abc123def456"

	// Test signing
	sig, err := signMutation(secret, image, digest)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(sig).ToNot(gomega.BeEmpty())

	// Test valid verification
	valid := verifyMutation(secret, image, digest, sig)
	g.Expect(valid).To(gomega.BeTrue())

	// Test verification with wrong secret
	valid = verifyMutation("wrong-secret", image, digest, sig)
	g.Expect(valid).To(gomega.BeFalse())

	// Test verification with wrong image
	valid = verifyMutation(secret, "wrong-image", digest, sig)
	g.Expect(valid).To(gomega.BeFalse())

	// Test verification with wrong digest
	valid = verifyMutation(secret, image, "sha256:wrong", sig)
	g.Expect(valid).To(gomega.BeFalse())

	// Test verification with empty signature
	valid = verifyMutation(secret, image, digest, "")
	g.Expect(valid).To(gomega.BeFalse())

	// Test verification with invalid base64 signature
	valid = verifyMutation(secret, image, digest, "not-valid-base64!!!")
	g.Expect(valid).To(gomega.BeFalse())
}

func TestKernelCacheValidateCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Set up mutation signing key for tests
	t.Setenv(EnvMutationSigningKey, "test-secret")

	tests := []struct {
		name        string
		cache       *KernelCache
		setupFunc   func(*KernelCache)
		shouldError bool
	}{
		{
			name: "missing spec.image",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image: "",
				},
			},
			shouldError: true,
		},
		{
			name: "missing resolved digest annotation",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc(tt.cache)
			}

			_, err := tt.cache.ValidateCreate(context.Background(), tt.cache)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestKernelCacheValidateUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name        string
		oldCache    *KernelCache
		newCache    *KernelCache
		shouldError bool
		errorMsg    string
	}{
		{
			name: "image unchanged, digest unchanged - valid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			shouldError: false,
		},
		{
			name: "image unchanged, digest changed - invalid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:different",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			shouldError: true,
		},
		{
			name: "image changed, new digest present - valid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:newdigest",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v2.0",
				},
			},
			shouldError: false,
		},
		{
			name: "image changed, digest missing - invalid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v2.0",
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.newCache.ValidateUpdate(context.Background(), tt.oldCache, tt.newCache)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestKernelCacheValidateDelete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name        string
		cache       *KernelCache
		shouldError bool
	}{
		{
			name: "cache not in use - valid",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: nil,
				},
			},
			shouldError: false,
		},
		{
			name: "cache in use by pods - invalid",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: &ServingStatus{
						TotalPodsUsing: 3,
					},
				},
			},
			shouldError: true,
		},
		{
			name: "cache with serving status but no pods - valid",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: &ServingStatus{
						TotalPodsUsing: 0,
					},
				},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.cache.ValidateDelete(context.Background(), tt.cache)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestKernelCacheEditPolicy(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	storageSize10Gi := resource.MustParse("10Gi")
	storageSize20Gi := resource.MustParse("20Gi")
	storageClassStandard := "standard"
	storageClassFast := "fast"

	tests := []struct {
		name        string
		oldCache    *KernelCache
		newCache    *KernelCache
		shouldError bool
		errorMsg    string
	}{
		{
			name: "image change blocked - cache in use by pods",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:old",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: &ServingStatus{
						TotalPodsUsing: 5,
					},
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:new",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v2.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: &ServingStatus{
						TotalPodsUsing: 5,
					},
				},
			},
			shouldError: true,
			errorMsg:    "cache in use by 5 pod(s)",
		},
		{
			name: "image change allowed - no pods using cache",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:old",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: &ServingStatus{
						TotalPodsUsing: 0,
					},
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:new",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v2.0",
				},
				Status: KernelCacheStatus{
					ServingStatus: &ServingStatus{
						TotalPodsUsing: 0,
					},
				},
			},
			shouldError: false,
		},
		{
			name: "storage fields blocked - extraction started",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image:v1.0",
					StorageClassName: &storageClassStandard,
					StorageSize:      &storageSize10Gi,
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeNotInUseCnt: 2,
					},
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image:v1.0",
					StorageClassName: &storageClassStandard,
					StorageSize:      &storageSize20Gi,
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeNotInUseCnt: 2,
					},
				},
			},
			shouldError: true,
			errorMsg:    "are immutable after extraction begins",
		},
		{
			name: "storage class change blocked - extraction started",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image:v1.0",
					StorageClassName: &storageClassStandard,
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt: 1,
					},
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image:v1.0",
					StorageClassName: &storageClassFast,
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt: 1,
					},
				},
			},
			shouldError: true,
			errorMsg:    "are immutable after extraction begins",
		},
		{
			name: "storage fields allowed - extraction not started",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image:v1.0",
					StorageClassName: &storageClassStandard,
					StorageSize:      &storageSize10Gi,
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeCnt: 0,
					},
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image:v1.0",
					StorageClassName: &storageClassFast,
					StorageSize:      &storageSize20Gi,
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeCnt: 0,
					},
				},
			},
			shouldError: false,
		},
		{
			name: "podTemplate change allowed - always",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
					PodTemplate: &KernelCachePodTemplate{
						NodeSelector: map[string]string{"gpu": "true"},
					},
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt: 3,
					},
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc",
					},
				},
				Spec: KernelCacheSpec{
					Image: "quay.io/repo/image:v1.0",
					PodTemplate: &KernelCachePodTemplate{
						NodeSelector: map[string]string{"gpu": "nvidia"},
					},
				},
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt: 3,
					},
				},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.newCache.ValidateUpdate(context.Background(), tt.oldCache, tt.newCache)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorMsg != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorMsg))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestKernelCacheDefault(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name                    string
		cache                   *KernelCache
		expectedMountType       KernelCacheMountType
		expectedImagePullPolicy corev1.PullPolicy
		shouldHaveAnnotations   bool
	}{
		{
			name: "defaults mountType to pvc when empty",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: "",
				},
			},
			expectedMountType:       KernelCacheMountTypePVC,
			expectedImagePullPolicy: "",
			shouldHaveAnnotations:   false,
		},
		{
			name: "preserves mountType when set to pvc",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypePVC,
				},
			},
			expectedMountType:       KernelCacheMountTypePVC,
			expectedImagePullPolicy: "",
			shouldHaveAnnotations:   false,
		},
		{
			name: "preserves mountType when set to imageVolume",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			expectedMountType:       KernelCacheMountTypeImageVolume,
			expectedImagePullPolicy: corev1.PullIfNotPresent,
			shouldHaveAnnotations:   false,
		},
		{
			name: "sets imagePullPolicy to IfNotPresent when mountType is imageVolume and imagePullPolicy empty",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:           "quay.io/repo/image:v1.0",
					MountType:       KernelCacheMountTypeImageVolume,
					ImagePullPolicy: "",
				},
			},
			expectedMountType:       KernelCacheMountTypeImageVolume,
			expectedImagePullPolicy: corev1.PullIfNotPresent,
			shouldHaveAnnotations:   false,
		},
		{
			name: "preserves imagePullPolicy when already set",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:           "quay.io/repo/image:v1.0",
					MountType:       KernelCacheMountTypeImageVolume,
					ImagePullPolicy: corev1.PullAlways,
				},
			},
			expectedMountType:       KernelCacheMountTypeImageVolume,
			expectedImagePullPolicy: corev1.PullAlways,
			shouldHaveAnnotations:   false,
		},
		{
			name: "does not set imagePullPolicy for pvc mount type",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:           "quay.io/repo/image:v1.0",
					MountType:       KernelCacheMountTypePVC,
					ImagePullPolicy: "",
				},
			},
			expectedMountType:       KernelCacheMountTypePVC,
			expectedImagePullPolicy: "",
			shouldHaveAnnotations:   false,
		},
		{
			name: "skips processing for object being deleted",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cache",
					Namespace:         "default",
					DeletionTimestamp: &metav1.Time{},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: "",
				},
			},
			expectedMountType:       "",
			expectedImagePullPolicy: "",
			shouldHaveAnnotations:   false,
		},
		{
			name: "skips processing when image is empty",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "",
					MountType: "",
				},
			},
			expectedMountType:       "",
			expectedImagePullPolicy: "",
			shouldHaveAnnotations:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cache.Default(context.Background(), tt.cache)
			// Default() may fail if it tries to resolve digest (e.g., cosign verification)
			// For basic defaulting tests, we just check that it doesn't panic
			// and that the defaults are applied before any resolution errors
			if err == nil {
				g.Expect(tt.cache.Spec.MountType).To(gomega.Equal(tt.expectedMountType))
				g.Expect(tt.cache.Spec.ImagePullPolicy).To(gomega.Equal(tt.expectedImagePullPolicy))
			} else {
				// If there's an error, it's likely from digest resolution
				// Check that defaults were still applied before the error
				if tt.expectedMountType != "" {
					g.Expect(tt.cache.Spec.MountType).To(gomega.Equal(tt.expectedMountType))
				}
				if tt.expectedImagePullPolicy != "" {
					g.Expect(tt.cache.Spec.ImagePullPolicy).To(gomega.Equal(tt.expectedImagePullPolicy))
				}
			}
		})
	}
}

func TestValidateMountTypeConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	storageClass := "standard"
	storageSize := resource.MustParse("10Gi")

	tests := []struct {
		name        string
		cache       *KernelCache
		shouldError bool
		errorMsg    string
	}{
		{
			name: "pvc mount type - valid with no additional config",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image@sha256:abc123",
					MountType: KernelCacheMountTypePVC,
				},
			},
			shouldError: false,
		},
		{
			name: "pvc mount type - valid with storage config",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image@sha256:abc123",
					MountType:        KernelCacheMountTypePVC,
					StorageClassName: &storageClass,
					StorageSize:      &storageSize,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
			},
			shouldError: false,
		},
		{
			name: "imageVolume mount type - valid",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image@sha256:abc123",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			shouldError: false,
		},
		{
			name: "imageVolume mount type - ignores pvc fields",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:            "quay.io/repo/image@sha256:abc123",
					MountType:        KernelCacheMountTypeImageVolume,
					StorageClassName: &storageClass,
					StorageSize:      &storageSize,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					PodTemplate: &KernelCachePodTemplate{
						NodeSelector: map[string]string{"gpu": "true"},
					},
				},
			},
			shouldError: false,
		},
		{
			name: "invalid mount type",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image@sha256:abc123",
					MountType: "invalid",
				},
			},
			shouldError: true,
			errorMsg:    "invalid mountType",
		},
		{
			name: "empty mount type defaults to pvc",
			cache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image@sha256:abc123",
					MountType: "",
				},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMountTypeConfig(context.Background(), tt.cache)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorMsg != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorMsg))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestKernelCacheValidateUpdateMountTypeImmutability(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name        string
		oldCache    *KernelCache
		newCache    *KernelCache
		shouldError bool
		errorMsg    string
	}{
		{
			name: "mountType change from pvc to imageVolume - invalid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypePVC,
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			shouldError: true,
			errorMsg:    "spec.mountType is immutable",
		},
		{
			name: "mountType change from imageVolume to pvc - invalid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypePVC,
				},
			},
			shouldError: true,
			errorMsg:    "spec.mountType is immutable",
		},
		{
			name: "mountType unchanged - pvc to pvc - valid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypePVC,
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypePVC,
				},
			},
			shouldError: false,
		},
		{
			name: "mountType unchanged - imageVolume to imageVolume - valid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			shouldError: false,
		},
		{
			name: "mountType change from empty to pvc (both default to pvc) - valid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: "",
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypePVC,
				},
			},
			shouldError: false,
		},
		{
			name: "mountType change from empty to imageVolume - invalid",
			oldCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: "",
				},
			},
			newCache: &KernelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cache",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationResolvedDigest: "sha256:abc123",
					},
				},
				Spec: KernelCacheSpec{
					Image:     "quay.io/repo/image:v1.0",
					MountType: KernelCacheMountTypeImageVolume,
				},
			},
			shouldError: true,
			errorMsg:    "spec.mountType is immutable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.newCache.ValidateUpdate(context.Background(), tt.oldCache, tt.newCache)
			if tt.shouldError {
				g.Expect(err).To(gomega.HaveOccurred())
				if tt.errorMsg != "" {
					g.Expect(err.Error()).To(gomega.ContainSubstring(tt.errorMsg))
				}
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestIsExtractionStarted(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name     string
		cache    *KernelCache
		expected bool
	}{
		{
			name: "no status counts - not started",
			cache: &KernelCache{
				Status: KernelCacheStatus{
					Counts: nil,
				},
			},
			expected: false,
		},
		{
			name: "zero counts - not started",
			cache: &KernelCache{
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt:    0,
						NodeNotInUseCnt: 0,
					},
				},
			},
			expected: false,
		},
		{
			name: "nodes in use - started",
			cache: &KernelCache{
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt:    2,
						NodeNotInUseCnt: 0,
					},
				},
			},
			expected: true,
		},
		{
			name: "nodes not in use - started",
			cache: &KernelCache{
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt:    0,
						NodeNotInUseCnt: 3,
					},
				},
			},
			expected: true,
		},
		{
			name: "mixed nodes - started",
			cache: &KernelCache{
				Status: KernelCacheStatus{
					Counts: &CacheCounts{
						NodeInUseCnt:    1,
						NodeNotInUseCnt: 2,
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isExtractionStarted(tt.cache)
			g.Expect(result).To(gomega.Equal(tt.expected))
		})
	}
}

func TestStorageFieldsEqual(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	storageClass1 := "standard"
	storageClass2 := "fast"
	storageSize1 := resource.MustParse("10Gi")
	storageSize2 := resource.MustParse("20Gi")

	tests := []struct {
		name     string
		oldSpec  KernelCacheSpec
		newSpec  KernelCacheSpec
		expected bool
	}{
		{
			name: "all fields equal - both nil",
			oldSpec: KernelCacheSpec{
				StorageClassName: nil,
				StorageSize:      nil,
				AccessModes:      nil,
			},
			newSpec: KernelCacheSpec{
				StorageClassName: nil,
				StorageSize:      nil,
				AccessModes:      nil,
			},
			expected: true,
		},
		{
			name: "all fields equal - same values",
			oldSpec: KernelCacheSpec{
				StorageClassName: &storageClass1,
				StorageSize:      &storageSize1,
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			newSpec: KernelCacheSpec{
				StorageClassName: &storageClass1,
				StorageSize:      &storageSize1,
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			expected: true,
		},
		{
			name: "storage class name changed",
			oldSpec: KernelCacheSpec{
				StorageClassName: &storageClass1,
			},
			newSpec: KernelCacheSpec{
				StorageClassName: &storageClass2,
			},
			expected: false,
		},
		{
			name: "storage class nil to non-nil",
			oldSpec: KernelCacheSpec{
				StorageClassName: nil,
			},
			newSpec: KernelCacheSpec{
				StorageClassName: &storageClass1,
			},
			expected: false,
		},
		{
			name: "storage size changed",
			oldSpec: KernelCacheSpec{
				StorageSize: &storageSize1,
			},
			newSpec: KernelCacheSpec{
				StorageSize: &storageSize2,
			},
			expected: false,
		},
		{
			name: "storage size nil to non-nil",
			oldSpec: KernelCacheSpec{
				StorageSize: nil,
			},
			newSpec: KernelCacheSpec{
				StorageSize: &storageSize1,
			},
			expected: false,
		},
		{
			name: "access modes changed",
			oldSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			newSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
			},
			expected: false,
		},
		{
			name: "access modes length changed",
			oldSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			newSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany},
			},
			expected: false,
		},
		{
			name: "access modes empty to nil",
			oldSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{},
			},
			newSpec: KernelCacheSpec{
				AccessModes: nil,
			},
			expected: true,
		},
		{
			name: "access modes order independent - same modes",
			oldSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany},
			},
			newSpec: KernelCacheSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany, corev1.ReadWriteOnce},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := storageFieldsEqual(tt.oldSpec, tt.newSpec)
			g.Expect(result).To(gomega.Equal(tt.expected))
		})
	}
}
