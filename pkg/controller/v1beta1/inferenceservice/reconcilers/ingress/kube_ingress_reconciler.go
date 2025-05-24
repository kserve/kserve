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

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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

	if !utils.GetForceStopRuntime(isvc) {
		if !isInternal && !r.ingressConfig.DisableIngressCreation {
			ingress, err := createRawIngress(r.scheme, isvc, r.ingressConfig, r.isvcConfig)
			if ingress == nil {
				return nil
			}
			if err != nil {
				return err
			}

			// reconcile ingress
			existingIngress := &netv1.Ingress{}
			err = r.client.Get(ctx, types.NamespacedName{
				Namespace: isvc.Namespace,
				Name:      isvc.Name,
			}, existingIngress)
			if getExistingErr != nil {
				if apierr.IsNotFound(getExistingErr) {
					err = r.client.Create(ctx, ingress)
					log.Info("creating ingress", "ingressName", isvc.Name, "err", err)
				} else {
					return getExistingErr
				}
			} else {
				if !semanticIngressEquals(ingress, existingIngress) {
					err = r.client.Update(ctx, ingress)
					log.Info("updating ingress", "ingressName", isvc.Name, "err", err)
				}
			}
			if err != nil {
				return err
			}
		}

		isvc.Status.URL, err = createRawURL(isvc, r.ingressConfig)
		if err != nil {
			return err
		}
		isvc.Status.Address = &duckv1.Addressable{
			URL: &apis.URL{
				Host:   getRawServiceHost(isvc),
				Scheme: r.ingressConfig.UrlScheme,
				Path:   "",
			},
		}
		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionTrue,
		})
	} else {
		// Delete the ingress
		if getExistingErr == nil {
			// Make sure that we only delete the ingress owned by the isvc
			if existingIngress.OwnerReferences[0].UID == isvc.UID && existingIngress.GetDeletionTimestamp() == nil {
				log.Info("The InferenceService ", isvc.Name, " is marked as stopped — delete its associated http route")
				if err := r.client.Delete(ctx, existingIngress); err != nil {
					return err
				}
			}
		} else if !apierr.IsNotFound(getExistingErr) {
			return getExistingErr
		}

		isvc.Status.SetCondition(v1beta1.IngressReady, &apis.Condition{
			Type:   v1beta1.IngressReady,
			Status: corev1.ConditionFalse,
			Reason: constants.StoppedISVCReason,
		})
	}
	return nil
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
		return !utils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
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

func createRawIngress(scheme *runtime.Scheme, isvc *v1beta1.InferenceService,
	ingressConfig *v1beta1.IngressConfig, isvcConfig *v1beta1.InferenceServicesConfig,
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
