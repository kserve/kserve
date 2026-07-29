package kservemodule

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"

	platformv1alpha1 "github.com/opendatahub-io/kserve-module/pkg/apis/v1alpha1"
)

// --- Module CR (cluster-scoped singleton) ---
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=kserves,verbs=list;watch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=kserves,resourceNames=default-kserve,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=kserves/status,resourceNames=default-kserve,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=kserves/finalizers,resourceNames=default-kserve,verbs=update

// --- Cluster environment detection ---
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get

// --- Operand resources (namespace-scoped, cluster-wide because operands may span user namespaces) ---
// +kubebuilder:rbac:groups="",resources=configmaps;services;serviceaccounts,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;delete;patch;update,resourceNames=kserve-webhook-server-secret;workload-variant-autoscaler-controller-manager-token;workload-variant-autoscaler-epp-metrics-token;workload-variant-autoscaler-metrics-reader-token
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=create;delete;get;list;patch;watch

// --- Operand RBAC (cluster-scoped: operand ClusterRoles grant end-user access across namespaces) ---
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=create;delete;get;list;patch;update;watch
// escalate/bind scoped to the exact roles and clusterroles deployed by this controller
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind;escalate,resourceNames=account-editor-role;account-viewer-role;kserve-admin;kserve-edit;kserve-models-admin;kserve-models-edit;kserve-models-view;kserve-view;kserve-manager-role;kserve-proxy-role;kserve-llmisvc-manager-role;kserve-llmisvc-distro-role;kserve-inferenceservice-distro-role;kserve-metrics-reader-cluster-role;openshift-ai-llminferenceservice-scc;openshift-ai-inferenceservice-image-volume-scc;odh-model-controller-role;proxy-role;model-serving-api;metrics-reader;kserve-prometheus-k8s;workload-variant-autoscaler-manager-role;workload-variant-autoscaler-metrics-auth-role;workload-variant-autoscaler-epp-metrics-reader-role;workload-variant-autoscaler-variantautoscaling-admin-role;workload-variant-autoscaler-variantautoscaling-editor-role;workload-variant-autoscaler-variantautoscaling-viewer-role;workload-variant-autoscaler-metrics-reader;kserve-localmodel-manager-role;kserve-localmodel-distro-role;kserve-localmodel-permfix-role;kserve-localmodelnode-agent-role;kserve-localmodelnode-distro-role;kserve-tls-distro-role
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=bind;escalate,resourceNames=kserve-leader-election-role;llmisvc-leader-election-role;leader-election-role;workload-variant-autoscaler-leader-election-role
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles/finalizers;rolebindings/finalizers;clusterroles/finalizers;clusterrolebindings/finalizers,verbs=update

// --- NIM account CRD (cluster-scoped: NIM accounts are managed across namespaces) ---
// +kubebuilder:rbac:groups=nim.opendatahub.io,resources=accounts,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=nim.opendatahub.io,resources=accounts/finalizers,verbs=get;update
// +kubebuilder:rbac:groups=nim.opendatahub.io,resources=accounts/status,verbs=get;update

// --- Operand CRDs (cluster-scoped: controller deploys KServe, LLMInferenceService, and related CRDs) ---
// no delete — CRDs survive component removal (consistent with odh-operator GC unremovables)
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;patch;update;watch

// --- cert-manager (cluster-scoped: ClusterIssuers are cluster-scoped; Issuers/Certificates for webhook TLS) ---
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers,verbs=create;delete;get;list;patch;update;watch

// --- Admission webhooks (cluster-scoped: webhook configs are cluster resources) ---
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=create;delete;get;list;patch;update;watch

// --- KServe cluster-scoped operand resources ---
// +kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceserviceconfigs;clusterstoragecontainers,verbs=create;delete;get;list;patch;update;watch

// --- OpenShift-specific cluster-scoped resources ---
// SCCs: required for InferenceService and LLMInferenceService workload pods
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=create;delete;get;list;patch;update;watch
// Templates: OpenShift template objects deployed by operand manifests
// +kubebuilder:rbac:groups=template.openshift.io,resources=templates,verbs=create;delete;get;list;patch;update;watch

// --- Monitoring (cluster-scoped: ServiceMonitors and Prometheus API for metrics collection) ---
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheuses/api,resourceNames=k8s,verbs=get;create;update

// --- Observability (Perses dashboards deployed to monitoring namespace when COO is present) ---
// +kubebuilder:rbac:groups=perses.dev,resources=persesdashboards,verbs=create;delete;get;list;patch;update;watch

