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
	"cmp"
	"context"
	"fmt"
	"slices"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1b1ingress "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/ingress"
)

func (r *LLMISVCReconciler) reconcileIngress(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, ingressConfig *v1beta1.IngressConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Ingress")

	expectedIngress, err := r.expectedIngress(ctx, llmSvc, ingressConfig)
	if err != nil {
		return fmt.Errorf("failed to construct expected ingress: %w", err)
	}

	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Route != nil || llmSvc.Spec.Router.Route.HTTP != nil ||
		llmSvc.Spec.Router.Ingress == nil || llmSvc.Spec.Router.Ingress.Refs != nil {
		return Delete(ctx, r, llmSvc, expectedIngress)
	}

	referencedRoutes, err := r.collectReferencedIngresses(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to collect referenced ingress: %w", err)
	}

	ingress := llmSvc.Spec.Router.Ingress

	if ingress.HasRefs() {
		return Delete(ctx, r, llmSvc, expectedIngress)
	}

	if ingress != nil && !ingress.HasRefs() {
		if err := Reconcile(ctx, r, llmSvc, &netv1.Ingress{}, expectedIngress, semanticIngressIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile Ingress %s/%s: %w", expectedIngress.GetNamespace(), expectedIngress.GetName(), err)
		}
	}

	return r.updateRoutingStatusFromIngress(ctx, llmSvc, ingressConfig, referencedRoutes...)
}

func (r *LLMISVCReconciler) expectedIngress(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, ingressConfig *v1beta1.IngressConfig) (*netv1.Ingress, error) {
	host, err := v1b1ingress.GenerateDomainName(llmSvc.Name, llmSvc.ObjectMeta, ingressConfig)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to generate domain name for Ingress")
		return nil, err
	}

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name,
			Namespace: llmSvc.GetNamespace(),
			Labels:    RouterLabels(llmSvc),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Ingress != nil && llmSvc.Spec.Router.Ingress.Refs == nil {
		ingress.Spec.Rules = []netv1.IngressRule{
			{
				Host: host,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: ptr.To(netv1.PathTypePrefix),
							},
						},
					},
				},
			},
		}
	}

	return ingress, nil
}

func (r *LLMISVCReconciler) collectReferencedIngresses(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) ([]*netv1.Ingress, error) {
	var referencedIngress []*netv1.Ingress

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Ingress != nil {
		for _, ingressRef := range llmSvc.Spec.Router.Ingress.Refs {
			ingress := &netv1.Ingress{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: string(ingressRef.Namespace), Name: string(ingressRef.Name)}, ingress); err != nil {
				if apierrors.IsNotFound(err) {
					// TODO: mark condition if not found
					continue
				}
				return referencedIngress, fmt.Errorf("failed to get Ingress %s/%s: %w", ingressRef.Namespace, ingressRef.Name, err)
			}
			referencedIngress = append(referencedIngress, ingress)
		}
	}

	return referencedIngress, nil
}

func (r *LLMISVCReconciler) updateRoutingStatusFromIngress(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, ingressConfig *v1beta1.IngressConfig, ingresses ...*netv1.Ingress) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Updating routing status from ingress", "ingressCount", len(ingresses))

	hosts := extractIngressHostNames(ingresses)
	if len(hosts) == 0 {
		return fmt.Errorf("no ingress hosts found for llmisvc %s/%s", llmSvc.GetNamespace(), llmSvc.GetName())
	}
	scheme := ingressConfig.UrlScheme
	urls := make([]*apis.URL, 0, len(hosts))
	for _, host := range hosts {
		url := &apis.URL{
			Scheme: scheme,
			Host:   host,
		}
		urls = append(urls, url)
	}

	slices.SortStableFunc(urls, func(a, b *apis.URL) int {
		return cmp.Compare(a.String(), b.String())
	})
	llmSvc.Status.URL = urls[0]

	llmSvc.Status.Addresses = make([]duckv1.Addressable, 0, len(urls))
	for _, url := range urls {
		llmSvc.Status.Addresses = append(llmSvc.Status.Addresses, duckv1.Addressable{
			URL: url,
		})
	}
	return nil
}

func semanticIngressIsEqual(expected, actual *netv1.Ingress) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, actual.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, actual.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, actual.Annotations)
}

func extractIngressHostNames(ingresses []*netv1.Ingress) []string {
	var hostNames []string
	seen := make(map[string]struct{})
	for _, ingress := range ingresses {
		if ingress == nil {
			continue
		}
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == "" { // skip empty hosts
				continue
			}
			if _, exists := seen[rule.Host]; exists {
				continue
			}
			seen[rule.Host] = struct{}{}
			hostNames = append(hostNames, rule.Host)
		}
	}
	return hostNames
}
