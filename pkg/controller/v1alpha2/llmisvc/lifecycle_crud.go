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
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// clientWithRecorder combines the client and event recorder interfaces
// This allows CRUD operations to both modify resources and emit events
type clientWithRecorder interface {
	client.Client
	record.EventRecorder
}

// Create creates a new Kubernetes resource and emits an event on the owner
// This provides a standardized way to create owned resources with proper event logging
func Create[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, expected T) error {
	if err := c.Create(ctx, expected); err != nil {
		return fmt.Errorf("failed to create %s %s/%s: %w", logLineForObject(expected), expected.GetNamespace(), expected.GetName(), err)
	}

	if !reflect.ValueOf(owner).IsNil() {
		c.Eventf(owner, corev1.EventTypeNormal, "Created", "Created %s %s/%s", logLineForObject(expected), expected.GetNamespace(), expected.GetName())
	}

	return nil
}

// Delete safely deletes a Kubernetes resource with ownership validation
// It ensures only the owner can delete the resource and handles garbage collection properly
func Delete[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, expected T) error {
	typeLogLine := logLineForObject(expected)
	isOwnerNil := reflect.ValueOf(owner).IsNil()

	ownerLogLine := ""
	if !isOwnerNil {
		ownerLogLine = logLineForObject(owner)
	}

	if isNamespaced, err := apiutil.IsObjectNamespaced(expected, c.Scheme(), c.RESTMapper()); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("failed to resolve if resource is namespaced %s: %w", typeLogLine, err)
	} else if isNamespaced && !isOwnerNil {
		if !metav1.IsControlledBy(expected, owner) {
			return fmt.Errorf("failed to delete %s %s/%s: it is not controlled by %s %s/%s",
				typeLogLine,
				expected.GetNamespace(), expected.GetName(),
				ownerLogLine,
				owner.GetNamespace(), owner.GetName(),
			)
		} else if !owner.GetDeletionTimestamp().IsZero() {
			// If the owner is being deleted, assume the owned resource is going
			// to be automatically garbage colleted by the cluster
			return nil
		}
	}

	// Don't re-try deletion, if the owned object is already being deleted
	if !expected.GetDeletionTimestamp().IsZero() {
		return nil
	}

	if err := c.Delete(ctx, expected); err != nil {
		if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
		}
		return nil
	}

	if !isOwnerNil {
		c.Eventf(owner, corev1.EventTypeNormal, "Deleted", "Deleted %s %s/%s", typeLogLine, expected.GetNamespace(), expected.GetName())
	}

	return nil
}

// Reconcile ensures a resource exists and matches the expected state
// It creates the resource if it doesn't exist, or updates it if it differs from expected state
func Reconcile[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, empty, expected T, isEqual SemanticEqual[T], opts ...UpdateOption[T]) error {
	typeLogLine := logLineForObject(expected)

	// Try to fetch the current state of the resource
	curr := empty.DeepCopyObject().(T)
	if err := c.Get(ctx, client.ObjectKeyFromObject(expected), curr); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
		}
		// Resource doesn't exist, create it
		return Create(ctx, c, owner, expected)
	}
	// Resource exists, update it if necessary
	return Update(ctx, c, owner, curr, expected, isEqual, opts...)
}

