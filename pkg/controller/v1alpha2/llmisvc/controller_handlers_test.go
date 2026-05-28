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
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestHasRoutingGatewayRef(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		Status: v1alpha2.LLMInferenceServiceStatus{
			Router: &v1alpha2.RouterStatus{
				Gateways: []v1alpha2.ObservedGateway{
					{
						ObjectReference: gwapiv1.ObjectReference{
							Name:      "gateway-a",
							Namespace: ptr.To(gwapiv1.Namespace("networking")),
						},
					},
					{
						ObjectReference: gwapiv1.ObjectReference{
							Name:      "gateway-b",
							Namespace: ptr.To(gwapiv1.Namespace("kserve")),
						},
					},
				},
			},
		},
	}

	g.Expect(hasRoutingGatewayRef(llmSvc, gwapiv1.ObjectName("gateway-b"), gwapiv1.Namespace("kserve"))).To(BeTrue())
	g.Expect(hasRoutingGatewayRef(llmSvc, gwapiv1.ObjectName("gateway-b"), gwapiv1.Namespace("networking"))).To(BeFalse())
	g.Expect(hasRoutingGatewayRef(llmSvc, gwapiv1.ObjectName("gateway-missing"), gwapiv1.Namespace("kserve"))).To(BeFalse())
}

func TestHasRoutingGatewayRefReturnsFalseWithoutObservedGateways(t *testing.T) {
	g := NewGomegaWithT(t)

	g.Expect(hasRoutingGatewayRef(&v1alpha2.LLMInferenceService{}, gwapiv1.ObjectName("gateway"), gwapiv1.Namespace("default"))).To(BeFalse())
	g.Expect(hasRoutingGatewayRef(&v1alpha2.LLMInferenceService{
		Status: v1alpha2.LLMInferenceServiceStatus{
			Router: &v1alpha2.RouterStatus{},
		},
	}, gwapiv1.ObjectName("gateway"), gwapiv1.Namespace("default"))).To(BeFalse())
}

func TestHasRoutingHTTPRouteRef(t *testing.T) {
	g := NewGomegaWithT(t)

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "routing",
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Router: &v1alpha2.RouterStatus{
				Gateways: []v1alpha2.ObservedGateway{
					{
						HTTPRoutes: []gwapiv1.ObjectReference{
							{
								Name:      "llm-route-missing",
								Kind:      "HTTPRoute",
								Namespace: ptr.To(gwapiv1.Namespace("routing")),
							},
							{
								Name:      "llm-route-match",
								Kind:      "HTTPRoute",
								Namespace: ptr.To(gwapiv1.Namespace("routing")),
							},
						},
					},
				},
			},
		},
	}

	g.Expect(hasRoutingHTTPRouteRef(llmSvc, gwapiv1.ObjectName("llm-route-match"), "routing")).To(BeTrue())
	g.Expect(hasRoutingHTTPRouteRef(llmSvc, gwapiv1.ObjectName("llm-route-match"), "other")).To(BeFalse())
}

func TestHasRoutingHTTPRouteRefReturnsFalseWithoutObservedRoutes(t *testing.T) {
	g := NewGomegaWithT(t)

	g.Expect(hasRoutingHTTPRouteRef(&v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "routing",
		},
	}, gwapiv1.ObjectName("llm-route"), "routing")).To(BeFalse())
}

func TestSetRoutingPoolStatusOnlyWritesWhenRoutingExists(t *testing.T) {
	g := NewGomegaWithT(t)
	llmSvc := &v1alpha2.LLMInferenceService{}
	poolRef := gwapiv1.ObjectReference{
		Group: "inference.networking.k8s.io",
		Kind:  "InferencePool",
		Name:  "managed-pool",
	}
	svcRef := gwapiv1.ObjectReference{
		Kind: "Service",
		Name: "epp-service",
	}

	setRoutingPoolStatus(llmSvc, poolRef, svcRef)
	g.Expect(llmSvc.Status.Router).To(BeNil())

	llmSvc.Status.Router = &v1alpha2.RouterStatus{
		Gateways: []v1alpha2.ObservedGateway{
			{
				ObjectReference: gwapiv1.ObjectReference{
					Group: "gateway.networking.k8s.io",
					Kind:  "Gateway",
					Name:  "kserve-gateway",
				},
			},
		},
	}
	setRoutingPoolStatus(llmSvc, poolRef, svcRef)
	g.Expect(llmSvc.Status.Router).ToNot(BeNil())
	g.Expect(llmSvc.Status.Router.Scheduler.InferencePool).ToNot(BeNil())
	g.Expect(string(llmSvc.Status.Router.Scheduler.InferencePool.Name)).To(Equal("managed-pool"))
	g.Expect(llmSvc.Status.Router.Scheduler.Service).ToNot(BeNil())
	g.Expect(string(llmSvc.Status.Router.Scheduler.Service.Name)).To(Equal("epp-service"))
}

