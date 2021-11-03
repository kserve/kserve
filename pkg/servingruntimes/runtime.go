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

package servingruntimes

import (
	"context"

	goerrors "github.com/pkg/errors"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StringSet map[string]struct{}

var exists = struct{}{} // empty struct placeholder

func (ss StringSet) Add(s string) {
	ss[s] = exists
}

func (ss StringSet) Contains(s string) bool {
	_, found := ss[s]
	return found
}

func GetRuntimesSupportingModelType(cl client.Client, namespace string, p v1beta1.PredictorSpec) ([]v1alpha1.ServingRuntimeSpec, error) {

	// List all namespace-scoped runtimes.
	runtimes := &v1alpha1.ServingRuntimeList{}
	if err := cl.List(context.TODO(), runtimes, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	// List all cluster-scoped runtimes.
	clusterRuntimes := &v1alpha1.ClusterServingRuntimeList{}
	if err := cl.List(context.TODO(), clusterRuntimes); err != nil {
		return nil, err
	}

	srSpecs := make([]v1alpha1.ServingRuntimeSpec, 0, len(runtimes.Items)+len(clusterRuntimes.Items))
	for _, rt := range runtimes.Items {
		if !rt.Spec.IsDisabled() && RuntimeSupportsPredictor(rt.GetName(), &rt.Spec, &p) {
			srSpecs = append(srSpecs, rt.Spec)
		}
	}

	for _, crt := range clusterRuntimes.Items {
		if !crt.Spec.IsDisabled() && RuntimeSupportsPredictor(crt.GetName(), &crt.Spec, &p) {
			srSpecs = append(srSpecs, crt.Spec)
		}
	}
	return srSpecs, nil
}

// Get a ServingRuntime by name. First, ServingRuntimes in the given namespace will be checked.
// If a resource of the specified name is not found, then ClusterServingRuntimes will be checked.
func GetRuntime(cl client.Client, name string, namespace string) (*v1alpha1.ServingRuntimeSpec, error) {

	runtime := &v1alpha1.ServingRuntime{}
	err := cl.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: namespace}, runtime)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		return &runtime.Spec, nil
	}

	clusterRuntime := &v1alpha1.ClusterServingRuntime{}
	err = cl.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: namespace}, clusterRuntime)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		return &runtime.Spec, nil
	}
	return nil, goerrors.New("No ServingRuntimes or ClusterServingRuntimes with the name: " + name)
}

func RuntimeSupportsPredictor(name string, srs *v1alpha1.ServingRuntimeSpec, p *v1beta1.PredictorSpec) bool {
	// assignment to a runtime depends on the model type labels
	runtimeLabelSet := GetServingRuntimeSupportedModelTypeLabelSet(name, srs.SupportedModelTypes)
	predictorLabel := GetPredictorModelTypeLabel(p)
	// if the runtime has the predictor's label, then it supports that predictor
	return runtimeLabelSet.Contains(predictorLabel)
}

func GetServingRuntimeSupportedModelTypeLabelSet(name string, supportedModelTypes []v1beta1.Framework) StringSet {
	set := make(StringSet, 2*len(supportedModelTypes)+1)

	// model type labels
	for _, t := range supportedModelTypes {
		set.Add("mt:" + t.Name)
		if t.Version != nil {
			set.Add("mt:" + t.Name + ":" + *t.Version)
		}
	}
	// runtime label
	set.Add("rt:" + name)
	return set
}

func GetPredictorModelTypeLabel(p *v1beta1.PredictorSpec) string {
	if p.Model.Runtime != nil {
		// constrain placement to specific runtime
		return "rt:" + *p.Model.Runtime
	}
	// constrain placement based on model type
	mt := p.Model.Framework
	if mt.Version != nil {
		return "mt:" + mt.Name + ":" + *mt.Version
	}
	return "mt:" + mt.Name
}