// Update modifies an existing Kubernetes resource to match the expected state
// It validates ownership and only updates if the resource has actually changed
func Update[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, curr, expected T, isEqual SemanticEqual[T], opts ...UpdateOption[T]) error {
	options := &updateOptions[T]{}
	for _, opt := range opts {
		opt(options)
	}

	typeLogLine := logLineForObject(expected)
	isOwnerNil := reflect.ValueOf(owner).IsNil()

	ownerLogLine := ""
	if !isOwnerNil {
		ownerLogLine = logLineForObject(owner)
	}

	if isNamespaced, err := apiutil.IsObjectNamespaced(expected, c.Scheme(), c.RESTMapper()); err != nil {
		return fmt.Errorf("failed to resolve if resource is namespaced %s: %w", typeLogLine, err)
	} else if isNamespaced && !isOwnerNil {
		if !metav1.IsControlledBy(curr, owner) {
			return fmt.Errorf("failed to update %s %s/%s: it is not controlled by %s %s/%s",
				typeLogLine,
				curr.GetNamespace(), curr.GetName(),
				ownerLogLine,
				owner.GetNamespace(), owner.GetName(),
			)
		}
	}

	expectedGiven := expected.DeepCopyObject().(T)

	expected.SetResourceVersion(curr.GetResourceVersion())
	if err := c.Update(ctx, expected, client.DryRunAll); err != nil {
		return fmt.Errorf("failed to get defaults for %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
	}

	// Apply after dry-run mutations (e.g., preserve fields from curr)
	for _, fn := range options.afterDryRunFns {
		fn(expected, expectedGiven, curr)
	}

	if isEqual(expected, curr) {
		return nil
	}

	log.FromContext(ctx).V(2).Info("Updating "+typeLogLine,
		"expected", expected,
		"curr", curr,
	)

	if err := c.Update(ctx, expected); err != nil {
		return fmt.Errorf("failed to update %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
	}

	if !isOwnerNil {
		c.Eventf(owner, corev1.EventTypeNormal, "Updated", "Updated %s %s/%s", typeLogLine, expected.GetNamespace(), expected.GetName())
	}

	return nil
}

// logLineForObject returns a human-readable type name for logging
// We use reflection since GetObjectKind() is often empty in practice
func logLineForObject(obj client.Object) string {
	// Note: don't use `obj.GetObjectKind()` as it's always empty.
	return strings.Replace(reflect.TypeOf(obj).String(), "*", "", 1)
}

// SemanticEqual is a function type for comparing two objects to determine if they are equivalent
// This allows custom comparison logic beyond simple equality checks
type SemanticEqual[T client.Object] func(expected T, curr T) bool

// UpdateOption is a functional option for the Update and Reconcile functions
type UpdateOption[T client.Object] func(*updateOptions[T])

// AfterDryRunFunc is a callback function type for AfterDryRun options.
// It receives:
//   - expected: the object after dry-run (with server defaults applied) - modify this to take effect
//   - expectedGiven: the original object before dry-run - use this to check what was originally set
//   - curr: the current state of the resource in the cluster
type AfterDryRunFunc[T client.Object] func(expected, expectedGiven, curr T)

type updateOptions[T client.Object] struct {
	afterDryRunFns []AfterDryRunFunc[T]
}

// AfterDryRun configures Update to call the provided function after the dry-run
// populates server-side defaults but before comparing expected with curr.
//
// Multiple AfterDryRun options can be provided and they will be applied in order.
//
// This allows preserving fields from curr that shouldn't be overwritten
// (e.g., Replicas when the owner doesn't set it and HPA manages scaling).
func AfterDryRun[T client.Object](fn AfterDryRunFunc[T]) UpdateOption[T] {
	return func(o *updateOptions[T]) {
		o.afterDryRunFns = append(o.afterDryRunFns, fn)
	}
}

// GetOption is a functional option for the Get function
type GetOption[T client.Object] func(*getOptions[T])

type getOptions[T client.Object] struct {
	fallback func(ctx context.Context, namespace, name string) (T, error)
}

// WithGetFallback configures Get to fall back to the API server if the resource
// is not found in the cache. This is useful when the cache only includes resources
// with specific labels, but you need to fetch resources without those labels.
func WithGetFallback[T client.Object](getter func(ctx context.Context, namespace, name string) (T, error)) GetOption[T] {
	return func(o *getOptions[T]) {
		o.fallback = getter
	}
}

func WithGetFallbackAPIServerConfigMap(c kubernetes.Interface) GetOption[*corev1.ConfigMap] {
	return WithGetFallback[*corev1.ConfigMap](func(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
		return c.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	})
}

// Get retrieves a Kubernetes resource by namespace and name.
// It first tries the cached client, and optionally falls back to a direct API server
// call if the resource is not found and WithGetFallback option is provided.
// Returns the object (either from cache or fallback) and any error.
func Get[T client.Object](ctx context.Context, c client.Client, key client.ObjectKey, obj T, opts ...GetOption[T]) (T, error) {
	options := &getOptions[T]{}
	for _, opt := range opts {
		opt(options)
	}

	typeLogLine := logLineForObject(obj)

	if err := c.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) || options.fallback == nil {
			return obj, fmt.Errorf("failed to get %s %s/%s from cached client: %w", typeLogLine, key.Namespace, key.Name, err)
		}
		// Resource not in cache - try fallback.
		r, err := options.fallback(ctx, key.Namespace, key.Name)
		if err != nil {
			return obj, fmt.Errorf("failed to get %s %s/%s from API Server: %w", typeLogLine, key.Namespace, key.Name, err)
		}
		return r, nil
	}
	return obj, nil
}
