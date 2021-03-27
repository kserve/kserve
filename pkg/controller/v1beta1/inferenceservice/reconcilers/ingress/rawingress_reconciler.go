/*
Copyright 2021 kubeflow.org.
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

package ingress

import (
	"context"

	v1beta1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	knapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//RawIngressReconciler is the struct of Raw K8S Object
type RawIngressReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	Ingress       *netv1.Ingress
	URL           *knapis.URL
	IngressConfig *v1beta1api.IngressConfig
}

func NewRawIngressReconciler(client client.Client,
	scheme *runtime.Scheme,
	ingressConfig *v1beta1api.IngressConfig,
	isvc *v1beta1api.InferenceService) (*RawIngressReconciler, error) {
	ingress, err := createRawIngress(client, scheme, isvc, ingressConfig)
	if err != nil {
		return nil, err
	}
	return &RawIngressReconciler{
		client:        client,
		scheme:        scheme,
		Ingress:       ingress,
		URL:           createRawURL(client, isvc, ingressConfig),
		IngressConfig: ingressConfig,
	}, nil
}
func createRawURL(client client.Client,
	isvc *v1beta1api.InferenceService,
	ingressConfig *v1beta1api.IngressConfig) *knapis.URL {
	url := &knapis.URL{}
	url.Scheme = "http"
	url.Host = isvc.Name + "-" + isvc.Namespace + "." + ingressConfig.IngressDomain
	return url
}
func generateRule(podSpec *corev1.PodSpec,
	ingressHost string,
	componentName string) netv1.IngressRule {
	pathType := netv1.PathType(netv1.PathTypeImplementationSpecific)
	rule := netv1.IngressRule{
		Host: ingressHost,
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{
					{
						PathType: &pathType,
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: componentName,
								Port: netv1.ServiceBackendPort{
									Number: constants.CommonDefaultHttpPort,
								},
							},
						},
					},
				},
			},
		},
	}
	return rule
}

func generateMetadata(isvc *v1beta1api.InferenceService,
	componentType constants.InferenceServiceComponent) metav1.ObjectMeta {
	var name string
	switch componentType {
	case constants.Transformer:
		name = constants.DefaultTransformerServiceName(isvc.Name)
	case constants.Explainer:
		name = constants.DefaultExplainerServiceName(isvc.Name)
	case constants.Predictor:
		name = constants.DefaultPredictorServiceName(isvc.Name)
	}
	//get annotations from isvc
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	objectMeta := metav1.ObjectMeta{
		Name:      name,
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(componentType),
		}),
		Annotations: annotations,
	}
	return objectMeta
}

//GenerateIngressHost return the config domain in configmap.IngressDomain
func GenerateIngressHost(ingressConfig *v1beta1api.IngressConfig,
	isvc *v1beta1api.InferenceService,
	componentType string,
	topLevelFlag bool) string {
	if ingressConfig.IngressDomain == "" {
		ingressConfig.IngressDomain = "example.com"
	}
	metadata := generateMetadata(isvc, constants.InferenceServiceComponent(componentType))
	if topLevelFlag {
		return metadata.Name + "-" + metadata.Namespace + "." + ingressConfig.IngressDomain
	} else {
		return isvc.Name + "-" + isvc.Namespace + "." + ingressConfig.IngressDomain
	}
}

func createRawIngress(client client.Client,
	scheme *runtime.Scheme,
	isvc *v1beta1api.InferenceService,
	ingressConfig *v1beta1api.IngressConfig) (*netv1.Ingress, error) {
	topLevelFlag := false
	var rules []netv1.IngressRule
	if isvc.Spec.Transformer != nil {
		host := GenerateIngressHost(ingressConfig, isvc, string(constants.Transformer), topLevelFlag)
		//set topLevelFlag as true
		topLevelFlag = true
		podSpec := corev1.PodSpec(isvc.Spec.Transformer.PodSpec)
		rules = append(rules, generateRule(&podSpec, host, constants.DefaultTransformerServiceName(isvc.Name)))
	}
	if isvc.Spec.Explainer != nil {
		host := GenerateIngressHost(ingressConfig, isvc, string(constants.Explainer), topLevelFlag)
		//set topLevelFlag as true
		topLevelFlag = true
		podSpec := corev1.PodSpec(isvc.Spec.Explainer.PodSpec)
		rules = append(rules, generateRule(&podSpec, host, constants.DefaultExplainerServiceName(isvc.Name)))
	}
	//add predictor rule
	host := GenerateIngressHost(ingressConfig, isvc, string(constants.Predictor), topLevelFlag)
	podSpec := corev1.PodSpec(isvc.Spec.Predictor.PodSpec)
	rules = append(rules, generateRule(&podSpec, host, constants.DefaultPredictorServiceName(isvc.Name)))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.ObjectMeta.Name,
			Namespace:   isvc.ObjectMeta.Namespace,
			Annotations: isvc.Annotations,
		},
		Spec: netv1.IngressSpec{
			Rules: rules,
		},
	}
	if err := controllerutil.SetControllerReference(isvc, ingress, scheme); err != nil {
		return nil, err
	}
	return ingress, nil
}

//checkRawIngressExist checks if the ingress exists?
func (r *RawIngressReconciler) checkRawIngressExist(client client.Client) (constants.CheckResultType, error) {
	//get ingress
	exsitingIngress := &netv1.Ingress{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Ingress.Namespace,
		Name:      r.Ingress.Name,
	}, exsitingIngress)
	if err != nil {
		if apierr.IsNotFound(err) {
			return constants.CheckResultCreate, nil
		}
		return constants.CheckResultUnknown, err
	}

	//existed, check equivalent
	if semanticIngressEquals(r.Ingress, exsitingIngress) {
		return constants.CheckResultExisted, nil
	}
	return constants.CheckResultUpdate, nil
}

func semanticIngressEquals(desired, existing *netv1.Ingress) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}

//Reconcile ...
func (r *RawIngressReconciler) Reconcile(isvc *v1beta1api.InferenceService) error {
	//reconcile ingress
	checkResult, err := r.checkRawIngressExist(r.client)
	if err != nil {
		return err
	}
	log.Info("ingress reconcile", "checkResult", checkResult, "err", err)
	if checkResult == constants.CheckResultCreate {
		err = r.client.Create(context.TODO(), r.Ingress)
	} else if checkResult == constants.CheckResultUpdate { //CheckResultUpdate
		err = r.client.Update(context.TODO(), r.Ingress)
	}
	if err != nil {
		return err
	}
	isvc.Status.URL = r.URL
	isvc.Status.Address = &duckv1.Addressable{
		URL: r.URL,
	}
	isvc.Status.SetCondition(v1beta1api.IngressReady, &apis.Condition{
		Type:   v1beta1api.IngressReady,
		Status: corev1.ConditionTrue,
	})
	return nil
}
