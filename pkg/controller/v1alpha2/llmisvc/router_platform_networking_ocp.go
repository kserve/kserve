//go:build distro

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
	"slices"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	istionetworking "istio.io/api/networking/v1alpha3"
	istioapi "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	// istioInferencePoolLabelName is the label name that Istio uses to label the "shadow service" and reference the
	// associated InferencePool.
	istioInferencePoolLabelName = "istio.io/inferencepool-name"

	// llmManagedLabelKey marks resources managed by the LLM controller for Istio integration.
	llmManagedLabelKey = "llm-d.ai/managed"
)

// istioGatewayControllerNames lists the GatewayClass controller names that identify an Istio-based gateway.
var istioGatewayControllerNames = []string{
	"istio.io/gateway-controller",
	"istio.io/unmanaged-gateway",
	"openshift.io/gateway-controller/v1",
}

// IstioCACertificatePath is the path to the CA certificate used by Istio DestinationRules for TLS verification.
// Default is the OpenShift service-ca path. Override via ISTIO_CA_CERTIFICATE_PATH environment variable.
var IstioCACertificatePath = constants.GetEnvOrDefault("ISTIO_CA_CERTIFICATE_PATH", "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt")

func isIstioGatewayController(name string) bool {
	return slices.Contains(istioGatewayControllerNames, name)
}

func (r *LLMISVCReconciler) hasIstioGateway(ctx context.Context, routes []*gwapiv1.HTTPRoute) (bool, error) {
	for _, route := range routes {
		gateways, err := DiscoverGateways(ctx, r.Client, route)
		if err != nil {
			return false, err
		}
		for _, g := range gateways {
			if g.gatewayClass != nil && isIstioGatewayController(string(g.gatewayClass.Spec.ControllerName)) {
				return true, nil
			}
		}
	}
	return false, nil
}

// reconcileRouterPlatformNetworking configures Istio DestinationRules so the gateway can communicate with the
// scheduler and workload pods over TLS using self-signed certificates without injected sidecars.
func (r *LLMISVCReconciler) reconcileRouterPlatformNetworking(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	log.FromContext(ctx).Info("Reconciling Istio Destination Rules")

	if llmSvc.Spec.Router == nil {
		return r.reconcileIstioDestinationRules(ctx, llmSvc, false)
	}

	routes, err := r.collectReferencedRoutes(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to collect referenced routes: %w", err)
	}

	if llmSvc.Spec.Router.Route != nil && !llmSvc.Spec.Router.Route.HTTP.HasRefs() {
		routes = append(routes, r.expectedHTTPRoute(ctx, llmSvc))
	}

	isIstio, err := r.hasIstioGateway(ctx, routes)
	if err != nil {
		return fmt.Errorf("failed to discover gateways: %w", err)
	}

	return r.reconcileIstioDestinationRules(ctx, llmSvc, isIstio)
}

func (r *LLMISVCReconciler) reconcileIstioDestinationRules(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, isIstio bool) error {
	shouldDelete := !isIstio || utils.GetForceStopRuntime(llmSvc)
	if err := r.reconcileIstioDestinationRuleForScheduler(ctx, llmSvc, shouldDelete); err != nil {
		return fmt.Errorf("failed to reconcile Istio destination rule for scheduler: %w", err)
	}
	if err := r.reconcileIstioDestinationRuleForWorkload(ctx, llmSvc, shouldDelete); err != nil {
		return fmt.Errorf("failed to reconcile Istio destination rule for workload: %w", err)
	}
	if err := r.reconcileIstioDestinationRuleForShadowService(ctx, llmSvc, shouldDelete); err != nil {
		return fmt.Errorf("failed to reconcile Istio destination rule for shadow service: %w", err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileIstioDestinationRuleForShadowService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	expected, err := r.expectedIstioDestinationRuleForShadowService(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to get expected Istio destination rule for shadow service: %w", err)
	}
	if shouldDelete || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	if expected.Spec.GetHost() == "" {
		// Host is required. This happens when the shadow service hasn't been created yet.
		// The resource will get re-queued once the shadow service is created.
		log.FromContext(ctx).Info("Istio shadow service is not present yet, skipping DestinationRule reconciliation")
		return nil
	}
	return Reconcile(ctx, r, llmSvc, &istioapi.DestinationRule{}, expected, semanticDestinationRuleIsEqual)
}

func (r *LLMISVCReconciler) reconcileIstioDestinationRuleForWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	expected := r.expectedIstioDestinationRuleForWorkload(ctx, llmSvc)
	if shouldDelete || llmSvc.Spec.Router == nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &istioapi.DestinationRule{}, expected, semanticDestinationRuleIsEqual)
}

func (r *LLMISVCReconciler) reconcileIstioDestinationRuleForScheduler(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	expected, err := r.expectedIstioDestinationRuleForScheduler(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to get expected Istio destination rule for scheduler: %w", err)
	}
	if shouldDelete || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &istioapi.DestinationRule{}, expected, semanticDestinationRuleIsEqual)
}

