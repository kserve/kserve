/*
Copyright 2026 The KServe Authors.

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

package reconcilers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcacheutil "github.com/kserve/kserve/pkg/kernelcache"
	"github.com/kserve/kserve/pkg/kernelcache/captureconfig"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
	"github.com/kserve/kserve/pkg/kernelcache/reporter"
	"github.com/kserve/kserve/pkg/kernelcache/workload"
)

// KernelCacheCaptureControllerReconciler prepares capture reporter identities in workload namespaces.
type KernelCacheCaptureControllerReconciler struct {
	client.Client
	Clientset              kubernetes.Interface
	Reader                 client.Reader
	OperatorNamespace      string
	OperatorServiceAccount string
	hasLLM                 bool
}

func (r *KernelCacheCaptureControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ns := req.Namespace
	if ns == "" {
		return ctrl.Result{}, nil
	}
	namespace := &corev1.Namespace{}
	if err := r.Reader.Get(ctx, client.ObjectKey{Name: ns}, namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if namespace.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	cfg, err := kernelcacheconfig.Load(ctx, r.Reader)
	if err != nil {
		return ctrl.Result{}, err
	}
	capturePod := &corev1.Pod{}
	isCapturePod := false
	var inferenceService *v1beta1.InferenceService
	var capture *v1alpha1.KernelCacheCapture
	if err := r.Reader.Get(ctx, client.ObjectKey{Namespace: ns, Name: req.Name}, capturePod); err == nil {
		if capturePod.DeletionTimestamp != nil {
			return ctrl.Result{}, nil
		}
		_, found, configErr := captureConfigFromPod(capturePod)
		if configErr != nil {
			return ctrl.Result{}, configErr
		}
		if found {
			inferenceService, err = workload.ResolveDeploymentBackedInferenceService(ctx, r.Reader, capturePod)
			if apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			if err != nil {
				return ctrl.Result{}, err
			}
			if inferenceService == nil {
				return ctrl.Result{}, nil
			}
			isCapturePod = true
		}
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	captureEnabled := cfg.Enabled
	if captureEnabled {
		if !isCapturePod {
			captureEnabled, err = r.hasCaptureWorkload(ctx, ns, cfg)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	if !captureEnabled {
		bindings := map[string]string{
			reporter.TokenRequesterRole: reporter.TokenRequesterManagedLabel,
		}
		for name, managedLabel := range bindings {
			binding := &rbacv1.RoleBinding{}
			if err := r.Reader.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, binding); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return ctrl.Result{}, err
			}
			if binding.Labels[managedLabel] == "true" {
				if err := r.Delete(ctx, binding, client.Preconditions{UID: &binding.UID, ResourceVersion: &binding.ResourceVersion}); client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
	}
	if isCapturePod {
		captureAvailable, err := r.ensureCaptureForPod(ctx, capturePod, inferenceService, cfg)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !captureAvailable {
			return ctrl.Result{}, nil
		}
		captureConfig, _, err := captureConfigFromPod(capturePod)
		if err != nil {
			return ctrl.Result{}, err
		}
		capture = &v1alpha1.KernelCacheCapture{}
		if err := r.Reader.Get(ctx, client.ObjectKey{Namespace: ns, Name: captureConfig.Capture.Name}, capture); err != nil {
			return ctrl.Result{}, err
		}
		if isTerminalCapture(capture) {
			if !isReopenableProducerGoneCapture(capture) {
				return ctrl.Result{}, nil
			}
			reopened, reopenErr := reopenProducerGoneCapture(ctx, r.Client, r.Reader, client.ObjectKeyFromObject(capture))
			if reopenErr != nil {
				return ctrl.Result{}, reopenErr
			}
			if reopened == nil {
				return ctrl.Result{}, nil
			}
			capture = reopened
		}
		// Reporter credentials use TokenRequest even when registry publishing
		// authentication is disabled. Ensure the static binding exists before
		// issuing the first reporter token.
		if err := r.ensureTokenRequesterBinding(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.ensureReporterIdentityForCapture(ctx, capture); err != nil {
			return ctrl.Result{}, err
		}
		if r.Clientset == nil {
			return ctrl.Result{}, errors.New("kernelcache reporter access requires a Kubernetes clientset")
		}
		owner := captureOwnerReference(capture)
		_, issueErr := (&reporter.Credentials{Client: r.Clientset}).IssueForCapture(ctx, capturePod, capture.Name, owner)
		if issueErr != nil {
			if apierrors.IsNotFound(issueErr) || apierrors.IsGone(issueErr) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("issue capture reporter access: %w", issueErr)
		}
	}
	if isCapturePod {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return ctrl.Result{}, nil
}

func (r *KernelCacheCaptureControllerReconciler) ensureCaptureForPod(ctx context.Context, pod *corev1.Pod, inferenceService *v1beta1.InferenceService, cfg *v1beta1.KernelCacheConfig) (bool, error) {
	captureConfig, found, err := captureConfigFromPod(pod)
	if err != nil {
		return false, err
	}
	if !found || captureConfig.Capture.Name == "" {
		return false, nil
	}
	if inferenceService == nil {
		return false, errors.New("capture Pod has no resolved InferenceService")
	}
	revisionID := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
	if revisionID == "" {
		return false, nil
	}
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	revision, resolveErr := workload.ResolveDeploymentBackedInferenceServiceRevision(ctx, reader, pod)
	if resolveErr != nil {
		return false, resolveErr
	}
	if revision == nil || revision.Source.UID != inferenceService.UID {
		return false, errors.New("capture Pod workload revision could not be validated")
	}
	ownerRef := metav1.NewControllerRef(revision.ReplicaSet, appsv1.SchemeGroupVersion.WithKind("ReplicaSet"))
	// The KCC should be garbage-collected with its ReplicaSet, but it must not
	// block ReplicaSet deletion. Setting blockOwnerDeletion requires permission
	// to update the owner's finalizers, which the kernel-cache controller does
	// not need and should not hold.
	blockOwnerDeletion := false
	ownerRef.BlockOwnerDeletion = &blockOwnerDeletion
	name := constants.KernelCacheCaptureRevisionName(inferenceService.Name, revisionID)
	if name == "" || captureConfig.Capture.Name != name {
		return false, errors.New("capture Pod capture name does not match resolved workload revision")
	}
	current := &v1alpha1.KernelCacheCapture{}
	getErr := reader.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: name}, current)
	if getErr == nil {
		if !captureSourceMatches(current, inferenceService) {
			return false, errors.New("capture source does not match resolved InferenceService")
		}
		if !captureOwnerMatches(current, ownerRef) {
			return false, errors.New("capture owner does not match resolved workload")
		}
		return true, nil
	}
	if !apierrors.IsNotFound(getErr) {
		return false, getErr
	}
	cachePaths := append([]v1alpha1.KernelCachePath(nil), captureConfig.CachePaths...)
	for index := range cachePaths {
		ociPath, resolveErr := kernelcacheutil.ResolveOCIPath(cachePaths[index].OCIPath)
		if resolveErr != nil {
			return false, resolveErr
		}
		cachePaths[index].OCIPath = ociPath
	}

	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pod.Namespace,
			Labels:    map[string]string{constants.KernelCacheCaptureGeneratedLabelKey: "true"},
			OwnerReferences: []metav1.OwnerReference{
				*ownerRef,
			},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			SourceRef:   v1alpha1.KernelCacheSourceRef{Kind: "InferenceService", Name: inferenceService.Name},
			TargetImage: captureConfig.TargetImage,
			CachePaths:  cachePaths,
		},
	}
	capture.Spec.Signing = captureSigningSpec(cfg)
	if err := r.Create(ctx, capture); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return false, err
		}
		current = &v1alpha1.KernelCacheCapture{}
		if err := reader.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: name}, current); err != nil {
			return false, err
		}
		if !captureSourceMatches(current, inferenceService) {
			return false, fmt.Errorf("existing capture %s/%s has a different source", pod.Namespace, name)
		}
		if !captureOwnerMatches(current, ownerRef) {
			return false, fmt.Errorf("existing capture %s/%s has a different workload owner", pod.Namespace, name)
		}
	}
	return true, nil
}

func captureConfigFromPod(pod *corev1.Pod) (captureconfig.CaptureConfig, bool, error) {
	for _, container := range pod.Spec.Containers {
		if container.Name != "mcv" {
			continue
		}
		envs := make(map[string]string, len(container.Env))
		for _, env := range container.Env {
			envs[env.Name] = env.Value
		}
		value := envs[captureconfig.CaptureConfigEnv]
		if value == "" {
			return captureconfig.CaptureConfig{}, false, nil
		}
		config, err := captureconfig.ParseCaptureConfig(value)
		if err != nil {
			return captureconfig.CaptureConfig{}, true, err
		}
		return config, true, nil
	}
	return captureconfig.CaptureConfig{}, false, nil
}

func captureSourceMatches(capture *v1alpha1.KernelCacheCapture, inferenceService *v1beta1.InferenceService) bool {
	if capture == nil || inferenceService == nil {
		return false
	}
	ref := capture.Spec.SourceRef
	return ref.Kind == "InferenceService" &&
		ref.Name == inferenceService.Name &&
		capture.Namespace == inferenceService.Namespace
}

func captureOwnerMatches(capture *v1alpha1.KernelCacheCapture, expected *metav1.OwnerReference) bool {
	if capture == nil || expected == nil {
		return false
	}
	owner := metav1.GetControllerOf(capture)
	return owner != nil && owner.UID == expected.UID && owner.Kind == expected.Kind && owner.APIVersion == expected.APIVersion && owner.Name == expected.Name
}

func (r *KernelCacheCaptureControllerReconciler) ensureReporterIdentityForCapture(ctx context.Context, capture *v1alpha1.KernelCacheCapture) error {
	namespace := capture.Namespace
	serviceAccountName := reporter.ServiceAccountName(capture.Name)
	owner := captureOwnerReference(capture)
	if err := r.ensureManagedServiceAccountWithOwner(ctx, namespace, serviceAccountName, reporter.ManagedLabel, owner); err != nil {
		return err
	}
	roleName := reporter.RoleName(capture.Name)
	desiredRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: namespace, Labels: map[string]string{reporter.ManagedLabel: "true"}, OwnerReferences: []metav1.OwnerReference{*owner}},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"serving.kserve.io"}, Resources: []string{"kernelcachecaptures"}, ResourceNames: []string{capture.Name}, Verbs: []string{"get"}},
			{APIGroups: []string{"serving.kserve.io"}, Resources: []string{"kernelcachecaptures/status"}, ResourceNames: []string{capture.Name}, Verbs: []string{"get", "update"}},
		},
	}
	currentRole := &rbacv1.Role{}
	if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(desiredRole), currentRole); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, desiredRole); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(desiredRole), currentRole); err != nil {
				return err
			}
		} else {
			currentRole = nil
		}
	}
	if currentRole != nil && currentRole.Labels[reporter.ManagedLabel] != "true" {
		return fmt.Errorf("reserved Role %s/%s already exists and is not managed by kernelcache", namespace, roleName)
	}
	if currentRole != nil {
		currentOwner := metav1.GetControllerOf(currentRole)
		if currentOwner == nil || currentOwner.UID != owner.UID {
			return fmt.Errorf("reserved Role %s/%s has a different owner", namespace, roleName)
		}
		if !reflect.DeepEqual(currentRole.Rules, desiredRole.Rules) {
			base := currentRole.DeepCopy()
			currentRole.Rules = desiredRole.Rules
			if err := r.Patch(ctx, currentRole, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
				return err
			}
		}
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleBindingName(capture.Name), Namespace: namespace, Labels: map[string]string{reporter.ManagedLabel: "true"}, OwnerReferences: []metav1.OwnerReference{*owner}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: roleName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: serviceAccountName, Namespace: namespace}},
	}
	return r.ensureRoleBinding(ctx, binding, reporter.ManagedLabel)
}

func captureOwnerReference(capture *v1alpha1.KernelCacheCapture) *metav1.OwnerReference {
	owner := metav1.NewControllerRef(capture, v1alpha1.SchemeGroupVersion.WithKind("KernelCacheCapture"))
	blockOwnerDeletion := false
	owner.BlockOwnerDeletion = &blockOwnerDeletion
	return owner
}

func (r *KernelCacheCaptureControllerReconciler) ensureTokenRequesterBinding(ctx context.Context, namespace string) error {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: reporter.TokenRequesterRole, Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: reporter.TokenRequesterRole},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: r.OperatorServiceAccount, Namespace: r.OperatorNamespace}},
	}
	return r.ensureRoleBinding(ctx, binding, reporter.TokenRequesterManagedLabel)
}

func (r *KernelCacheCaptureControllerReconciler) ensureManagedServiceAccountWithOwner(ctx context.Context, namespace, name, label string, owner *metav1.OwnerReference) error {
	automount := false
	desired := &corev1.ServiceAccount{
		ObjectMeta:                   metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{label: "true"}},
		AutomountServiceAccountToken: &automount,
	}
	if owner != nil {
		desired.OwnerReferences = []metav1.OwnerReference{*owner}
	}
	current := &corev1.ServiceAccount{}
	err := r.Reader.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err == nil {
			return nil
		} else if !apierrors.IsAlreadyExists(err) {
			return err
		}
		if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	if current.Labels[label] != "true" {
		return fmt.Errorf("reserved ServiceAccount %s/%s already exists and is not managed by kernelcache", namespace, name)
	}
	if owner != nil {
		currentOwner := metav1.GetControllerOf(current)
		if currentOwner == nil || currentOwner.UID != owner.UID {
			return fmt.Errorf("reserved ServiceAccount %s/%s has a different owner", namespace, name)
		}
	}
	if current.AutomountServiceAccountToken == nil || *current.AutomountServiceAccountToken {
		base := current.DeepCopy()
		current.AutomountServiceAccountToken = desired.AutomountServiceAccountToken
		if err := r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
			return err
		}
	}
	return nil
}

func (r *KernelCacheCaptureControllerReconciler) ensureRoleBinding(ctx context.Context, desired *rbacv1.RoleBinding, label string) error {
	desired.Labels = map[string]string{label: "true"}
	current := &rbacv1.RoleBinding{}
	if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		err = r.Create(ctx, desired)
		if err == nil {
			return nil
		}
		if apierrors.IsAlreadyExists(err) {
			if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if current.Labels[label] != "true" || current.RoleRef != desired.RoleRef {
		return fmt.Errorf("reserved RoleBinding %s/%s conflicts with a KernelCache-managed identity", desired.Namespace, desired.Name)
	}
	if desiredOwner := metav1.GetControllerOf(desired); desiredOwner != nil {
		currentOwner := metav1.GetControllerOf(current)
		if currentOwner == nil || currentOwner.UID != desiredOwner.UID {
			return fmt.Errorf("reserved RoleBinding %s/%s has a different owner", desired.Namespace, desired.Name)
		}
	}
	if reflect.DeepEqual(current.Subjects, desired.Subjects) {
		return nil
	}
	base := current.DeepCopy()
	current.Subjects = desired.Subjects
	return r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

func captureEnabled(obj client.Object, cfg *v1beta1.KernelCacheConfig) bool {
	if obj.GetDeletionTimestamp() != nil {
		return false
	}
	value, set := obj.GetAnnotations()[constants.KernelCacheSidecarInjectionAnnotationKey]
	if set {
		return value == "true"
	}
	return cfg.DefaultSidecarInjection
}

func (r *KernelCacheCaptureControllerReconciler) hasCaptureWorkload(ctx context.Context, ns string, cfg *v1beta1.KernelCacheConfig) (bool, error) {
	services := &v1beta1.InferenceServiceList{}
	if err := r.List(ctx, services, client.InNamespace(ns)); err != nil {
		return false, err
	}
	for i := range services.Items {
		if captureEnabled(&services.Items[i], cfg) {
			return true, nil
		}
	}
	if r.hasLLM {
		services := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, services, client.InNamespace(ns)); err != nil {
			return false, err
		}
		for i := range services.Items {
			if captureEnabled(&services.Items[i], cfg) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *KernelCacheCaptureControllerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var err error
	r.hasLLM, err = hasLLMInferenceServiceCRD(mgr)
	if err != nil {
		return err
	}
	if r.OperatorNamespace == "" || r.OperatorServiceAccount == "" {
		return errors.New("kernelcache capture identity requires operator identity")
	}
	if r.Reader == nil {
		r.Reader = mgr.GetAPIReader()
	}
	// Capture identities are bootstrapped when an MCV Pod is created. Later Pod
	// updates must not recreate identities after terminal capture cleanup.
	podCreatePredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			pod, ok := e.Object.(*corev1.Pod)
			return ok && isMCVCapturePod(pod)
		},
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
	b := ctrl.NewControllerManagedBy(mgr).Named("kernelcache-capture").
		For(&v1beta1.InferenceService{}).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.captureConfigRequests), builder.WithPredicates(predicate.NewPredicateFuncs(isInferenceServiceConfigMap))).
		Watches(&rbacv1.RoleBinding{}, handler.EnqueueRequestsFromMapFunc(r.tokenRequesterBindingRequests), builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetName() == reporter.TokenRequesterRole
		}))).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			pod, ok := obj.(*corev1.Pod)
			if !ok || !isMCVCapturePod(pod) {
				return nil
			}
			return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
		}), builder.WithPredicates(podCreatePredicate))
	if r.hasLLM {
		b = b.Watches(&v1alpha2.LLMInferenceService{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}}}
		}))
	}
	return b.Complete(r)
}

func (r *KernelCacheCaptureControllerReconciler) tokenRequesterBindingRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	services := &v1beta1.InferenceServiceList{}
	if err := r.List(ctx, services, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(services.Items))
	for index := range services.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&services.Items[index])})
	}
	if r.hasLLM {
		llmServices := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llmServices, client.InNamespace(obj.GetNamespace())); err == nil {
			for index := range llmServices.Items {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: obj.GetNamespace(), Name: llmServices.Items[index].Name}})
			}
		}
	}
	return requests
}

func hasMCVContainer(pod *corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == "mcv" {
			return true
		}
	}
	return false
}

func isMCVCapturePod(pod *corev1.Pod) bool {
	return pod != nil && pod.Labels[constants.InferenceServicePodLabelKey] != "" && hasMCVContainer(pod)
}

func hasLLMInferenceServiceCRD(mgr ctrl.Manager) (bool, error) {
	_, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{
		Group: v1alpha2.SchemeGroupVersion.Group,
		Kind:  "LLMInferenceService",
	}, v1alpha2.SchemeGroupVersion.Version)
	if meta.IsNoMatchError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *KernelCacheCaptureControllerReconciler) captureConfigRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetName() != constants.InferenceServiceConfigMapName || obj.GetNamespace() != constants.KServeNamespace {
		return nil
	}
	namespaces := map[string]struct{}{}
	services := &v1beta1.InferenceServiceList{}
	if err := r.List(ctx, services); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "List capture identity workloads")
		return nil
	}
	for _, svc := range services.Items {
		namespaces[svc.Namespace] = struct{}{}
	}
	if r.hasLLM {
		llms := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, llms); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "List capture identity LLM workloads")
			return nil
		}
		for _, svc := range llms.Items {
			namespaces[svc.Namespace] = struct{}{}
		}
	}
	requests := make([]reconcile.Request, 0, len(namespaces))
	for ns := range namespaces {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: ns, Name: "capture-config"}})
	}
	return requests
}
