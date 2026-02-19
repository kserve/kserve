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

package fixture

import (
	"context"
	"fmt"
	"hash/crc32"
	"regexp"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgtest "github.com/kserve/kserve/pkg/testing"
)

// TestNamespace encapsulates a test namespace with its lifecycle management.
// It ensures proper setup and cleanup ordering.
type TestNamespace struct {
	Name      string
	Namespace *corev1.Namespace
	client    *pkgtest.Client
	// resources tracks all resources created via options for cleanup
	resources []client.Object
}

// TestNamespaceOption configures a TestNamespace during creation
type TestNamespaceOption func(ctx context.Context, tn *TestNamespace)

// WithIstioShadowService creates an Istio shadow service in the namespace.
// This is required for tests that verify Istio integration.
func WithIstioShadowService(svcName string) TestNamespaceOption {
	return func(ctx context.Context, tn *TestNamespace) {
		svc := IstioShadowService(svcName, tn.Name)
		gomega.Expect(tn.client.Create(ctx, svc)).To(gomega.Succeed())
		tn.resources = append(tn.resources, svc)
	}
}

// WithDefaultServiceAccount creates the default ServiceAccount.
// This is automatically applied but can be explicitly added for clarity.
// Note: envtest doesn't run kube-controller-manager, so default SA isn't auto-created.
func WithDefaultServiceAccount() TestNamespaceOption {
	return func(ctx context.Context, tn *TestNamespace) {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: tn.Name,
			},
		}
		err := tn.client.Create(ctx, sa)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
		if err == nil {
			tn.resources = append(tn.resources, sa)
		}
	}
}

// WithServiceAccount creates a custom ServiceAccount in the namespace.
func WithServiceAccount(name string) TestNamespaceOption {
	return func(ctx context.Context, tn *TestNamespace) {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: tn.Name,
			},
		}
		err := tn.client.Create(ctx, sa)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
		if err == nil {
			tn.resources = append(tn.resources, sa)
		}
	}
}

// NewTestNamespace creates a namespace for testing with proper cleanup registered.
//
// It automatically:
//   - Creates the namespace with name derived from current Ginkgo test info
//   - Creates default ServiceAccount when using envtest (real clusters auto-create it)
//   - Registers cleanup via DeferCleanup (ensures proper ordering)
//
// Example usage:
//
//	testNs := NewTestNamespace(ctx, envTest,
//	    WithIstioShadowService("my-service"),
//	)
func NewTestNamespace(ctx context.Context, c *pkgtest.Client, opts ...TestNamespaceOption) *TestNamespace {
	nsName := generateNamespaceName(ginkgo.CurrentSpecReport())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}

	gomega.Expect(c.Create(ctx, namespace)).To(gomega.Succeed())

	tn := &TestNamespace{
		Name:      nsName,
		Namespace: namespace,
		client:    c,
	}

	// Create default ServiceAccount only for envtest (real clusters auto-create it)
	if !c.UsingExistingCluster() {
		WithDefaultServiceAccount()(ctx, tn)
	}

	// Apply additional options
	for _, opt := range opts {
		opt(ctx, tn)
	}

	// Register cleanup via DeferCleanup - this ensures it runs AFTER any
	// inline defers registered after this call (LIFO order)
	// Note: envtest has no GC, so we must explicitly delete all resources
	ginkgo.DeferCleanup(func(ctx context.Context) {
		// Delete tracked resources in reverse order (LIFO)
		for i := len(tn.resources) - 1; i >= 0; i-- {
			if err := c.Delete(ctx, tn.resources[i]); err != nil && !apierrors.IsNotFound(err) {
				// Log but don't fail - cleanup should be best-effort
				fmt.Fprintf(ginkgo.GinkgoWriter, "Warning: failed to delete resource %v: %v\n",
					client.ObjectKeyFromObject(tn.resources[i]), err)
			}
		}
		c.DeleteAll(ctx, namespace)
	})

	return tn
}

// DeleteAndWait deletes the given object and waits for it to be fully removed.
// This should be called for objects with finalizers (like LLMInferenceService)
// before namespace cleanup to avoid race conditions.
func (tn *TestNamespace) DeleteAndWait(ctx context.Context, obj client.Object) {
	err := tn.client.Delete(ctx, obj)
	if apierrors.IsNotFound(err) {
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Wait for the object to be fully deleted (including finalizer processing)
	gomega.Eventually(func(g gomega.Gomega) {
		getErr := tn.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		g.Expect(apierrors.IsNotFound(getErr)).To(gomega.BeTrue(),
			"expected object to be deleted, got: %v", getErr)
	}).WithContext(ctx).Should(gomega.Succeed())
}

// generateNamespaceName creates a DNS-safe namespace name from test info.
// Uses full test hierarchy (Describe/Context/It) for stable namespace name.
//
// Example: Describe("Controller") > Context("Reconciliation") > It("should create")
// becomes: "test-controller-reconciliation-should-create"
// (or "test-controller-reconci-a1b2c3d4" if exceeds 63 chars)
func generateNamespaceName(spec ginkgo.SpecReport) string {
	texts := make([]string, len(spec.ContainerHierarchyTexts)+1)
	copy(texts, spec.ContainerHierarchyTexts)
	texts[len(spec.ContainerHierarchyTexts)] = spec.LeafNodeText
	fullPath := strings.Join(texts, "-")
	return truncateWithHash("test-"+sanitizeForDNS(fullPath), 63)
}

var nonDNSChars = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeForDNS(s string) string {
	return strings.Trim(nonDNSChars.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// truncateWithHash returns name as-is if it fits in maxLen,
// otherwise truncates and appends a hash suffix for uniqueness.
func truncateWithHash(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	hash := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(name)))
	// hash (8 chars) + separator (1 char) = 9 chars reserved for suffix
	truncateAt := max(maxLen-9, 0)
	return strings.TrimRight(name[:truncateAt], "-") + "-" + hash
}
