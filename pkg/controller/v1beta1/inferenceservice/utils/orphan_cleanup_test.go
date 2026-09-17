// Copyright 2026 The KServe Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type deleteErrorClient struct {
	client.Client
	errors   map[string]error
	attempts []string
}

func (c *deleteErrorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.attempts = append(c.attempts, obj.GetName())
	if err, found := c.errors[obj.GetName()]; found {
		return err
	}
	return c.Client.Delete(ctx, obj, opts...)
}

type listErrorClient struct {
	client.Client
	err error
}

func (c *listErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

func TestDeleteOrphansContinuesAfterDeleteErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	labels := map[string]string{"app": "my-isvc"}
	objects := []client.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "expected", Namespace: "default", Labels: labels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "failed-one", Namespace: "default", Labels: labels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "deleted", Namespace: "default", Labels: labels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "failed-two", Namespace: "default", Labels: labels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "already-gone", Namespace: "default", Labels: labels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "default", Labels: map[string]string{"app": "other"}}},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	c := &deleteErrorClient{
		Client: baseClient,
		errors: map[string]error{
			"failed-one":   errors.New("first failure"),
			"failed-two":   errors.New("second failure"),
			"already-gone": apierr.NewNotFound(schema.GroupResource{Resource: "services"}, "already-gone"),
		},
	}

	err := DeleteOrphans[*corev1.ServiceList](t.Context(), c, OrphanScope{
		Namespace:   "default",
		Labels:      client.MatchingLabels(labels),
		RetainNames: sets.New("expected"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fails to delete orphaned Service failed-one: first failure")
	assert.Contains(t, err.Error(), "fails to delete orphaned Service failed-two: second failure")
	assert.NotContains(t, err.Error(), "already-gone")
	assert.ElementsMatch(t, []string{"failed-one", "deleted", "failed-two", "already-gone"}, c.attempts)

	getErr := baseClient.Get(t.Context(), types.NamespacedName{Name: "deleted", Namespace: "default"}, &corev1.Service{})
	assert.True(t, apierr.IsNotFound(getErr))
	for _, name := range []string{"expected", "failed-one", "failed-two", "unrelated"} {
		getErr = baseClient.Get(t.Context(), types.NamespacedName{Name: name, Namespace: "default"}, &corev1.Service{})
		assert.NoError(t, getErr)
	}
}

func TestDeleteOrphansReturnsListError(t *testing.T) {
	wantErr := errors.New("list failure")
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	err := DeleteOrphans[*corev1.ServiceList](t.Context(), &listErrorClient{Client: baseClient, err: wantErr}, OrphanScope{
		Namespace: "default",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, wantErr))
	assert.Contains(t, err.Error(), "fails to list Service resources for cleanup")
}

func TestDeleteOrphansReturnsGVKError(t *testing.T) {
	baseClient := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	err := DeleteOrphans[*corev1.ServiceList](t.Context(), baseClient, OrphanScope{Namespace: "default"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fails to determine resource kind for orphan cleanup")
}
