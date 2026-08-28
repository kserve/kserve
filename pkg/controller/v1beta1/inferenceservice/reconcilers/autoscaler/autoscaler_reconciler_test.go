/*
Copyright 2021 The KServe Authors.

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

package autoscaler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestGetAutoscalerClass(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	testCases := []struct {
		name                   string
		isvcMetaData           *metav1.ObjectMeta
		expectedAutoScalerType constants.AutoscalerClassType
	}{
		{
			name: "Return default AutoScaler,if the autoscalerClass annotation is not set",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{},
			},

			expectedAutoScalerType: constants.AutoscalerClassHPA,
		},
		{
			name: "Return default AutoScaler,if the autoscalerClass annotation set hpa",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "hpa"},
			},

			expectedAutoScalerType: constants.AutoscalerClassHPA,
		},
		{
			name: "Return external AutoScaler,if the autoscalerClass annotation set external",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "external"},
			},
			expectedAutoScalerType: constants.AutoscalerClassExternal,
		},
		{
			name: "Return none AutoScaler,if the autoscalerClass annotation set none",
			isvcMetaData: &metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "none"},
			},
			expectedAutoScalerType: constants.AutoscalerClassNone,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getAutoscalerClass(*tt.isvcMetaData)
			if diff := cmp.Diff(tt.expectedAutoScalerType, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func TestCreateAutoscaler(t *testing.T) {
	type args struct {
		client        client.Client
		scheme        *runtime.Scheme
		componentMeta metav1.ObjectMeta
		componentExt  *v1beta1.ComponentExtensionSpec
		configMap     *corev1.ConfigMap
	}
	serviceName := "my-model"
	namespace := "test"
	baseMeta := metav1.ObjectMeta{
		Name:      serviceName,
		Namespace: namespace,
	}
	tests := []struct {
		name        string
		annotations map[string]string
		wantType    string
		wantErr     bool
	}{
		{
			name:        "Return HPAReconciler for default (no annotation)",
			annotations: map[string]string{},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return HPAReconciler for hpa annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "hpa"},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return HPAReconciler for external annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "external"},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return HPAReconciler for none annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "none"},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return NoOpAutoscaler for keda annotation without autoScaling spec",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "keda"},
			wantType:    "*autoscaler.NoOpAutoscaler",
			wantErr:     false,
		},
		{
			name:        "Return error for unknown annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "unknown"},
			wantType:    "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := baseMeta
			meta.Annotations = tt.annotations

			// Provide a dummy configMap for keda autoscalerClass to avoid nil pointer panic
			var configMap *corev1.ConfigMap
			if tt.annotations["serving.kserve.io/autoscalerClass"] == "keda" {
				configMap = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-config",
						Namespace: "default",
					},
					Data: map[string]string{},
				}
			}

			// Use nils for client, scheme as we only test type selection logic
			as, err := createAutoscaler(nil, nil, meta, &v1beta1.ComponentExtensionSpec{}, configMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("createAutoscaler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if as != nil {
					t.Errorf("Expected nil Autoscaler on error, got %T", as)
				}
				return
			}
			if as == nil {
				t.Errorf("Expected Autoscaler, got nil")
				return
			}
			gotType := fmt.Sprintf("%T", as)
			if gotType != tt.wantType {
				t.Errorf("Expected Autoscaler type %s, got %s", tt.wantType, gotType)
			}
		})
	}
}

func TestNewAutoscalerReconciler(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	baseMeta := metav1.ObjectMeta{
		Name:      serviceName,
		Namespace: namespace,
	}
	tests := []struct {
		name        string
		annotations map[string]string
		wantType    string
		wantErr     bool
	}{
		{
			name:        "Return AutoscalerReconciler with HPAReconciler for default (no annotation)",
			annotations: map[string]string{},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return AutoscalerReconciler with HPAReconciler for hpa annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "hpa"},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return AutoscalerReconciler with HPAReconciler for external annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "external"},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return AutoscalerReconciler with HPAReconciler for none annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "none"},
			wantType:    "*hpa.HPAReconciler",
			wantErr:     false,
		},
		{
			name:        "Return AutoscalerReconciler with NoOpAutoscaler for keda annotation without autoScaling spec",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "keda"},
			wantType:    "*autoscaler.NoOpAutoscaler",
			wantErr:     false,
		},
		{
			name:        "Return error for unknown annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "unknown"},
			wantType:    "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := baseMeta
			meta.Annotations = tt.annotations

			// Provide a dummy configMap for keda autoscalerClass to avoid nil pointer panic
			var configMap *corev1.ConfigMap
			if tt.annotations["serving.kserve.io/autoscalerClass"] == "keda" {
				configMap = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-config",
						Namespace: "default",
					},
					Data: map[string]string{},
				}
			}

			ar, err := NewAutoscalerReconciler(nil, nil, meta, &v1beta1.ComponentExtensionSpec{}, configMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAutoscalerReconciler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if ar != nil {
					t.Errorf("Expected nil AutoscalerReconciler on error, got %T", ar)
				}
				return
			}
			if ar == nil {
				t.Errorf("Expected AutoscalerReconciler, got nil")
				return
			}
			if ar.Autoscaler == nil {
				t.Errorf("Expected Autoscaler in AutoscalerReconciler, got nil")
				return
			}
			gotType := fmt.Sprintf("%T", ar.Autoscaler)
			if gotType != tt.wantType {
				t.Errorf("Expected Autoscaler type %s, got %s", tt.wantType, gotType)
			}
		})
	}
}

type fakeAutoscaler struct {
	reconcileCalled bool
	reconcileErr    error
}

var _ Autoscaler = &fakeAutoscaler{}

// Implement Autoscaler interface
func (f *fakeAutoscaler) Reconcile(ctx context.Context) error {
	f.reconcileCalled = true
	return f.reconcileErr
}

func (f *fakeAutoscaler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return nil
}

func TestAutoscalerReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name         string
		reconcileErr error
		wantErr      bool
	}{
		{
			name:         "Reconcile succeeds",
			reconcileErr: nil,
			wantErr:      false,
		},
		{
			name:         "Reconcile returns error",
			reconcileErr: errors.New("some error"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAutoscaler{reconcileErr: tt.reconcileErr}
			ar := &AutoscalerReconciler{
				Autoscaler: fake,
			}
			err := ar.Reconcile(t.Context())
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !fake.reconcileCalled {
				t.Errorf("Expected Reconcile to be called on Autoscaler")
			}
		})
	}
}

func TestNoOpAutoscaler(t *testing.T) {
	noOp := &NoOpAutoscaler{}

	// Test Reconcile method
	err := noOp.Reconcile(t.Context())
	if err != nil {
		t.Errorf("NoOpAutoscaler.Reconcile() should not return error, got: %v", err)
	}

	// Test SetControllerReferences method
	err = noOp.SetControllerReferences(nil, nil)
	if err != nil {
		t.Errorf("NoOpAutoscaler.SetControllerReferences() should not return error, got: %v", err)
	}
}

func TestExternalAutoscalerWithNilComponentExt(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	meta := metav1.ObjectMeta{
		Name:        serviceName,
		Namespace:   namespace,
		Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "external"},
	}

	// Test with nil componentExt - this should not panic
	as, err := createAutoscaler(nil, nil, meta, nil, nil)
	if err != nil {
		t.Errorf("createAutoscaler() with nil componentExt should not error for external class, got: %v", err)
	}

	if as == nil {
		t.Errorf("Expected HPAReconciler, got nil")
		return
	}

	gotType := fmt.Sprintf("%T", as)
	expectedType := "*hpa.HPAReconciler"
	if gotType != expectedType {
		t.Errorf("Expected autoscaler type %s, got %s", expectedType, gotType)
	}
}

func TestNoneAutoscalerWithNilComponentExt(t *testing.T) {
	serviceName := "my-model"
	namespace := "test"
	meta := metav1.ObjectMeta{
		Name:        serviceName,
		Namespace:   namespace,
		Annotations: map[string]string{"serving.kserve.io/autoscalerClass": "none"},
	}

	// Test with nil componentExt - this should not panic
	as, err := createAutoscaler(nil, nil, meta, nil, nil)
	if err != nil {
		t.Errorf("createAutoscaler() with nil componentExt should not error for none class, got: %v", err)
	}

	if as == nil {
		t.Errorf("Expected HPAReconciler, got nil")
		return
	}

	gotType := fmt.Sprintf("%T", as)
	expectedType := "*hpa.HPAReconciler"
	if gotType != expectedType {
		t.Errorf("Expected autoscaler type %s, got %s", expectedType, gotType)
	}
}

// TestKedaIndependentComponentAutoscaling verifies that when the ISVC-level
// autoscalerClass annotation is set to "keda", only components that explicitly
// declare an autoScaling spec get a KedaReconciler. Components without one
// get a NoOpAutoscaler, allowing predictor, transformer, and explainer to
// scale independently.
func TestKedaIndependentComponentAutoscaling(t *testing.T) {
	namespace := "test"
	serviceName := "my-model"
	kedaAnnotations := map[string]string{"serving.kserve.io/autoscalerClass": "keda"}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dummy-config", Namespace: "default"},
		Data:       map[string]string{},
	}
	utilization := int32(50)
	autoScalingSpec := &v1beta1.AutoScalingSpec{
		Metrics: []v1beta1.MetricsSpec{
			{
				Type: v1beta1.ResourceMetricSourceType,
				Resource: &v1beta1.ResourceMetricSource{
					Name: v1beta1.ResourceMetricCPU,
					Target: v1beta1.MetricTarget{
						Type:               v1beta1.UtilizationMetricType,
						AverageUtilization: &utilization,
					},
				},
			},
		},
	}

	tests := []struct {
		name         string
		componentExt *v1beta1.ComponentExtensionSpec
		wantType     string
	}{
		{
			name:         "component without autoScaling gets NoOpAutoscaler",
			componentExt: &v1beta1.ComponentExtensionSpec{
				// No AutoScaling field
			},
			wantType: "*autoscaler.NoOpAutoscaler",
		},
		{
			name:         "component with nil componentExt gets NoOpAutoscaler",
			componentExt: nil,
			wantType:     "*autoscaler.NoOpAutoscaler",
		},
		{
			name: "component with autoScaling spec gets KedaReconciler",
			componentExt: &v1beta1.ComponentExtensionSpec{
				AutoScaling: autoScalingSpec,
			},
			wantType: "*keda.KedaReconciler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := metav1.ObjectMeta{
				Name:        serviceName,
				Namespace:   namespace,
				Annotations: kedaAnnotations,
			}

			as, err := createAutoscaler(nil, nil, meta, tt.componentExt, configMap)
			if err != nil {
				t.Fatalf("createAutoscaler() unexpected error: %v", err)
			}
			gotType := fmt.Sprintf("%T", as)
			if gotType != tt.wantType {
				t.Errorf("Expected %s, got %s", tt.wantType, gotType)
			}
		})
	}
}

// TestKedaNoOpAnnotationOverride verifies that when createAutoscaler returns a
// NoOpAutoscaler for a KEDA component without autoScaling, callers can detect
// it via type assertion and override the annotation to "none" so the deployment
// reconciler lets the Deployment own its replica count.
func TestKedaNoOpAnnotationOverride(t *testing.T) {
	meta := metav1.ObjectMeta{
		Name:      "my-model-predictor",
		Namespace: "test",
		Annotations: map[string]string{
			constants.AutoscalerClass: string(constants.AutoscalerClassKeda),
		},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dummy-config", Namespace: "default"},
		Data:       map[string]string{},
	}

	as, err := createAutoscaler(nil, nil, meta, &v1beta1.ComponentExtensionSpec{}, configMap)
	if err != nil {
		t.Fatalf("createAutoscaler() unexpected error: %v", err)
	}

	// Verify NoOpAutoscaler is returned
	_, isNoOp := as.(*NoOpAutoscaler)
	if !isNoOp {
		t.Fatalf("Expected NoOpAutoscaler, got %T", as)
	}

	// Simulate the annotation override that raw_kube_reconciler performs
	if isNoOp && meta.Annotations[constants.AutoscalerClass] == string(constants.AutoscalerClassKeda) {
		meta.Annotations[constants.AutoscalerClass] = string(constants.AutoscalerClassNone)
	}

	if got := meta.Annotations[constants.AutoscalerClass]; got != string(constants.AutoscalerClassNone) {
		t.Errorf("Expected annotation to be overridden to %q, got %q", constants.AutoscalerClassNone, got)
	}
}