func TestRequestsForInferencePoolChangeMatchesExternalPoolRefs(t *testing.T) {
	g := NewGomegaWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	llmSvcWithMatch := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-with-match",
			Namespace: "routing",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker: &corev1.PodSpec{},
			},
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "shared-pool"},
					},
				},
			},
		},
	}

	llmSvcWithoutMatch := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-without-match",
			Namespace: "routing",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker: &corev1.PodSpec{},
			},
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "another-pool"},
					},
				},
			},
		},
	}

	reconciler := &LLMISVCReconciler{
		Client: clientfake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(llmSvcWithMatch, llmSvcWithoutMatch).
			Build(),
		Clientset: kubefake.NewSimpleClientset(testInferenceServiceConfigMap()),
		Validator: func(context.Context, *v1alpha2.LLMInferenceService) error {
			return nil
		},
	}

	pool := &igwapi.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-pool",
			Namespace: "routing",
		},
	}

	reqs := enqueueRequestsForObject(
		reconciler.enqueueOnInferencePoolChange(logr.Discard()),
		pool,
	)

	g.Expect(reqs).To(ConsistOf(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "routing",
			Name:      "svc-with-match",
		},
	}))
}

func TestRequestsForInferencePoolChangeIncludesRefsEvenWhenPoolIsOwned(t *testing.T) {
	g := NewGomegaWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ownerSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-owner",
			Namespace: "routing",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker: &corev1.PodSpec{},
			},
		},
	}

	consumerSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-consumer",
			Namespace: "routing",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker: &corev1.PodSpec{},
			},
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "managed-pool"},
					},
				},
			},
		},
	}

	reconciler := &LLMISVCReconciler{
		Client: clientfake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ownerSvc, consumerSvc).
			Build(),
		Clientset: kubefake.NewSimpleClientset(testInferenceServiceConfigMap()),
		Validator: func(context.Context, *v1alpha2.LLMInferenceService) error {
			return nil
		},
	}

	pool := &igwapi.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-pool",
			Namespace: "routing",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.LLMInferenceServiceGVK.Kind,
					Name:       "svc-owner",
					Controller: ptr.To(true),
				},
			},
		},
	}

	reqs := enqueueRequestsForObject(
		reconciler.enqueueOnInferencePoolChange(logr.Discard()),
		pool,
	)
	g.Expect(reqs).To(ConsistOf(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "routing",
			Name:      "svc-consumer",
		},
	}))
}

func enqueueRequestsForObject(eventHandler handler.EventHandler, object client.Object) []reconcile.Request {
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer queue.ShutDown()

	eventHandler.Create(context.Background(), event.CreateEvent{Object: object}, queue)

	reqs := make([]reconcile.Request, 0, queue.Len())
	for queue.Len() > 0 {
		req, shutdown := queue.Get()
		if shutdown {
			break
		}
		reqs = append(reqs, req)
		queue.Done(req)
		queue.Forget(req)
	}

	return reqs
}

func testInferenceServiceConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Data: map[string]string{
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"]
			}`,
			"storageInitializer": `{
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"cpuModelcar": "10m",
				"memoryModelcar": "15Mi",
				"enableModelcar": true
			}`,
			"credentials": `{
				"s3": {
					"s3AccessKeyIDName": "AWS_ACCESS_KEY_ID",
					"s3SecretAccessKeyName": "AWS_SECRET_ACCESS_KEY"
				}
			}`,
		},
	}
}
