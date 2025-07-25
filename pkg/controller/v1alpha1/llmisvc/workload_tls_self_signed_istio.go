/*
Copyright 2025 The KServe Authors.

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

package llmisvc

import (
	"context"
	"fmt"

	pbwrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	istionetworking "istio.io/api/networking/v1alpha3"
	istioapi "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

const (
	// istioInferencePoolLabelName is the label name that Istio uses to label the "shadow service" and reference the
	// associated InferencePool.
	istioInferencePoolLabelName = "istio.io/inferencepool-name"
)

// reconcileIstioDestinationRules configures Istio to allow the Gateway to communicate with the scheduler and the
// workload pods with TLS using self-signed certificates without injected sidecars.
func (r *LLMISVCReconciler) reconcileIstioDestinationRules(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	log.FromContext(ctx).Info("Reconciling Istio Destination Rules")

	if llmSvc.Spec.Router == nil {
		// Delete destination rules.

		if err := r.reconcileIstioDestinationRuleForScheduler(ctx, llmSvc); err != nil {
			return fmt.Errorf("failed to reconcile Istio destination rule for scheduler: %w", err)
		}
		if err := r.reconcileIstioDestinationRuleForWorkload(ctx, llmSvc); err != nil {
			return fmt.Errorf("failed to reconcile Istio destination rule for workload: %w", err)
		}
	}

	routes, err := r.collectReferencedRoutes(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to collect referenced routes: %w", err)
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Route != nil && !llmSvc.Spec.Router.Route.HTTP.HasRefs() {
		routes = append(routes, r.expectedHTTPRoute(ctx, llmSvc))
	}

	cfg, err := LoadConfig(ctx, r.Clientset)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	for _, route := range routes {
		gateways, err := DiscoverGateways(ctx, r.Client, route)
		if err != nil {
			return fmt.Errorf("failed to discover gateways : %w", err)
		}
		for _, g := range gateways {
			if g.gatewayClass != nil && cfg.isIstioGatewayController(string(g.gatewayClass.Spec.ControllerName)) {
				if err := r.reconcileIstioDestinationRuleForScheduler(ctx, llmSvc); err != nil {
					return fmt.Errorf("failed to reconcile Istio destination rule for scheduler: %w", err)
				}
				if err := r.reconcileIstioDestinationRuleForWorkload(ctx, llmSvc); err != nil {
					return fmt.Errorf("failed to reconcile Istio destination rule for workload: %w", err)
				}
				return nil
			}
		}
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileIstioDestinationRuleForWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	expected, err := r.expectedIstioDestinationRuleForWorkload(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to get expected Istio destination rule for workload: %w", err)
	}
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &istioapi.DestinationRule{}, expected, semanticDestinationRuleIsEqual)
}

func (r *LLMISVCReconciler) reconcileIstioDestinationRuleForScheduler(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	expected, err := r.expectedIstioDestinationRuleForScheduler(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to get expected Istio destination rule for scheduler: %w", err)
	}
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &istioapi.DestinationRule{}, expected, semanticDestinationRuleIsEqual)
}

func (r *LLMISVCReconciler) expectedIstioDestinationRuleForScheduler(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) (*istioapi.DestinationRule, error) {
	dr := &istioapi.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-scheduler"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/component": "llminferenceservice-router-scheduler",
				"app.kubernetes.io/name":      llmSvc.GetName(),
				"app.kubernetes.io/part-of":   "llminferenceservice",
				"llm-d.ai/managed":            "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
		Spec: istionetworking.DestinationRule{
			TrafficPolicy: &istionetworking.TrafficPolicy{
				Tls: &istionetworking.ClientTLSSettings{
					Mode:               istionetworking.ClientTLSSettings_SIMPLE,
					InsecureSkipVerify: &pbwrappers.BoolValue{Value: true},
				},
			},
			// Export to all namespaces, this is the default, however, we keep the configuration explicit.
			ExportTo: []string{"*"},
		},
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil {
		name := llmSvc.Spec.Router.EPPServiceName(llmSvc)
		if llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
			pool := igwapi.InferencePool{}
			if err := r.Client.Get(ctx, client.ObjectKey{Name: llmSvc.Spec.Router.Scheduler.Pool.Ref.Name, Namespace: llmSvc.GetNamespace()}, &pool); err != nil {
				return nil, fmt.Errorf("failed to get inference pool %s/%s: %w", llmSvc.GetNamespace(), llmSvc.Spec.Router.Scheduler.Pool.Ref.Name, err)
			}
			if pool.Spec.ExtensionRef != nil {
				name = string(pool.Spec.ExtensionRef.Name)
			}
		}
		dr.Spec.Host = network.GetServiceHostname(name, llmSvc.GetNamespace())
	}

	log.FromContext(ctx).V(2).Info("Expected destination rule for scheduler", "destinationrule", dr)

	return dr, nil
}

func (r *LLMISVCReconciler) expectedIstioDestinationRuleForWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) (*istioapi.DestinationRule, error) {
	shadowSvc, err := r.getIstioShadowInferencePoolService(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to get istio inference pool service: %w", err)
	}

	dr := &istioapi.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-workload"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/component": "llminferenceservice-workload",
				"app.kubernetes.io/name":      llmSvc.GetName(),
				"app.kubernetes.io/part-of":   "llminferenceservice",
				"llm-d.ai/managed":            "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
		Spec: istionetworking.DestinationRule{
			TrafficPolicy: &istionetworking.TrafficPolicy{
				Tls: &istionetworking.ClientTLSSettings{
					Mode:               istionetworking.ClientTLSSettings_SIMPLE,
					InsecureSkipVerify: &pbwrappers.BoolValue{Value: true},
				},
			},
			// Export to all namespaces, this is the default, however, we keep the configuration explicit.
			ExportTo: []string{"*"},
		},
	}
	if shadowSvc != nil {
		dr.Spec.Host = network.GetServiceHostname(shadowSvc.GetName(), shadowSvc.GetNamespace())
	}

	log.FromContext(ctx).V(2).Info("Expected destination rule for workload", "destinationrule", dr)

	return dr, nil
}

func (r *LLMISVCReconciler) getIstioShadowInferencePoolService(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) (*corev1.Service, error) {
	if llmSvc.Spec.Router == nil {
		return nil, nil
	}

	svcs := &corev1.ServiceList{}
	err := r.Client.List(ctx, svcs, client.InNamespace(llmSvc.GetNamespace()), client.MatchingLabels{
		istioInferencePoolLabelName: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services in namespace %q: %w", llmSvc.GetNamespace(), err)
	}
	if len(svcs.Items) == 0 {
		return nil, nil
	}
	return &svcs.Items[0], nil
}

func semanticDestinationRuleIsEqual(expected *istioapi.DestinationRule, curr *istioapi.DestinationRule) bool {
	return cmp.Equal(&expected.Spec, &curr.Spec, protocmp.Transform()) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}