// --- Dependency detection (read-only: check if required operators are installed) ---
// +kubebuilder:rbac:groups=operators.coreos.com,resources=subscriptions,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.openshift.io,resources=leaderworkersetoperators,verbs=get;list;watch
//
// ModelCache RBAC
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

// --- Upgrade path: HardwareProfile migration ---
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=list;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=servingruntimes,verbs=get
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=hardwareprofiles,verbs=get
// +kubebuilder:rbac:groups=opendatahub.io,resources=odhdashboardconfigs,verbs=get

type ResourceDeployer interface {
	Deploy(ctx context.Context, input deploy.DeployInput) error
}

type KserveModuleReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	ManifestsTemplatePath string
	Deployer              ResourceDeployer
	workDir               string
	initDone              bool
	applicationsNamespace string
	monitoringNamespace   string
	clusterType           *cluster.ClusterType

	controller     controller.Controller
	cache          cache.Cache
	dynamicWatches []*dynamicWatch
	dynamicWatchMu sync.Mutex
}

func (r *KserveModuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)

	kserve := &platformv1alpha1.Kserve{}
	if err := r.Get(ctx, req.NamespacedName, kserve); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !kserve.DeletionTimestamp.IsZero() {
		if cleanupErr := r.cleanupOnDelete(ctx); cleanupErr != nil {
			log.Error(cleanupErr, "component extra-cleanup failed during CR deletion")
			return ctrl.Result{}, cleanupErr
		}
		if controllerutil.RemoveFinalizer(kserve, ModuleFinalizerName) {
			if err := r.Update(ctx, kserve); err != nil {
				return ctrl.Result{}, fmt.Errorf("removing module finalizer: %w", err)
			}
			log.Info("removed module finalizer from Kserve CR")
		}
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(kserve, ModuleFinalizerName) {
		if err := r.Update(ctx, kserve); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding module finalizer: %w", err)
		}
		log.Info("added module finalizer to Kserve CR")
	}

	log.Info("reconciling Kserve CR", "name", kserve.Name)

	r.registerDynamicWatches(ctx)

	condMgr := newConditionManager(kserve)
	defer func() {
		if err := r.updateStatus(ctx, kserve, condMgr); err != nil && retErr == nil {
			retErr = err
		}
	}()

	depResult := r.checkDependencies(ctx, kserve)
	applyDependencyConditions(condMgr, depResult)
	if hasCriticalFailure(depResult) {
		applyProvisioningCondition(condMgr, map[string]error{
			"dependencies": fmt.Errorf("critical dependencies unavailable"),
		})
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	componentErrors := r.reconcile(ctx, kserve)
	applyProvisioningCondition(condMgr, componentErrors)
	if len(componentErrors) > 0 {
		var msgs []string
		for _, name := range slices.Sorted(maps.Keys(componentErrors)) {
			log.Error(componentErrors[name], "component reconciliation failed", "component", name)
			msgs = append(msgs, name+": "+componentErrors[name].Error())
		}
		return ctrl.Result{}, fmt.Errorf("reconciliation failed: %s", strings.Join(msgs, "; "))
	}

	r.updateComponentReadiness(ctx, kserve, condMgr)

	if !condMgr.IsHappy() {
		log.Info("not all components ready, requeueing")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *KserveModuleReconciler) reconcile(ctx context.Context, kserve *platformv1alpha1.Kserve) map[string]error {
	log := ctrl.LoggerFrom(ctx)

	if !r.initDone {
		if err := r.installCRDs(ctx, kserve); err != nil {
			return map[string]error{"crds": fmt.Errorf("installing CRDs: %w", err)}
		}
	}

	manifestDir, err := r.ensureWorkDir()
	if err != nil {
		errs := make(map[string]error, len(components))
		for _, comp := range components {
			errs[comp.name] = fmt.Errorf("preparing writable manifests: %w", err)
		}
		return errs
	}

	var allResources []unstructured.Unstructured
	componentErrors := make(map[string]error, len(components))

	for _, comp := range components {
		if comp.enabled != nil && !comp.enabled(kserve) {
			if err := r.defaultCleanup(ctx, comp); err != nil {
				componentErrors[comp.name] = fmt.Errorf("cleanup: %w", err)
				continue
			}
			if comp.extraCleanup != nil {
				if err := comp.extraCleanup(ctx, r); err != nil {
					componentErrors[comp.name] = fmt.Errorf("extra cleanup: %w", err)
					continue
				}
			}
			continue
		}
		resources, err := r.reconcileComponent(ctx, kserve, manifestDir, comp)
		if err != nil {
			componentErrors[comp.name] = err
			continue
		}
		allResources = append(allResources, resources...)
	}

	if len(componentErrors) > 0 {
		return componentErrors
	}

	owned, unowned := splitByOwnership(allResources)
	if err := r.Deployer.Deploy(ctx, deploy.DeployInput{
		Client:    r.Client,
		Owner:     kserve,
		Resources: owned,
	}); err != nil {
		return map[string]error{"deploy": fmt.Errorf("applying owned resources: %w", err)}
	}
	if len(unowned) > 0 {
		if err := r.Deployer.Deploy(ctx, deploy.DeployInput{
			Client:    r.Client,
			Resources: unowned,
		}); err != nil {
			return map[string]error{"deploy": fmt.Errorf("applying unowned resources: %w", err)}
		}
	}

	log.Info("deployed all resources", "owned", len(owned), "unowned", len(unowned))

	return nil
}

func splitByOwnership(resources []unstructured.Unstructured) (owned, unowned []unstructured.Unstructured) {
	for i := range resources {
		gk := resources[i].GroupVersionKind().GroupKind()
		if _, excluded := unownedGroupKinds[gk]; excluded {
			unowned = append(unowned, resources[i])
		} else {
			owned = append(owned, resources[i])
		}
	}
	return
}

func (r *KserveModuleReconciler) cleanupOnDelete(ctx context.Context) error {
	var errs []error
	for _, comp := range components {
		if comp.extraCleanup == nil {
			continue
		}
		if err := comp.extraCleanup(ctx, r); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", comp.name, err))
		}
	}
	return errors.Join(errs...)
}

