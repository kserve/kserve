package kservemodule

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhLabels "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"

	platformv1alpha1 "github.com/opendatahub-io/kserve-module/pkg/apis/v1alpha1"
)

type componentConfig struct {
	name          string
	manifestName  string // overrides name for manifest directory lookup; defaults to name if empty
	sourcePath    string
	sourcePathXKS string
	imageMap      map[string]string
	extraParams   func(kserve *platformv1alpha1.Kserve) map[string]string
	postRender    func(ctx context.Context, r *KserveModuleReconciler,
		kserve *platformv1alpha1.Kserve,
		resources []unstructured.Unstructured) ([]unstructured.Unstructured, error)
	enabled      func(kserve *platformv1alpha1.Kserve) bool
	extraCleanup func(ctx context.Context, r *KserveModuleReconciler) error
}

func (c componentConfig) dirName() string {
	if c.manifestName != "" {
		return c.manifestName
	}
	return c.name
}

var components = []componentConfig{
	{
		name:          KserveComponentName,
		sourcePath:    KserveManifestSourcePath,
		sourcePathXKS: KserveManifestSourcePathXKS,
		imageMap:      kserveImageParamMap,
		postRender:    kservePostRender,
	},
	{
		name:        OdhModelControllerComponentName,
		sourcePath:  ModelControllerSourcePath,
		imageMap:    modelControllerImageParamMap,
		extraParams: modelControllerExtraParams,
		postRender:  modelControllerPostRender,
	},
	{
		name:       WVAComponentName,
		sourcePath: WVAManifestSourcePathOCP,
		imageMap:   wvaImageParamMap,
		enabled:    isWVAEnabled,
		postRender: wvaPostRender,
	},
	{
		name:         ModelCacheComponentName,
		manifestName: KserveComponentName,
		sourcePath:   ModelCacheManifestSourcePath,
		imageMap:     kserveImageParamMap,
		enabled:      isModelCacheEnabled,
		postRender:   modelCacheComponentPostRender,
		extraCleanup: cleanupModelCacheComponent,
	},
	{
		// No enabled func: CRD check requires API client, handled in postRender.
		name:         ObservabilityComponentName,
		manifestName: KserveComponentName,
		sourcePath:   ObservabilityManifestSourcePath,
		imageMap:     map[string]string{},
		postRender:   observabilityPostRender,
	},
	{
		// No enabled func: namespace check requires API client, handled in postRender.
		name:         ConsoleDashboardsComponentName,
		manifestName: KserveComponentName,
		sourcePath:   ConsoleDashboardsManifestSourcePath,
		imageMap:     map[string]string{},
		postRender:   consoleDashboardsPostRender,
	},
}

func kservePostRender(ctx context.Context, r *KserveModuleReconciler,
	kserve *platformv1alpha1.Kserve,
	resources []unstructured.Unstructured) ([]unstructured.Unstructured, error) {

	log := ctrl.LoggerFrom(ctx)
	beforeCount := len(resources)
	resources = filterFastResources(resources)
	if afterCount := len(resources); beforeCount != afterCount {
		log.Info("filtered fast-variant resources", "before", beforeCount, "after", afterCount, "removed", beforeCount-afterCount)
	}

	resources, err := customizeKserveConfigMap(resources, kserve)
	if err != nil {
		return nil, fmt.Errorf("customizing configmap: %w", err)
	}

	versionPrefix := r.getVersionPrefix(ctx, kserve)
	resources, err = versionedWellKnownLLMInferenceServiceConfigs(resources, versionPrefix)
	if err != nil {
		return nil, fmt.Errorf("versioning LLMInferenceServiceConfigs: %w", err)
	}

	return resources, nil
}

func wvaPostRender(ctx context.Context, r *KserveModuleReconciler,
	kserve *platformv1alpha1.Kserve,
	resources []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
	return filterOutNamespaces(resources), nil
}

func filterOutNamespaces(resources []unstructured.Unstructured) []unstructured.Unstructured {
	filtered := make([]unstructured.Unstructured, 0, len(resources))
	for i := range resources {
		if resources[i].GetKind() == "Namespace" {
			continue
		}
		filtered = append(filtered, resources[i])
	}
	return filtered
}

