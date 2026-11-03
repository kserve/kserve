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

package raw

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/autoscaler"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
)

type cleanupWorkload struct {
	err    error
	called bool
}

func (r *cleanupWorkload) Reconcile(context.Context) ([]*appsv1.Deployment, error) {
	return nil, nil
}

func (r *cleanupWorkload) GetWorkloads() []metav1.Object {
	return nil
}

func (r *cleanupWorkload) SetControllerReferences(metav1.Object, *runtime.Scheme) error {
	return nil
}

func (r *cleanupWorkload) CleanupOrphans(context.Context, isvcutils.OrphanScope) error {
	r.called = true
	return r.err
}

type cleanupService struct {
	err    error
	called bool
}

func (r *cleanupService) Reconcile(context.Context) ([]*corev1.Service, error) {
	return nil, nil
}

func (r *cleanupService) GetServiceList() []*corev1.Service {
	return nil
}

func (r *cleanupService) SetControllerReferences(metav1.Object, *runtime.Scheme) error {
	return nil
}

func (r *cleanupService) CleanupOrphans(context.Context, isvcutils.OrphanScope) error {
	r.called = true
	return r.err
}

type cleanupAutoscaler struct {
	err    error
	called bool
}

func (r *cleanupAutoscaler) Reconcile(context.Context) error {
	return nil
}

func (r *cleanupAutoscaler) SetControllerReferences(metav1.Object, *runtime.Scheme) error {
	return nil
}

func (r *cleanupAutoscaler) CleanupOrphans(context.Context, isvcutils.OrphanScope) error {
	r.called = true
	return r.err
}

func TestCleanupOrphansAggregatesSubReconcilerErrors(t *testing.T) {
	workloadErr := errors.New("workload cleanup failed")
	serviceErr := errors.New("service cleanup failed")
	scalerErr := errors.New("scaler cleanup failed")
	workload := &cleanupWorkload{err: workloadErr}
	service := &cleanupService{err: serviceErr}
	scaler := &cleanupAutoscaler{err: scalerErr}
	reconciler := &RawKubeReconciler{
		Workload: workload,
		Service:  service,
		Scaler:   &autoscaler.AutoscalerReconciler{Autoscaler: scaler},
	}

	err := reconciler.CleanupOrphans(t.Context(), isvcutils.OrphanScope{Namespace: "default"})

	require.Error(t, err)
	assert.ErrorIs(t, err, workloadErr)
	assert.ErrorIs(t, err, serviceErr)
	assert.ErrorIs(t, err, scalerErr)
	assert.True(t, workload.called)
	assert.True(t, service.called)
	assert.True(t, scaler.called)
}