func (r *KserveModuleReconciler) reconcileComponent(ctx context.Context,
	kserve *platformv1alpha1.Kserve, manifestDir string, comp componentConfig) ([]unstructured.Unstructured, error) {

	log := ctrl.LoggerFrom(ctx)

	sourcePath := comp.sourcePath
	if r.isKubernetes(ctx) {
		if comp.sourcePathXKS == "" {
			log.Info("no XKS overlay, skipping component", "component", comp.name)
			return nil, nil
		}
		sourcePath = comp.sourcePathXKS
	}

	// Image params live in the base overlay (e.g. overlays/odh/params.env), not
	// the XKS overlay whose params.env only carries cert-manager keys.
	if err := applyParams(
		filepath.Join(manifestDir, comp.dirName(), comp.sourcePath),
		comp.imageMap,
	); err != nil {
		return nil, fmt.Errorf("applying %s image params: %w", comp.name, err)
	}

	if r.isKubernetes(ctx) {
		ns := r.getApplicationsNamespace()
		configData := r.getPlatformConfigData(ctx)
		certNS := r.getCertManagerNamespace(ctx, configData)
		if err := applyParams(
			filepath.Join(manifestDir, comp.dirName(), comp.sourcePathXKS),
			nil, buildCertManagerParams(ns, configData, certNS),
		); err != nil {
			return nil, fmt.Errorf("applying cert-manager params: %w", err)
		}
	}

	if comp.extraParams != nil {
		extra := comp.extraParams(kserve)
		if err := applyParams(
			filepath.Join(manifestDir, comp.dirName(), sourcePath),
			nil, extra,
		); err != nil {
			return nil, fmt.Errorf("applying %s extra params: %w", comp.name, err)
		}
	}

	renderPath := filepath.Join(manifestDir, comp.dirName(), sourcePath)
	resources, err := kustomize.Render(renderPath, nil, kustomize.WithNamespace(r.getApplicationsNamespace()))
	if err != nil {
		return nil, fmt.Errorf("rendering %s kustomize: %w", comp.name, err)
	}
	log.Info("rendered kustomize manifests", "component", comp.name, "resourceCount", len(resources))

	if comp.postRender != nil {
		resources, err = comp.postRender(ctx, r, kserve, resources)
		if err != nil {
			return nil, fmt.Errorf("%s post-render: %w", comp.name, err)
		}
	}

	commonPostRender(resources, KserveComponentName)

	log.Info("component rendering complete", "component", comp.name, "resources", len(resources))
	return resources, nil
}

func (r *KserveModuleReconciler) isKubernetes(ctx context.Context) bool {
	if r.clusterType != nil {
		return *r.clusterType == cluster.ClusterTypeKubernetes
	}

	ct, err := cluster.DetectClusterType(ctx, r.Client)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to detect cluster type, assuming OpenShift")
		return false
	}

	r.clusterType = &ct
	return ct == cluster.ClusterTypeKubernetes
}

