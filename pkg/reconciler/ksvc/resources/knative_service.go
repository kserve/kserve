/*
Copyright 2019 kubeflow.org.

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

package resources

import (
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/frameworks/tensorflow"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateModelServingContainer(modelName string, modelSpec *v1alpha1.ModelSpec) *v1.Container {
	if modelSpec.Tensorflow != nil {
		return tensorflow.CreateTensorflowContainer(modelName, modelSpec.Tensorflow)
	} else {
		//TODO(@yuzisun) handle other model types
		return &v1.Container{}
	}
}

func CreateKnativeService(kfsvc *v1alpha1.KFService) (*knservingv1alpha1.Service, error) {
	var revisions []string
	container := &v1.Container{}
	routingPercent := 0
	if kfsvc.Spec.Canary == nil || kfsvc.Spec.Canary.TrafficPercent == 0 {
		//TODO(@yuzisun) should we add model name to the spec, can be different than service name?
		container = CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Default)
		revisions = []string{knservingv1alpha1.ReleaseLatestRevisionKeyword}
	} else {
		container = CreateModelServingContainer(kfsvc.Name, &kfsvc.Spec.Canary.ModelSpec)
		revisions = []string{kfsvc.Status.Default.Name, knservingv1alpha1.ReleaseLatestRevisionKeyword}
		routingPercent = kfsvc.Spec.Canary.TrafficPercent
	}
	return &knservingv1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        kfsvc.Name,
			Namespace:   kfsvc.Namespace,
			Labels:      kfsvc.Labels,
			Annotations: kfsvc.Annotations,
		},
		Spec: knservingv1alpha1.ServiceSpec{
			Release: &knservingv1alpha1.ReleaseType{
				Revisions:      revisions,
				RolloutPercent: int(routingPercent),
				Configuration: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: *container,
						},
					},
				},
			},
		},
	}, nil
}