func (r *LLMISVCReconciler) expectedIstioDestinationRuleForScheduler(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*istioapi.DestinationRule, error) {
	dr := &istioapi.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-scheduler"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesComponentLabelKey: "llminferenceservice-router-scheduler",
				constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
				llmManagedLabelKey:                    "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: istionetworking.DestinationRule{
			TrafficPolicy: &istionetworking.TrafficPolicy{
				Tls: &istionetworking.ClientTLSSettings{
					Mode: istionetworking.ClientTLSSettings_SIMPLE,
					// The scheduler doesn't support watching and auto-reloading certificates yet.
					// Fixed by https://github.com/kubernetes-sigs/gateway-api-inference-extension/pull/1765.
					// Until that fix is available, skip verification to avoid SAN mismatch errors on upgrade.
					InsecureSkipVerify: &wrapperspb.BoolValue{Value: true},
				},
			},
			ExportTo: []string{"*"},
		},
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil {
		name := llmSvc.Spec.Router.EPPServiceName(llmSvc)
		if llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
			pool, err := r.getInferencePool(ctx, llmSvc)
			if err != nil {
				return nil, err
			}
			if pool != nil && pool.Spec.EndpointPickerRef.Name != "" {
				name = string(pool.Spec.EndpointPickerRef.Name)
			}
		}
		hostname := network.GetServiceHostname(name, llmSvc.GetNamespace())
		dr.Spec.Host = hostname
		dr.Spec.TrafficPolicy.Tls.Sni = hostname
	}

	log.FromContext(ctx).V(2).Info("Expected destination rule for scheduler", "destinationrule", dr)

	return dr, nil
}

func (r *LLMISVCReconciler) expectedIstioDestinationRuleForShadowService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*istioapi.DestinationRule, error) {
	shadowSvc, err := r.getIstioShadowInferencePoolService(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to get istio inference pool service: %w", err)
	}

	dr := &istioapi.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-shadow-svc"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesComponentLabelKey: "llminferenceservice-shadow-service",
				constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
				llmManagedLabelKey:                    "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: istionetworking.DestinationRule{
			TrafficPolicy: &istionetworking.TrafficPolicy{
				Tls: &istionetworking.ClientTLSSettings{
					Mode: istionetworking.ClientTLSSettings_SIMPLE,
					// The shadow service forwards traffic to the workload service. Skip verification
					// here for the same reason as the scheduler DR — the scheduler doesn't yet support
					// watching and auto-reloading certificates.
					InsecureSkipVerify: &wrapperspb.BoolValue{Value: true},
				},
			},
			ExportTo: []string{"*"},
		},
	}
	if shadowSvc != nil {
		hostname := network.GetServiceHostname(shadowSvc.GetName(), shadowSvc.GetNamespace())
		dr.Spec.Host = hostname
		dr.Spec.TrafficPolicy.Tls.Sni = network.GetServiceHostname(kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"), llmSvc.GetNamespace())
	}

	log.FromContext(ctx).V(2).Info("Expected destination rule for workload shadow service", "destinationrule", dr)

	return dr, nil
}

func (r *LLMISVCReconciler) expectedIstioDestinationRuleForWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *istioapi.DestinationRule {
	hostname := network.GetServiceHostname(kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"), llmSvc.GetNamespace())
	dr := &istioapi.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
				constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
				llmManagedLabelKey:                    "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: istionetworking.DestinationRule{
			Host: hostname,
			TrafficPolicy: &istionetworking.TrafficPolicy{
				Tls: &istionetworking.ClientTLSSettings{
					Mode:               istionetworking.ClientTLSSettings_SIMPLE,
					CaCertificates:     IstioCACertificatePath,
					InsecureSkipVerify: &wrapperspb.BoolValue{Value: false},
					Sni:                hostname,
				},
			},
			ExportTo: []string{"*"},
		},
	}

	if llmSvc.Spec.Prefill != nil {
		// The sidecar doesn't support watching and auto-reloading certificates yet.
		dr.Spec.TrafficPolicy.Tls.CaCertificates = ""
		dr.Spec.TrafficPolicy.Tls.InsecureSkipVerify = &wrapperspb.BoolValue{Value: true}
	}

	log.FromContext(ctx).V(2).Info("Expected destination rule for workload service", "destinationrule", dr)

	return dr
}

func (r *LLMISVCReconciler) getIstioShadowInferencePoolService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*corev1.Service, error) {
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return nil, nil
	}

	svcs := &corev1.ServiceList{}
	err := r.List(ctx, svcs, client.InNamespace(llmSvc.GetNamespace()), client.MatchingLabels{
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

func (r *LLMISVCReconciler) getInferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*igwapi.InferencePool, error) {
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Pool == nil || !llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		return nil, nil
	}
	pool := &igwapi.InferencePool{}
	if err := r.Get(ctx, client.ObjectKey{Name: llmSvc.Spec.Router.Scheduler.Pool.Ref.Name, Namespace: llmSvc.GetNamespace()}, pool); err != nil {
		return nil, fmt.Errorf("failed to get inference pool %s/%s: %w", llmSvc.GetNamespace(), llmSvc.Spec.Router.Scheduler.Pool.Ref.Name, err)
	}
	return pool, nil
}

func semanticDestinationRuleIsEqual(expected *istioapi.DestinationRule, curr *istioapi.DestinationRule) bool {
	return cmp.Equal(&expected.Spec, &curr.Spec, protocmp.Transform()) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}
