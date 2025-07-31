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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

var childResourcesPredicate, _ = predicate.LabelSelectorPredicate(metav1.LabelSelector{
	MatchLabels: map[string]string{
		"app.kubernetes.io/part-of": "llminferenceservice",
	},
})

type Config struct {
	SystemNamespace             string   `json:"systemNamespace,omitempty"`
	IngressGatewayName          string   `json:"ingressGatewayName,omitempty"`
	IngressGatewayNamespace     string   `json:"ingressGatewayNamespace,omitempty"`
	IstioGatewayControllerNames []string `json:"istioGatewayControllerNames,omitempty"`

	StorageConfig *kserveTypes.StorageInitializerConfig `json:"-"`
}

//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices/finalizers,verbs=update
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=leaderworkerset.x-k8s.io,resources=leaderworkersets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;gateways;gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencepools;inferencemodels;,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews;subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main entry point for the reconciliation loop.
// It fetches the LLMInferenceService and delegates the reconciliation of its constituent parts.
type LLMISVCReconciler struct {
	client.Client
	record.EventRecorder
	Clientset kubernetes.Interface
}

// NewConfig creates an instance of llm-specific config based on predefined values
// in IngressConfig struct
func NewConfig(ingressConfig *v1beta1.IngressConfig, storageConfig *kserveTypes.StorageInitializerConfig) *Config {
	igwNs := constants.KServeNamespace
	igwName := ingressConfig.KserveIngressGateway
	igw := strings.Split(igwName, "/")
	if len(igw) == 2 {
		igwNs = igw[0]
		igwName = igw[1]
	}

	return &Config{
		SystemNamespace:         constants.KServeNamespace,
		IngressGatewayNamespace: igwNs,
		IngressGatewayName:      igwName,
		// TODO make it configurable
		IstioGatewayControllerNames: []string{
			"istio.io/gateway-controller",
			"istio.io/unmanaged-gateway",
			"openshift.io/gateway-controller",
		},
		StorageConfig: storageConfig,
	}
}

func LoadConfig(ctx context.Context, clientset kubernetes.Interface) (*Config, error) {
	isvcConfigMap, errCfgMap := v1beta1.GetInferenceServiceConfigMap(ctx, clientset) // Fetch directly from API Server
	if errCfgMap != nil {
		return nil, fmt.Errorf("failed to load InferenceServiceConfigMap: %w", errCfgMap)
	}

	ingressConfig, errConvert := v1beta1.NewIngressConfig(isvcConfigMap)
	if errConvert != nil {
		return nil, fmt.Errorf("failed to convert InferenceServiceConfigMap to IngressConfig: %w", errConvert)
	}

	storageInitializerConfig, errConvert := v1beta1.GetStorageInitializerConfigs(isvcConfigMap)
	if errConvert != nil {
		return nil, fmt.Errorf("failed to convert InferenceServiceConfigMap to StorageInitializerConfig: %w", errConvert)
	}

	return NewConfig(ingressConfig, storageInitializerConfig), nil
}

func (r *LLMISVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling LLMISVC", "name", req.Name, "namespace", req.Namespace)
	// Implement the reconciliation logic here
	original := &v1alpha1.LLMInferenceService{}
	if err := r.Get(ctx, req.NamespacedName, original); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizerName := constants.KServeAPIGroupName + "/llmisvc-finalizer"
	if original.DeletionTimestamp.IsZero() {
		if controllerutil.AddFinalizer(original, finalizerName) {
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		logf.Log.Info("Marked for deletion, finalizing resources")
		if controllerutil.ContainsFinalizer(original, finalizerName) {
			if cleanupErr := r.finalize(ctx, original); cleanupErr != nil {
				logf.Log.Error(cleanupErr, "Finalization failed")
				return ctrl.Result{}, cleanupErr
			}

			controllerutil.RemoveFinalizer(original, finalizerName)
			if err := r.Update(ctx, original); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Do not reconcile, because llmisvc is being deleted.
		return ctrl.Result{}, nil
	}

	resource := original.DeepCopy()

	// TODO: add pre-process and post-process reconciliation and re-enable this code
	// reconciler.PreProcessReconcile(ctx, resource)
	// reconciler.PostProcessReconcile(ctx, resource, original)
	reconcileErr := r.reconcile(ctx, resource)

	if reconcileErr != nil {
		logf.Log.Error(reconcileErr, "Failed to reconcile LLMInferenceService")
		r.Eventf(original, corev1.EventTypeWarning, "Error", "Reconciliation failed: %v", reconcileErr.Error())
	}

	if err := r.updateStatus(ctx, resource); err != nil {
		logf.Log.Error(err, "Failed to update status for LLMInferenceService")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, reconcileErr
}

func (r *LLMISVCReconciler) reconcile(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := logf.FromContext(ctx).WithName("reconcile")
	ctx = logf.IntoContext(ctx, logger)

	// TODO(ctrl): add watch on CfgMap with predicate and cache tuning to trigger reconcile when it changes
	config, configErr := LoadConfig(ctx, r.Clientset)
	if configErr != nil {
		return fmt.Errorf("failed to load ingress config: %w", configErr)
	}

	baseCfg, err := r.combineBaseRefsConfig(ctx, llmSvc, config)
	if err != nil {
		llmSvc.MarkPresetsCombinedNotReady("CombineBaseError", err.Error())
		return fmt.Errorf("failed to combine base-configurations: %w", err)
	}
	llmSvc.MarkPresetsCombinedReady()
	// We are only writing to status, so we can safely use the original object.
	llmSvc.Spec = baseCfg.Spec

	logger.Info("Reconciling with combined base configurations", "spec", llmSvc.Spec)

	// TODO: add workload reconciliation and re-enable this code
	// if err := r.reconcileWorkload(ctx, llmSvc, config.StorageConfig); err != nil {
	// 	return fmt.Errorf("failed to reconcile workload: %w", err)
	// }

	// TODO: add router reconciliation and re-enable this code
	// if err := r.reconcileRouter(ctx, llmSvc); err != nil {
	// 	return fmt.Errorf("failed to reconcile networking: %w", err)
	// }

	return nil
}

func (r *LLMISVCReconciler) finalize(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	// TODO: add scheduler service account finalization and re-enable this code
	// if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
	// 	return fmt.Errorf("failed to finalize scheduler service account: %w", err)
	// }

	return nil
}

func (r *LLMISVCReconciler) updateStatus(ctx context.Context, desired *v1alpha1.LLMInferenceService) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1alpha1.LLMInferenceService{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(desired), latest); err != nil {
			return err
		}

		if equality.Semantic.DeepEqual(latest.Status, desired.Status) {
			return nil
		}

		latest.Status = desired.Status

		if err := r.Client.Status().Update(ctx, latest); err != nil {
			return fmt.Errorf("failed to update status for LLMInferenceService: %w", err)
		}

		return nil
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMISVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LLMInferenceService{}).
		Complete(r)
}
