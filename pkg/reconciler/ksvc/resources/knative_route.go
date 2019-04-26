package resources

import (
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateKnativeRoute(kfsvc *v1alpha1.KFService) *knservingv1alpha1.Route {
	defaultPercent := 100
	canaryPercent := 0
	if kfsvc.Spec.Canary != nil {
		defaultPercent = 100 - kfsvc.Spec.Canary.TrafficPercent
		canaryPercent = kfsvc.Spec.Canary.TrafficPercent
	}
	trafficTargets := []knservingv1alpha1.TrafficTarget{
		{
			ConfigurationName: constants.MakeDefaultConfigurationName(kfsvc.Name),
			Percent:           defaultPercent,
		},
	}
	if kfsvc.Spec.Canary != nil {
		trafficTargets = append(trafficTargets, knservingv1alpha1.TrafficTarget{
			ConfigurationName: constants.MakeCanaryConfigurationName(kfsvc.Name),
			Percent:           canaryPercent,
		})
	}
	return &knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kfsvc.Name,
			Namespace: kfsvc.Namespace,
		},
		Spec: knservingv1alpha1.RouteSpec{
			Traffic: trafficTargets,
		},
	}
}
