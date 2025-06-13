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

package ingress

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	v1beta1utils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

// RawIngressReconciler reconciles the kubernetes ingress
type RawIngressReconciler struct {
	client        client.Client
	scheme        *runtime.Scheme
	ingressConfig *v1beta1.IngressConfig
	isvcConfig    *v1beta1.InferenceServicesConfig
}

func NewRawIngressReconciler(client client.Client,
	scheme *runtime.Scheme,
	ingressConfig *v1beta1.IngressConfig,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*RawIngressReconciler, error) {
	return &RawIngressReconciler{
		client:        client,
		scheme:        scheme,
		ingressConfig: ingressConfig,
		isvcConfig:    isvcConfig,
	}, nil
}

func (r *RawIngressReconciler) Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) error {
	var err error
	isInternal := false
	// disable ingress creation if service is labelled with cluster local or kserve domain is cluster local
	if val, ok := isvc.Labels[constants.NetworkVisibility]; ok && val == constants.ClusterLocalVisibility {
		isInternal = true
	}
	if r.ingressConfig.IngressDomain == constants.ClusterLocalDomain {
		isInternal = true
	}

	existingIngress := &netv1.Ingress{}
	getExistingErr := r.client.Get(ctx, types.NamespacedName{
		Namespace: isvc.Namespace,
		Name:      isvc.Name,
	}, existingIngress)
	ingressIsNotFound := apierr.IsNotFound(getExistingErr)
	if getExistingErr != nil && !ingressIsNotFound {
		return fmt.Errorf("failed to get existing ingress: %w", getExistingErr)
	}

	// ISVC is stopped, delete the ingress if it exists, otherwise, do nothing
	forceStopRuntime := utils.GetForceStopRuntime(isvc)
	if (getExistingErr != nil && ingressIsNotFound) && forceStopRuntime {
		return nil
	}
	if forceStopRuntime {
		if ctrl := metav1.GetControllerOf(existingIngress); ctrl != nil && ctrl.UID == isvc.UID {
			log.Info("The InferenceService is marked as stopped — deleting its associated ingress", "name", isvc.Name)
			if err := r.client.Delete(ctx, existingIngress); err != nil {
				return err
			}
		}

		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: v1beta1.StoppedISVCReason,
		})

		return nil
	}

	// Create or update ingress to match the desired state
	if !isInternal && !r.ingressConfig.DisableIngressCreation {
		ingress, err := createRawIngress(ctx, r.scheme, isvc, r.ingressConfig, r.client, r.isvcConfig)
		if err != nil {
			return err
		}
		if ingress == nil {
			return nil
		}

		if getExistingErr != nil && ingressIsNotFound {
			log.Info("creating ingress", "ingressName", isvc.Name, "err", err)
			if err := r.client.Create(ctx, ingress); err != nil {
				log.Error(err, "Failed to create ingress", "name", ingress.Name)
				return err
			}
		} else if !semanticIngressEquals(ingress, existingIngress) {
			log.Info("updating ingress", "ingressName", isvc.Name, "err", err)
			if err := r.client.Update(ctx, ingress); err != nil {
				log.Error(err, "Failed to update ingress", "name", ingress.Name)
				return err
			}
		}
	}

	authEnabled := false
	if val, ok := isvc.Annotations[constants.ODHKserveRawAuth]; ok && strings.EqualFold(val, "true") {
		authEnabled = true
	}

	isvc.Status.URL, err = createRawURLODH(ctx, r.client, isvc, authEnabled)
	if err != nil {
		return err
	}

	internalHost := getRawServiceHost(ctx, isvc, r.client)
	url := &apis.URL{
		Host:   internalHost,
		Scheme: "http",
		Path:   "",
	}

	if authEnabled {
		internalHost += ":" + strconv.Itoa(constants.OauthProxyPort)
		url.Host = internalHost
		url.Scheme = "https"
	}

	isvc.Status.Address = &duckv1.Addressable{
		URL: url,
	}

	isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
		Type:   v1beta1.IngressReady,
		Status: corev1.ConditionTrue,
	})

	return nil
}

func createRawURLODH(ctx context.Context, client client.Client, isvc *v1beta1.InferenceService, authEnabled bool) (*knapis.URL, error) {
	// upstream implementation
	// var err error
	// url := &knapis.URL{}
	// url.Scheme = ingressConfig.UrlScheme
	// url.Host, err = GenerateDomainName(isvc.Name, isvc.ObjectMeta, ingressConfig)
	// if err != nil {
	//	return nil, err
	// }
	// if authEnabled {
	//	url.Host += ":" + strconv.Itoa(constants.OauthProxyPort)
	// }
	// return url, nil

	// ODH changes
	var url *knapis.URL
	if val, ok := isvc.Labels[constants.NetworkVisibility]; ok && val == constants.ODHRouteEnabled {
		var err error
		url, err = v1beta1utils.GetRouteURLIfExists(ctx, client, isvc.ObjectMeta, isvc.Name)
		if err != nil {
			return nil, err
		}
	} else {
		url = &apis.URL{
			Host:   getRawServiceHost(ctx, isvc, client),
			Scheme: "http",
			Path:   "",
		}
		if authEnabled {
			url.Host += ":" + strconv.Itoa(constants.OauthProxyPort)
			url.Scheme = "https"
		}
	}
	return url, nil
}