func observabilityPostRender(ctx context.Context, r *KserveModuleReconciler,
	_ *platformv1alpha1.Kserve,
	resources []unstructured.Unstructured) ([]unstructured.Unstructured, error) {

	log := ctrl.LoggerFrom(ctx)

	reasons := r.checkCRD(ctx, dependencyCheck{
		name:    "PersesDashboard",
		crdName: "persesdashboards.perses.dev",
	})
	if len(reasons) > 0 {
		log.Info("PersesDashboard CRD not available, skipping observability dashboards")
		return nil, nil
	}

	ns := r.getMonitoringNamespace()
	for i := range resources {
		resources[i].SetNamespace(ns)
	}
	log.Info("set monitoring namespace on observability resources", "namespace", ns, "count", len(resources))

	return resources, nil
}

func consoleDashboardsPostRender(ctx context.Context, r *KserveModuleReconciler,
	kserve *platformv1alpha1.Kserve,
	resources []unstructured.Unstructured) ([]unstructured.Unstructured, error) {

	log := ctrl.LoggerFrom(ctx)

	if kserve == nil || !ptr.Deref(kserve.Spec.EnableLLMInferenceServiceConsoleDashboards, true) {
		log.Info("EnableLLMInferenceServiceConsoleDashboards is disabled, skipping console dashboards")
		return nil, nil
	}

	ns := &corev1.Namespace{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: consoleDashboardsNamespace}, ns); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("namespace not found, skipping console dashboards", "namespace", consoleDashboardsNamespace)
			return nil, nil
		}
		return nil, fmt.Errorf("checking namespace %s: %w", consoleDashboardsNamespace, err)
	}

	for i := range resources {
		if kind := resources[i].GetKind(); kind != "ConfigMap" {
			return nil, fmt.Errorf("unauthorized resource kind %s in console dashboards manifest", kind)
		}
		resources[i].SetNamespace(consoleDashboardsNamespace)
	}
	log.Info("set namespace on console dashboard resources", "namespace", consoleDashboardsNamespace, "count", len(resources))

	return resources, nil
}

func isWVAEnabled(kserve *platformv1alpha1.Kserve) bool {
	return kserve.Spec.WVA.ManagementState == common.Managed
}

func modelControllerExtraParams(kserve *platformv1alpha1.Kserve) map[string]string {
	nimState := string(common.Removed)
	modelRegistryState := string(common.Removed)
	if platformv1alpha1.GetManagementState(kserve) == common.Managed {
		nimState = string(kserve.Spec.NIM.ManagementState)
		if nimState == "" {
			nimState = string(common.Managed)
		}
		modelRegistryState = string(kserve.Spec.ModelRegistry.ManagementState)
		if modelRegistryState == "" {
			modelRegistryState = string(common.Removed)
		}
	}
	return map[string]string{
		"nim-state":           strings.ToLower(nimState),
		"kserve-state":        strings.ToLower(string(platformv1alpha1.GetManagementState(kserve))),
		"modelregistry-state": strings.ToLower(modelRegistryState),
	}
}

func modelControllerPostRender(ctx context.Context, _ *KserveModuleReconciler,
	_ *platformv1alpha1.Kserve,
	resources []unstructured.Unstructured) ([]unstructured.Unstructured, error) {

	log := ctrl.LoggerFrom(ctx)
	beforeCount := len(resources)
	result := filterFastResources(resources)
	if afterCount := len(result); beforeCount != afterCount {
		log.Info("filtered fast-variant resources", "before", beforeCount, "after", afterCount, "removed", beforeCount-afterCount)
	}
	return result, nil
}

func commonPostRender(resources []unstructured.Unstructured, componentName string) {
	applyManagedByLabel(resources, componentName)
}

func applyManagedByLabel(resources []unstructured.Unstructured, componentName string) {
	for i := range resources {
		labels := resources[i].GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[odhLabels.PlatformPartOf] = componentName
		resources[i].SetLabels(labels)
	}
}

