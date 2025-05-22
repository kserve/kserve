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
			name:        "Return KedaReconciler for keda annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "keda"},
			wantType:    "*keda.KedaReconciler",
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
			// Use nils for client, scheme, configMap as we only test type selection logic
			as, err := createAutoscaler(nil, nil, meta, &v1beta1.ComponentExtensionSpec{}, nil)
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
			name:        "Return AutoscalerReconciler with KedaReconciler for keda annotation",
			annotations: map[string]string{"serving.kserve.io/autoscalerClass": "keda"},
			wantType:    "*keda.KedaReconciler",
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
			ar, err := NewAutoscalerReconciler(nil, nil, meta, &v1beta1.ComponentExtensionSpec{}, nil)
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
			err := ar.Reconcile(context.TODO())
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !fake.reconcileCalled {
				t.Errorf("Expected Reconcile to be called on Autoscaler")
			}
		})
	}
}