func generateRule(ingressHost string, componentName string, path string, port int32) netv1.IngressRule { //nolint:unparam
	pathType := netv1.PathTypePrefix
	rule := netv1.IngressRule{
		Host: ingressHost,
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{
					{
						Path:     path,
						PathType: &pathType,
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: componentName,
								Port: netv1.ServiceBackendPort{
									Number: port,
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

func generateMetadata(isvc *v1beta1.InferenceService,
	componentType constants.InferenceServiceComponent, name string,
	isvcConfig *v1beta1.InferenceServicesConfig,
) metav1.ObjectMeta {
	// get annotations from isvc
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(v1beta1utils.FilterList(isvcConfig.ServiceAnnotationDisallowedList, constants.ODHKserveRawAuth), key)
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

// generateIngressHost return the config domain in configmap.IngressDomain
func generateIngressHost(ingressConfig *v1beta1.IngressConfig,
	isvc *v1beta1.InferenceService,
	isvcConfig *v1beta1.InferenceServicesConfig,
	componentType string,
	topLevelFlag bool,
	name string,
) (string, error) {
	metadata := generateMetadata(isvc, constants.InferenceServiceComponent(componentType), name, isvcConfig)
	if !topLevelFlag {
		return GenerateDomainName(metadata.Name, isvc.ObjectMeta, ingressConfig)
	} else {
		return GenerateDomainName(isvc.Name, isvc.ObjectMeta, ingressConfig)
	}
}

func createRawIngress(ctx context.Context, scheme *runtime.Scheme, isvc *v1beta1.InferenceService,
	ingressConfig *v1beta1.IngressConfig, client client.Client,
	isvcConfig *v1beta1.InferenceServicesConfig,
) (*netv1.Ingress, error) {
	if !isvc.Status.IsConditionReady(v1beta1.PredictorReady) {
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: "Predictor ingress not created",
		})
		return nil, nil
	}
	var rules []netv1.IngressRule
	existing := &corev1.Service{}
	predictorName := constants.PredictorServiceName(isvc.Name)
	switch {
	case isvc.Spec.Transformer != nil:
		if !isvc.Status.IsConditionReady(v1beta1.TransformerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Transformer ingress not created",
			})
			return nil, nil
		}
		transformerName := constants.TransformerServiceName(isvc.Name)
		explainerName := constants.ExplainerServiceName(isvc.Name)
		err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultTransformerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			transformerName = constants.DefaultTransformerServiceName(isvc.Name)
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
			explainerName = constants.DefaultExplainerServiceName(isvc.Name)
		}
		host, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Transformer), true, transformerName)
		if err != nil {
			return nil, fmt.Errorf("failed creating top level transformer ingress host: %w", err)
		}
		transformerHost, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Transformer), false, transformerName)
		if err != nil {
			return nil, fmt.Errorf("failed creating transformer ingress host: %w", err)
		}
		if isvc.Spec.Explainer != nil {
			explainerHost, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Explainer), false, transformerName)
			if err != nil {
				return nil, fmt.Errorf("failed creating explainer ingress host: %w", err)
			}
			rules = append(rules, generateRule(explainerHost, explainerName, "/", constants.CommonDefaultHttpPort))
		}
		// :predict routes to the transformer when there are both predictor and transformer
		rules = append(rules, generateRule(host, transformerName, "/", constants.CommonDefaultHttpPort))
		rules = append(rules, generateRule(transformerHost, predictorName, "/", constants.CommonDefaultHttpPort))
	case isvc.Spec.Explainer != nil:
		if !isvc.Status.IsConditionReady(v1beta1.ExplainerReady) {
			isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionFalse,
				Reason: "Explainer ingress not created",
			})
			return nil, nil
		}
		explainerName := constants.ExplainerServiceName(isvc.Name)
		err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultExplainerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			explainerName = constants.DefaultExplainerServiceName(isvc.Name)
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		}
		host, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Explainer), true, explainerName)
		if err != nil {
			return nil, fmt.Errorf("failed creating top level explainer ingress host: %w", err)
		}
		explainerHost, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Explainer), false, explainerName)
		if err != nil {
			return nil, fmt.Errorf("failed creating explainer ingress host: %w", err)
		}
		// :predict routes to the predictor when there is only predictor and explainer
		rules = append(rules, generateRule(host, predictorName, "/", constants.CommonDefaultHttpPort))
		rules = append(rules, generateRule(explainerHost, explainerName, "/", constants.CommonDefaultHttpPort))
	default:
		err := client.Get(ctx, types.NamespacedName{Name: constants.DefaultPredictorServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
		if err == nil {
			predictorName = constants.DefaultPredictorServiceName(isvc.Name)
		}
		host, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Predictor), true, predictorName)
		if err != nil {
			return nil, fmt.Errorf("failed creating top level predictor ingress host: %w", err)
		}
		rules = append(rules, generateRule(host, predictorName, "/", constants.CommonDefaultHttpPort))
	}
	// add predictor rule
	predictorHost, err := generateIngressHost(ingressConfig, isvc, isvcConfig, string(constants.Predictor), false, predictorName)
	if err != nil {
		return nil, fmt.Errorf("failed creating predictor ingress host: %w", err)
	}
	rules = append(rules, generateRule(predictorHost, predictorName, "/", constants.CommonDefaultHttpPort))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        isvc.ObjectMeta.Name,
			Namespace:   isvc.ObjectMeta.Namespace,
			Annotations: isvc.Annotations,
		},
		Spec: netv1.IngressSpec{
			IngressClassName: ingressConfig.IngressClassName,
			Rules:            rules,
		},
	}
	if err := controllerutil.SetControllerReference(isvc, ingress, scheme); err != nil {
		return nil, err
	}
	return ingress, nil
}

func semanticIngressEquals(desired, existing *netv1.Ingress) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec)
}