func (r *KserveModuleReconciler) getMonitoringNamespace() string {
	if r.monitoringNamespace != "" {
		return r.monitoringNamespace
	}

	if ns := os.Getenv("MONITORING_NAMESPACE"); ns != "" {
		r.monitoringNamespace = ns
		return ns
	}

	return "opendatahub"
}

func (r *KserveModuleReconciler) getApplicationsNamespace() string {
	if r.applicationsNamespace != "" {
		return r.applicationsNamespace
	}

	if ns := os.Getenv("APPLICATIONS_NAMESPACE"); ns != "" {
		r.applicationsNamespace = ns
		return ns
	}

	return "opendatahub"
}

// getCertManagerNamespace dynamically discovers the namespace where cert-manager
// stores its CA secrets. It checks (in order):
// 1. Platform ConfigMap (CERT_MANAGER_CA_SECRET_NAMESPACE key)
// 2. "cert-manager" namespace existence on the cluster
// 3. "cert-manager-operator" namespace existence (OCP OLM install)
// 4. Falls back to "cert-manager" constant
func (r *KserveModuleReconciler) getCertManagerNamespace(ctx context.Context, configData map[string]string) string {
	if ns, ok := configData[certManagerCASecretNamespaceKey]; ok && ns != "" {
		return ns
	}

	for _, candidate := range []string{certManagerNSCandidate, certManagerOperatorNS} {
		var ns corev1.Namespace
		if err := r.Get(ctx, types.NamespacedName{Name: candidate}, &ns); err == nil {
			ctrl.LoggerFrom(ctx).V(1).Info("Discovered cert-manager namespace", "namespace", candidate)
			return candidate
		}
	}

	return defaultCertManagerNS
}

func (r *KserveModuleReconciler) getPlatformConfigData(ctx context.Context) map[string]string {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: platformVersionConfigMap, Namespace: r.getApplicationsNamespace()}
	if err := r.Get(ctx, key, cm); err != nil {
		ctrl.LoggerFrom(ctx).V(1).Info("Platform ConfigMap not found",
			"configmap", key, "error", err)
		return nil
	}
	return cm.Data
}

func (r *KserveModuleReconciler) getPlatformVersion(ctx context.Context) string {
	return configOrDefault(r.getPlatformConfigData(ctx), platformVersionConfigMapKey, "")
}

func (r *KserveModuleReconciler) getVersionPrefix(ctx context.Context, kserve *platformv1alpha1.Kserve) string {
	if v := r.getPlatformVersion(ctx); v != "" {
		return "v" + strings.ReplaceAll(v, ".", "-")
	}
	if ann := kserve.GetAnnotations(); ann != nil {
		if v := ann["platform.opendatahub.io/version"]; v != "" {
			return "v" + strings.ReplaceAll(v, ".", "-")
		}
	}
	return "v0-0-0"
}

func (r *KserveModuleReconciler) WorkDir() string {
	return r.workDir
}

func (r *KserveModuleReconciler) SetClusterType(ct cluster.ClusterType) {
	r.clusterType = &ct
}

func (r *KserveModuleReconciler) SetWorkDir(dir string) {
	r.workDir = dir
	r.initDone = true
}

func (r *KserveModuleReconciler) installCRDs(ctx context.Context, kserve *platformv1alpha1.Kserve) error {
	crdPath := filepath.Join(r.ManifestsTemplatePath, KserveComponentName, KserveCRDManifestSourcePath)
	resources, err := kustomize.Render(crdPath, nil, kustomize.WithNamespace(r.getApplicationsNamespace()))
	if err != nil {
		return fmt.Errorf("rendering CRD manifests: %w", err)
	}

	if err := r.Deployer.Deploy(ctx, deploy.DeployInput{
		Client:    r.Client,
		Owner:     kserve,
		Resources: resources,
	}); err != nil {
		return fmt.Errorf("applying CRDs: %w", err)
	}
	return nil
}

func (r *KserveModuleReconciler) ensureWorkDir() (string, error) {
	if r.initDone && r.workDir != "" {
		return r.workDir, nil
	}

	workDir := "/opt/manifests"
	srcDir := r.ManifestsTemplatePath
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		dst := filepath.Join(workDir, rel)
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("copying manifests to workdir: %w", err)
	}

	r.workDir = workDir
	r.initDone = true
	return workDir, nil
}

