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
