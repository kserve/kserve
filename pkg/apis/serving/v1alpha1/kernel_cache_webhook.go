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

// Webhook implementation adapted from GKM (github.com/redhat-et/GKM)
// api/v1alpha1/gkmcache_webhook.go

package v1alpha1

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/google/go-containerregistry/pkg/authn"
	gcrname "github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	gcrremote "github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/kserve/kserve/pkg/cosign"
)

// Webhook annotation keys (KServe namespaced)
const (
	// AnnotationResolvedDigest stores the resolved sha256 digest of the OCI image
	// Set by mutating webhook to ensure deterministic extraction
	AnnotationResolvedDigest = "internal.serving.kserve.io/resolved-digest"

	// AnnotationCacheSizeBytes stores the total uncompressed size of kernel cache in bytes
	// Extracted from OCI image labels during mutation
	AnnotationCacheSizeBytes = "internal.serving.kserve.io/cache-size-bytes"

	// AnnotationDigestError stores digest resolution error for debugging
	// Set when digest resolution fails (non-fatal in mutating webhook)
	AnnotationDigestError = "internal.serving.kserve.io/digest-error"

	// KyvernoVerifyImagesAnnotation is the Kyverno annotation that contains verification status
	// Format: {"<image>@<digest>":"pass"}
	KyvernoVerifyImagesAnnotation = "kyverno.io/verify-images"

	// ImageLabelCacheSizeBytesSubstring is the substring in OCI image labels that contains cache size
	// GKM's mcv tool sets labels like "io.kserve.cache-size-bytes.<layer>" with size values
	ImageLabelCacheSizeBytesSubstring = "cache-size-bytes"

	// EnvKyvernoEnabled is the environment variable to enable/disable Kyverno verification
	// Defaults to false if not set
	EnvKyvernoEnabled = "KYVERNO_VERIFICATION_ENABLED"

	// AnnotationMutationSig stores HMAC signature binding digest to mutating webhook
	// Prevents users from injecting arbitrary digests - only mutating webhook can set valid digest
	AnnotationMutationSig = "internal.serving.kserve.io/mutation-sig"

	// EnvMutationSigningKey is the environment variable containing the HMAC secret
	// Used to sign/verify mutation annotations to prevent tampering
	EnvMutationSigningKey = "MUTATION_SIGNING_KEY"

	// ImageVerificationTimeout is the timeout for cosign signature verification
	// Set to 30 seconds to accommodate v3 bundle verification which can take 15-20 seconds
	ImageVerificationTimeout = 30 * time.Second
)

var (
	kernelcacheLog                         = logf.Log.WithName("kernelcache-webhook")
	_              webhook.CustomDefaulter = &KernelCache{}
	_              webhook.CustomValidator = &KernelCache{}
)

// SetupWebhookWithManager registers the webhook with the controller-runtime manager
func (kc *KernelCache) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(kc).
		WithDefaulter(kc).
		WithValidator(kc).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-serving-kserve-io-v1alpha1-kernelcache,mutating=true,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=kernelcaches,verbs=create;update,versions=v1alpha1,name=kernelcache.kserve-webhook-server.defaulter,admissionReviewVersions=v1,reinvocationPolicy=Never
// +kubebuilder:webhook:path=/validate-serving-kserve-io-v1alpha1-kernelcache,mutating=false,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=kernelcaches,verbs=create;update;delete,versions=v1alpha1,name=kernelcache.kserve-webhook-server.validator,admissionReviewVersions=v1

