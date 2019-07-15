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

package knative

import (
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RouteBuilder struct {
}

func NewRouteBuilder() *RouteBuilder {
	return &RouteBuilder{}
}

func (r *RouteBuilder) CreateKnativeRoute(kfsvc *v1alpha1.KFService) *knservingv1alpha1.Route {
	defaultPercent := 100
	canaryPercent := 0
	if kfsvc.Spec.Canary != nil {
		defaultPercent = 100 - kfsvc.Spec.CanaryTrafficPercent
		canaryPercent = kfsvc.Spec.CanaryTrafficPercent
	}
	trafficTargets := []knservingv1alpha1.TrafficTarget{
		{
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: constants.DefaultConfigurationName(kfsvc.Name),
				Percent:           defaultPercent,
			},
		},
	}
	if kfsvc.Spec.Canary != nil {
		trafficTargets = append(trafficTargets, knservingv1alpha1.TrafficTarget{
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: constants.CanaryConfigurationName(kfsvc.Name),
				Percent:           canaryPercent,
			},
		})
	}
	var kfsvcAnnotations map[string]string
	filteredAnnotations := utils.Filter(kfsvc.Annotations, routeAnnotationFilter)
	if len(filteredAnnotations) > 0 {
		kfsvcAnnotations = filteredAnnotations
	}
	return &knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:        kfsvc.Name,
			Namespace:   kfsvc.Namespace,
			Labels:      kfsvc.Labels,
			Annotations: kfsvcAnnotations,
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: trafficTargets,
		},
	}
}

func routeAnnotationFilter(annotationKey string) bool {
	switch annotationKey {
	case "kubectl.kubernetes.io/last-applied-configuration":
		return false
	default:
		return true
	}
}
