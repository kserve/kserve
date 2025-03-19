package aigateway

import (
	"context"
	"fmt"

	aigwv1a1 "github.com/envoyproxy/ai-gateway/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

type AIServiceBackendReconciler struct {
	client client.Client
	isvc   *v1beta1.InferenceService
	log logr.Logger
}

func NewAIServiceBackendReconciler(client client.Client, isvc *v1beta1.InferenceService, logger logr.Logger) *AIServiceBackendReconciler {
	return &AIServiceBackendReconciler{
		client: client,
		isvc:   isvc,
		log: logger,
	}
}

func (r *AIServiceBackendReconciler) Reconcile(ctx context.Context) error {
	desired := r.createAIServiceBackend()
	if err := controllerutil.SetControllerReference(r.isvc, desired, r.client.Scheme()); err != nil {
		r.log.Error(err, "Failed to set controller reference for AIServiceBackend", "name", desired.Name, "namespace", desired.Namespace)
	}
	existing := &aigwv1a1.AIServiceBackend{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: r.isvc.Namespace, Name: desired.Name}, existing); err != nil {
		if apierr.IsNotFound(err) {
			r.log.Info("Creating AIServiceBackend", "name", desired.Name, "namespace", desired.Namespace)
			if err := r.client.Create(ctx, desired); err != nil {
				r.log.Error(err, "Failed to create AIServiceBackend", "name", desired.Name, "namespace", desired.Namespace)
				return err
			}
			return nil
		}
		return err
	}

	// Set ResourceVersion which is required for update operation.
	desired.ResourceVersion = existing.ResourceVersion
	// Do a dry-run update to avoid diffs generated by default values.
	// This will populate our local httpRoute with any default values that are present on the remote version.
	if err := r.client.Update(ctx, desired, client.DryRunAll); err != nil {
		r.log.Error(err, "Failed to perform dry-run update for AIServiceBackend", "name", desired.Name, "namespace", desired.Namespace)
		return err
	}
	if !equality.Semantic.DeepEqual(desired.Spec, existing.Spec) {
		r.log.Info("Updating AIServiceBackend", "name", desired.Name, "namespace", desired.Namespace)
		if err := r.client.Update(ctx, desired); err != nil {
			r.log.Error(err, "Failed to update AIServiceBackend", "name", desired.Name, "namespace", desired.Namespace)
		}
	}
	return nil
}

func (r *AIServiceBackendReconciler) createAIServiceBackend() (*aigwv1a1.AIServiceBackend) {
	serviceName := constants.PredictorServiceName(r.isvc.Name)
	if r.isvc.Spec.Transformer != nil {
		serviceName = constants.TransformerServiceName(r.isvc.Name)
	}
	aiServiceBackend := &aigwv1a1.AIServiceBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.isvc.Name,
			Namespace: r.isvc.Namespace,
		},
		Spec: aigwv1a1.AIServiceBackendSpec{
			APISchema: aigwv1a1.VersionedAPISchema{
				Name: aigwv1a1.APISchemaOpenAI,
			},
			BackendRef: gwapiv1.BackendObjectReference{
				Kind: ptr.To(gwapiv1.Kind(constants.KindService)),
				Name: gwapiv1.ObjectName(serviceName),
				Namespace: ptr.To(gwapiv1.Namespace(r.isvc.Namespace)),
				Port: ptr.To(gwapiv1.PortNumber(constants.CommonDefaultHttpPort)),
			},
			Timeouts: &gwapiv1.HTTPRouteTimeouts{
				Request: ptr.To(gwapiv1.Duration(fmt.Sprintf("%ds", constants.DefaultTimeoutSeconds))),
			},
		},
	}
	return aiServiceBackend
}