// Default implements webhook.CustomDefaulter (mutating webhook)
// Resolves OCI image tag to sha256 digest and extracts cache size from image labels
func (kc *KernelCache) Default(ctx context.Context, obj runtime.Object) error {
	kernelcacheLog.V(1).Info("Mutating webhook called", "object", obj)

	cache, ok := obj.(*KernelCache)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected KernelCache, got %T", obj))
	}
	kernelcacheLog.V(1).Info("Decoded KernelCache object", "name", cache.Name, "namespace", cache.Namespace)

	if cache.Annotations == nil {
		cache.Annotations = map[string]string{}
	}

	if cache.Spec.Image == "" {
		kernelcacheLog.Info("spec.image is empty, skipping")
		return nil
	}

	// Resolve & verify image -> digest with appropriate timeout
	verifyCtx, cancel := context.WithTimeout(context.Background(), ImageVerificationTimeout)
	defer cancel()

	kyvernoEnabled := isKyvernoVerificationEnabled()
	var digest string
	var err error

	if kyvernoEnabled {
		// Kyverno mode: extract digest from image (Kyverno adds it via ImageVerificationPolicy)
		// First check if the image already contains a digest (e.g., from Kyverno mutation)
		if extractedDigest := extractDigestFromImage(cache.Spec.Image); extractedDigest != "" {
			kernelcacheLog.Info("Image already contains digest (likely from Kyverno)", "image", cache.Spec.Image, "digest", extractedDigest)
			digest = extractedDigest
		}

		resolvedDigest, digestFound := cache.Annotations[AnnotationResolvedDigest]
		if digestFound && digest != "" {
			// Digest hasn't changed so just return
			if digest == resolvedDigest {
				return nil
			}
		}

		// If digest is empty when Kyverno is enabled, skip setting annotation
		// The webhook will be reinvoked after Kyverno adds the digest (reinvocationPolicy: IfNeeded)
		if digest == "" {
			kernelcacheLog.V(1).Info("Digest is empty, skipping annotation update (waiting for Kyverno)")
			return nil
		}
	} else {
		// Non-Kyverno mode: verify cosign signature and resolve digest
		// This ensures image authenticity when Kyverno is not available
		kernelcacheLog.V(1).Info("Verifying image signature with cosign (Kyverno disabled)", "image", cache.Spec.Image)
		digest, err = cosign.VerifyImageSignature(verifyCtx, cache.Spec.Image)
		if err != nil {
			kernelcacheLog.Error(err, "failed to verify image signature")
			return apierrors.NewBadRequest(fmt.Sprintf(
				"image signature verification failed for '%s': %s",
				cache.Spec.Image, err.Error(),
			))
		}
	}

	// Extract cache size from OCI image labels
	size := extractSizeFromImage(cache.Spec.Image)
	kernelcacheLog.Info("Extracted cache size", "bytes", size, "MB", float64(size)/(1024*1024))

	// Store digest and size in annotations
	cache.Annotations[AnnotationResolvedDigest] = digest
	cache.Annotations[AnnotationCacheSizeBytes] = strconv.FormatInt(size, 10)

	// Generate mutation signature to prevent digest tampering
	// Only the mutating webhook can create a valid signature
	secret, err := mutationKeyFromEnv()
	if err != nil {
		kernelcacheLog.Error(err, "MUTATION_SIGNING_KEY not configured")
		return apierrors.NewBadRequest(fmt.Sprintf("webhook configuration error: %s", err.Error()))
	}

	// Get admission request (not used in signature, but API requires it for context)
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return apierrors.NewBadRequest("unable to read admission request from context")
	}

	sig, err := signMutation(secret, string(req.UID), cache.Spec.Image, digest)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("failed to sign mutation: %v", err))
	}
	cache.Annotations[AnnotationMutationSig] = sig

	kernelcacheLog.Info("added/updated resolvedDigest", "image", cache.Spec.Image, "digest", digest)
	return nil
}

