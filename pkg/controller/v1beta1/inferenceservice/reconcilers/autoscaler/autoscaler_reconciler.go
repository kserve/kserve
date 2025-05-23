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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	hpa "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/hpa"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/keda"
)

// Autoscaler Interface implemented by all autoscalers
type Autoscaler interface {
	Reconcile(ctx context.Context) error
	SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error
}

// NoOpAutoscaler Autoscaler that does nothing. Can be used to disable creation of autoscaler resources.
type NoOpAutoscaler struct{}

func (*NoOpAutoscaler) Reconcile(ctx context.Context) error {
	return nil
}

func (a *NoOpAutoscaler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	return nil
}

// AutoscalerReconciler is the struct of Raw K8S Object
type AutoscalerReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	Autoscaler   Autoscaler
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewAutoscalerReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	configMap *corev1.ConfigMap,
) (*AutoscalerReconciler, error) {
	as, err := createAutoscaler(client, scheme, componentMeta, componentExt, configMap)
	if err != nil {
		return nil, err
	}
	return &AutoscalerReconciler{
		client:       client,
		scheme:       scheme,
		Autoscaler:   as,
		componentExt: componentExt,
	}, err
}

func getAutoscalerClass(metadata metav1.ObjectMeta) constants.AutoscalerClassType {
	annotations := metadata.Annotations
	if value, ok := annotations[constants.AutoscalerClass]; ok {
		return constants.AutoscalerClassType(value)
	} else {
		return constants.DefaultAutoscalerClass
	}
}

func createAutoscaler(client client.Client,
	scheme *runtime.Scheme, componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	configMap *corev1.ConfigMap,
) (Autoscaler, error) {
	ac := getAutoscalerClass(componentMeta)
	switch ac {
	case constants.AutoscalerClassHPA, constants.AutoscalerClassExternal, constants.AutoscalerClassNone:
		return hpa.NewHPAReconciler(client, scheme, componentMeta, componentExt)
	case constants.AutoscalerClassKeda:
		return keda.NewKedaReconciler(client, scheme, componentMeta, componentExt, configMap)
	default:
		return nil, fmt.Errorf("unknown autoscaler class type: %v", ac)
	}
}

// Reconcile autoscaling resources for HPA, KEDA ScaledObject.
func (r *AutoscalerReconciler) Reconcile(ctx context.Context) error {
	// reconcile Autoscaling resources
	err := r.Autoscaler.Reconcile(ctx)
	if err != nil {
		return err
	}
	return nil
}
