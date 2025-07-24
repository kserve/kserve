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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type clientWithRecorder interface {
	client.Client
	record.EventRecorder
}

func Create[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, expected T) error {
	if err := c.Create(ctx, expected); err != nil {
		return fmt.Errorf("failed to create %s %s/%s: %w", logLineForObject(expected), expected.GetNamespace(), expected.GetName(), err)
	}

	c.Eventf(owner, corev1.EventTypeNormal, "Created", "Created %s %s/%s", logLineForObject(expected), expected.GetNamespace(), expected.GetName())
	return nil
}

func Delete[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, expected T) error {
	typeLogLine := logLineForObject(expected)
	ownerLogLine := logLineForObject(owner)

	if !metav1.IsControlledBy(expected, owner) {
		return fmt.Errorf("failed to delete %s %s/%s: it is not controlled by %s %s/%s",
			typeLogLine,
			expected.GetNamespace(), expected.GetName(),
			ownerLogLine,
			owner.GetNamespace(), owner.GetName(),
		)
	}

	if err := c.Delete(ctx, expected); err != nil {
		if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
		}
		return nil
	}

	c.Eventf(owner, corev1.EventTypeNormal, "Deleted", "Deleted %s %s/%s", typeLogLine, expected.GetNamespace(), expected.GetName())
	return nil
}

func Reconcile[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, empty, expected T, isEqual SemanticEqual[T]) error {
	typeLogLine := logLineForObject(expected)

	curr := empty.DeepCopyObject().(T)
	if err := c.Get(ctx, client.ObjectKeyFromObject(expected), curr); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
		}
		return Create(ctx, c, owner, expected)
	}
	return Update(ctx, c, owner, curr, expected, isEqual)
}

func Update[O client.Object, T client.Object](ctx context.Context, c clientWithRecorder, owner O, curr, expected T, isEqual SemanticEqual[T]) error {
	typeLogLine := logLineForObject(expected)
	ownerLogLine := logLineForObject(owner)

	if !metav1.IsControlledBy(curr, owner) {
		return fmt.Errorf("failed to update %s %s/%s: it is not controlled by %s %s/%s",
			typeLogLine,
			curr.GetNamespace(), curr.GetName(),
			ownerLogLine,
			owner.GetNamespace(), owner.GetName(),
		)
	}

	expected.SetResourceVersion(curr.GetResourceVersion())
	if err := c.Update(ctx, expected, client.DryRunAll); err != nil {
		return fmt.Errorf("failed to get defaults for %s %s/%s: %w", typeLogLine, expected.GetNamespace(), expected.GetName(), err)
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
	c.Eventf(owner, corev1.EventTypeNormal, "Updated", "Updated %s %s/%s", typeLogLine, expected.GetNamespace(), expected.GetName())

	return nil
}

func logLineForObject(obj client.Object) string {
	// Note: don't use `obj.GetObjectKind()` as it's always empty.
	return reflect.TypeOf(obj).String()
}

type SemanticEqual[T client.Object] func(expected T, curr T) bool