// ValidateCreate implements webhook.CustomValidator for CREATE operations
func (kc *KernelCache) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cache, ok := obj.(*KernelCache)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected KernelCache, got %T", obj))
	}

	kernelcacheLog.Info("Validating KernelCache create", "name", cache.Name, "namespace", cache.Namespace)

	// Validate required fields
	if cache.Spec.Image == "" {
		return nil, errors.New("spec.image must be set")
	}

	// Ensure mutating webhook set the digest annotation
	digest := cache.Annotations[AnnotationResolvedDigest]
	sig := cache.Annotations[AnnotationMutationSig]

	if digest == "" {
		return nil, fmt.Errorf("%s must be set by mutating webhook", AnnotationResolvedDigest)
	}

	// Verify mutation signature to ensure digest was set by mutating webhook
	// This prevents users from injecting arbitrary digests
	secret, err := mutationKeyFromEnv()
	if err != nil {
		return nil, fmt.Errorf("webhook configuration error: %s", err.Error())
	}

	// Get admission request (not used in signature, but API requires it for context)
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to read admission request from context")
	}

	if !verifyMutation(secret, string(req.UID), cache.Spec.Image, digest, sig) {
		return nil, fmt.Errorf("%s present but missing/invalid %s; digest must be set only by the mutating webhook",
			AnnotationResolvedDigest, AnnotationMutationSig)
	}

	kernelcacheLog.V(1).Info("Mutation signature validated", "image", cache.Spec.Image, "digest", digest)

	// If Kyverno is enabled, verify the signature was validated
	if isKyvernoVerificationEnabled() {
		if _, exists := cache.Annotations[KyvernoVerifyImagesAnnotation]; !exists {
			return nil, fmt.Errorf("%s must be set by Kyverno", KyvernoVerifyImagesAnnotation)
		}

		// Check Kyverno verification status
		if err := verifyKyvernoAnnotation(cache.Annotations); err != nil {
			return nil, fmt.Errorf("Kyverno verification failed: %w", err)
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator for UPDATE operations
func (kc *KernelCache) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	kernelcacheLog.V(1).Info("Validating webhook called", "oldObj", oldObj, "newObj", newObj)

	oldCache, ok1 := oldObj.(*KernelCache)
	newCache, ok2 := newObj.(*KernelCache)
	if !ok1 || !ok2 {
		return nil, apierrors.NewBadRequest("type assertion to KernelCache failed")
	}

	oldImg := oldCache.Spec.Image
	newImg := newCache.Spec.Image

	oldDigest := oldCache.Annotations[AnnotationResolvedDigest]
	newDigest := newCache.Annotations[AnnotationResolvedDigest]
	oldSize := oldCache.Annotations[AnnotationCacheSizeBytes]
	newSize := newCache.Annotations[AnnotationCacheSizeBytes]

	// If image didn't change, digest must not change
	if oldImg == newImg {
		if oldDigest != newDigest {
			kernelcacheLog.Info("Digests don't match", "oldDigest", oldDigest, "newDigest", newDigest, "oldSize", oldSize, "newSize", newSize)
			return nil, fmt.Errorf("%s is immutable when spec.image is unchanged", AnnotationResolvedDigest)
		}
		return nil, nil
	}

	// Image changed -> new digest must be present
	if newImg == "" {
		return nil, errors.New("spec.image must be set")
	}
	if newDigest == "" {
		return nil, fmt.Errorf("%s must be set by mutating webhook when spec.image changes", AnnotationResolvedDigest)
	}

	// Validate Kyverno verification if enabled
	if isKyvernoVerificationEnabled() {
		if _, exists := newCache.Annotations[KyvernoVerifyImagesAnnotation]; !exists {
			return nil, fmt.Errorf("%s must be set by Kyverno", KyvernoVerifyImagesAnnotation)
		}

		if err := verifyKyvernoAnnotation(newCache.Annotations); err != nil {
			return nil, fmt.Errorf("Kyverno verification failed: %w", err)
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator for DELETE operations
func (kc *KernelCache) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cache, ok := obj.(*KernelCache)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected KernelCache, got %T", obj))
	}

	kernelcacheLog.Info("Validating KernelCache delete", "name", cache.Name, "namespace", cache.Namespace)

	// Check if cache is in use by pods (via ServingStatus)
	// Phase 2: when ServingStatus is populated by agent
	// For now, allow deletion (finalizer handles cleanup)
	if cache.Status.ServingStatus != nil && cache.Status.ServingStatus.TotalPods > 0 {
		return nil, fmt.Errorf("cannot delete KernelCache: in use by %d pods", cache.Status.ServingStatus.TotalPods)
	}

	return nil, nil
}

// extractDigestFromImage extracts the digest from an image reference if it contains one.
// Returns empty string if the image reference doesn't contain a digest.
// Example: "quay.io/repo/image:tag@sha256:abc123" -> "sha256:abc123"
func extractDigestFromImage(imageRef string) string {
	ref, err := gcrname.ParseReference(imageRef)
	if err != nil {
		return ""
	}

	// Identifier() returns the digest if present, otherwise the tag
	identifier := ref.Identifier()
	// Check if it's actually a digest (starts with sha256:)
	if len(identifier) > 7 && identifier[:7] == "sha256:" {
		return identifier
	}

	return ""
}

// extractSizeFromImage walks the layers in the image and adds the sizes
// of all layers with cache-size-bytes labels.
// Returns 0 if unable to calculate the size.
// Note: This requires reading OCI image metadata from registry
func extractSizeFromImage(imageRef string) int64 {
	var totalUncompressedSize int64

	ref, err := gcrname.ParseReference(imageRef)
	if err != nil {
		kernelcacheLog.Error(err, "ParseReference failed")
		return totalUncompressedSize
	}

	img, err := remote.Image(ref)
	if err != nil {
		kernelcacheLog.Error(err, "remote.Image failed")
		return totalUncompressedSize
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		kernelcacheLog.Error(err, "ConfigFile failed")
		return totalUncompressedSize
	}

	labels := cfg.Config.Labels
	if len(labels) == 0 {
		kernelcacheLog.V(1).Info("No labels found in image")
		return totalUncompressedSize
	}

	// Look for labels containing cache-size-bytes substring
	for key, value := range labels {
		if strings.Contains(key, ImageLabelCacheSizeBytesSubstring) {
			size, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				kernelcacheLog.Error(err, "ParseInt failed", "label", key, "value", value)
			} else {
				kernelcacheLog.Info("Found cache size label", "label", key, "bytes", value)
				totalUncompressedSize += size
			}
		}
	}

	return totalUncompressedSize
}

// verifyKyvernoAnnotation checks the kyverno.io/verify-images annotation to ensure
// the image signature was verified by Kyverno and the status is "pass".
// The annotation format is: {"<image>@<digest>":"pass"}
func verifyKyvernoAnnotation(annotations map[string]string) error {
	kyvernoAnnotation, exists := annotations[KyvernoVerifyImagesAnnotation]
	if !exists {
		return fmt.Errorf("failed to find %s annotation", KyvernoVerifyImagesAnnotation)
	}

	// Parse the JSON annotation
	var verifications map[string]string
	if err := json.Unmarshal([]byte(kyvernoAnnotation), &verifications); err != nil {
		return fmt.Errorf("failed to parse %s annotation: %w", KyvernoVerifyImagesAnnotation, err)
	}

	// Check if all entries have status "pass"
	for imageRef, status := range verifications {
		if status != "pass" {
			return fmt.Errorf("Kyverno verification status for %s is not 'pass': %s", imageRef, status)
		}
	}

	return nil
}

// isKyvernoVerificationEnabled checks if Kyverno verification is enabled.
// It reads from the KYVERNO_VERIFICATION_ENABLED environment variable.
// Defaults to false (disabled) if not set or invalid.
func isKyvernoVerificationEnabled() bool {
	envValue := os.Getenv(EnvKyvernoEnabled)
	if envValue == "" {
		// Default to disabled
		return false
	}

	// Parse the value - accept "true", "1", "yes" as enabled
	switch strings.ToLower(envValue) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		// Invalid value, default to disabled and log a warning
		kernelcacheLog.Info("Invalid value for KYVERNO_VERIFICATION_ENABLED, defaulting to disabled", "value", envValue)
		return false
	}
}

