package kservemodule

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"
)

var (
	kserveDeploymentsOCP = []string{
		kserveControllerDeployment,
		llmISVCControllerDeployment,
	}
	kserveDeploymentsXKS = []string{
		llmISVCControllerDeployment,
	}
	modelControllerDeploymentsOCP = []string{
		odhModelControllerDeployment,
	}
	wvaDeploymentsOCP = []string{
		wvaControllerDeployment,
	}
)

func NewDeployer() *deploy.Deployer {
	modelControllerGVK := schema.GroupVersionKind{Group: "components.platform.opendatahub.io", Version: "v1alpha1", Kind: "ModelController"}

	return deploy.NewDeployer(
		deploy.WithFieldOwner(fieldOwner),
		deploy.WithApplyOrder(),
		deploy.WithCache(),
		deploy.WithMergeStrategy(
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			deploy.MergeDeployments,
		),
		deploy.WithLegacyOwners(modelControllerGVK),
	)
}

func checkKServeReadiness(ctx context.Context, cli client.Client, namespace string, isXKS bool) error {
	if isXKS {
		return checkDeploymentsReady(ctx, cli, namespace, kserveDeploymentsXKS)
	}
	return checkDeploymentsReady(ctx, cli, namespace, kserveDeploymentsOCP)
}

func checkModelControllerReadiness(ctx context.Context, cli client.Client, namespace string, isXKS bool) error {
	if isXKS {
		return nil
	}
	return checkDeploymentsReady(ctx, cli, namespace, modelControllerDeploymentsOCP)
}

func checkWVAReadiness(ctx context.Context, cli client.Client, namespace string) error {
	return checkDeploymentsReady(ctx, cli, namespace, wvaDeploymentsOCP)
}

func (r *KserveModuleReconciler) defaultCleanup(ctx context.Context, comp componentConfig) error {
	log := ctrl.LoggerFrom(ctx)

	manifestDir, err := r.ensureWorkDir()
	if err != nil {
		return fmt.Errorf("preparing writable manifests for cleanup: %w", err)
	}

	sourcePath := comp.sourcePath
	if r.isKubernetes(ctx) {
		if comp.sourcePathXKS == "" {
			log.Info("no XKS overlay, skipping cleanup", "component", comp.name)
			return nil
		}
		sourcePath = comp.sourcePathXKS
	}

	renderPath := filepath.Join(manifestDir, comp.dirName(), sourcePath)
	if _, err := os.Stat(renderPath); os.IsNotExist(err) {
		log.Info("manifest directory not found, nothing to clean up", "component", comp.name, "path", renderPath)
		return nil
	}
	resources, err := kustomize.Render(renderPath, nil, kustomize.WithNamespace(r.getApplicationsNamespace()))
	if err != nil {
		return fmt.Errorf("rendering %s manifests for cleanup: %w", comp.name, err)
	}

	if comp.postRender != nil {
		resources, err = comp.postRender(ctx, r, nil, resources)
		if err != nil {
			return fmt.Errorf("post-render for cleanup %s: %w", comp.name, err)
		}
	}

	var errs []string
	for i := range resources {
		res := &resources[i]
		if res.GetKind() == "CustomResourceDefinition" {
			continue
		}
		key := client.ObjectKeyFromObject(res)
		if err := deleteResourceIfPresent(ctx, r.Client, res); err != nil {
			log.Error(err, "failed to delete resource during cleanup", "gvk", res.GroupVersionKind(), "key", key)
			errs = append(errs, fmt.Sprintf("%s %s: %v", res.GroupVersionKind().Kind, key, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup %s: %s", comp.name, strings.Join(errs, "; "))
	}

	log.Info("default cleanup completed", "component", comp.name, "resourceCount", len(resources))
	return nil
}

func deleteResourceIfPresent(ctx context.Context, cli client.Client, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	lookup := obj.DeepCopyObject().(client.Object)
	if err := cli.Get(ctx, key, lookup); err != nil {
		if client.IgnoreNotFound(err) == nil || meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to check %s %s: %w", obj.GetObjectKind().GroupVersionKind().Kind, key, err)
	}
	if err := cli.Delete(ctx, lookup); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		}
		return fmt.Errorf("failed to delete %s %s: %w", obj.GetObjectKind().GroupVersionKind().Kind, key, err)
	}
	return nil
}

func checkDeploymentsReady(ctx context.Context, cli client.Client, namespace string, deployments []string) error {
	var notReady []string
	for _, name := range deployments {
		dep := &appsv1.Deployment{}
		key := client.ObjectKey{Namespace: namespace, Name: name}
		if err := cli.Get(ctx, key, dep); err != nil {
			notReady = append(notReady, fmt.Sprintf("%s (get: %v)", name, err))
			continue
		}
		if dep.Status.AvailableReplicas < 1 {
			notReady = append(notReady, fmt.Sprintf("%s (availableReplicas=%d)", name, dep.Status.AvailableReplicas))
		}
	}

	if len(notReady) > 0 {
		return fmt.Errorf("deployments not ready: %s", strings.Join(notReady, ", "))
	}
	return nil
}
