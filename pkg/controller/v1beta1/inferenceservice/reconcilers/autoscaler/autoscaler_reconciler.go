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
	"github.com/pkg/errors"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	hpa "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/hpa"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("AutoscalerReconciler")

type Autoscaler struct {
	AutoscalerClass constants.AutoscalerClassType
	HPA             *hpa.HPAReconciler
}

// AutoscalerReconciler is the struct of Raw K8S Object
type AutoscalerReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	Autoscaler   *Autoscaler
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewAutoscalerReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec) (*AutoscalerReconciler, error) {
	as, err := createAutoscaler(client, scheme, componentMeta, componentExt)
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
	componentExt *v1beta1.ComponentExtensionSpec) (*Autoscaler, error) {
	as := &Autoscaler{}
	ac := getAutoscalerClass(componentMeta)
	as.AutoscalerClass = ac
	switch ac {
	case constants.AutoscalerClassHPA:
		as.HPA = hpa.NewHPAReconciler(client, scheme, componentMeta, componentExt)
	default:
		return nil, errors.New("unknown autoscaler class type.")
	}
	return as, nil
}

// Reconcile ...
func (r *AutoscalerReconciler) Reconcile() (*Autoscaler, error) {
	//reconcile Autoscaler
	if r.Autoscaler.AutoscalerClass == constants.AutoscalerClassHPA {
		_, err := r.Autoscaler.HPA.Reconcile()
		if err != nil {
			return nil, err
		}
	}
	return r.Autoscaler, nil
}