// resolveImageDigest resolves an image reference to its digest without verifying signatures.
// This is used when Kyverno verification is disabled (development/testing mode).
// It returns the image digest string (sha256:...) if successful.
func resolveImageDigest(ctx context.Context, imageRef string) (string, error) {
	// Parse the image reference (tag or digest)
	ref, err := gcrname.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parse image reference: %w", err)
	}

	// Registry access options (authn.DefaultKeychain covers most cases)
	remoteOpts := []gcrremote.Option{
		gcrremote.WithAuthFromKeychain(authn.DefaultKeychain),
		gcrremote.WithContext(ctx),
	}

	// Get the image descriptor to retrieve the digest
	descriptor, err := gcrremote.Get(ref, remoteOpts...)
	if err != nil {
		return "", fmt.Errorf("fetch image descriptor: %w", err)
	}

	return descriptor.Digest.String(), nil
}

// mutationKeyFromEnv retrieves the HMAC secret from environment variable
func mutationKeyFromEnv() (string, error) {
	k := os.Getenv(EnvMutationSigningKey)
	if k == "" {
		return "", fmt.Errorf("%s environment variable not set", EnvMutationSigningKey)
	}
	return k, nil
}

// signMutation creates HMAC signature: HMAC(secret, image|digest), base64-encoded
// The signature binds the digest to the image ref to prevent digest tampering
// Note: Cannot use requestUID as mutating and validating webhooks have different UIDs
func signMutation(secret, requestUID, image, digest string) (string, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(image))
	mac.Write([]byte("|"))
	mac.Write([]byte(digest))
	sum := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(sum), nil
}

// verifyMutation verifies the HMAC signature matches expected value
// Uses constant-time comparison to prevent timing attacks
func verifyMutation(secret, requestUID, image, digest, sigB64 string) bool {
	if sigB64 == "" {
		return false
	}
	wantSig, _ := signMutation(secret, requestUID, image, digest)
	want, _ := base64.StdEncoding.DecodeString(wantSig)
	got, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	return hmac.Equal(want, got)
}
