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

package llmisvc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const (
	adapterLifecyclePollInterval = 30 * time.Second
	adapterModelsRequestTimeout  = 3 * time.Second
	maxAdapterModelsResponseSize = 1 << 20
	defaultModelServerPort       = int32(8000)
)

type adapterRegistrationState uint8

const (
	adapterRegistrationUnknown adapterRegistrationState = iota
	adapterRegistrationAbsent
	adapterRegistrationPresent
)

// adapterWarmCopyTracker keeps the last observed registration state per
// adapter and pod. A present registration contributes one warm copy. This is
// deliberately based on GET /v1/models registration state; it does not claim
// to observe or prevent vLLM's internal GPU-cache eviction.
type adapterWarmCopyTracker struct {
	mu            sync.RWMutex
	registrations map[string]map[types.UID]adapterRegistrationState
	warmCopies    map[string]int
}

func newAdapterWarmCopyTracker() *adapterWarmCopyTracker {
	return &adapterWarmCopyTracker{
		registrations: make(map[string]map[types.UID]adapterRegistrationState),
		warmCopies:    make(map[string]int),
	}
}

// UpdatePodSnapshot replaces the observations for one pod with a complete
// /v1/models snapshot. knownAdapters includes configured adapters and adapters
// previously reported by lifecycle operations; missing names become absent.
func (t *adapterWarmCopyTracker) UpdatePodSnapshot(podUID types.UID, knownAdapters, registeredAdapters []string) {
	if podUID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	known := make(map[string]struct{}, len(knownAdapters)+len(registeredAdapters))
	registered := make(map[string]struct{}, len(registeredAdapters))
	for _, name := range knownAdapters {
		if name != "" {
			known[name] = struct{}{}
		}
	}
	for name := range t.registrations {
		known[name] = struct{}{}
	}
	for _, name := range registeredAdapters {
		if name != "" {
			known[name] = struct{}{}
			registered[name] = struct{}{}
		}
	}

	for name := range known {
		state := adapterRegistrationAbsent
		if _, ok := registered[name]; ok {
			state = adapterRegistrationPresent
		}
		t.setLocked(name, podUID, state)
	}
}

// RecordOperationResult applies the result of this controller's load or unload
// request immediately. Call it only after the adapter endpoint confirms success.
func (t *adapterWarmCopyTracker) RecordOperationResult(podUID types.UID, adapterName string, registered bool) {
	if podUID == "" || adapterName == "" {
		return
	}

	state := adapterRegistrationAbsent
	if registered {
		state = adapterRegistrationPresent
	}
	t.mu.Lock()
	t.setLocked(adapterName, podUID, state)
	t.mu.Unlock()
}

func (t *adapterWarmCopyTracker) MarkPodUnknown(podUID types.UID, adapterNames ...string) {
	if podUID == "" {
		return
	}
	t.mu.Lock()
	known := make(map[string]struct{}, len(adapterNames)+len(t.registrations))
	for _, name := range adapterNames {
		if name != "" {
			known[name] = struct{}{}
		}
	}
	for name := range t.registrations {
		known[name] = struct{}{}
	}
	for name := range known {
		t.setLocked(name, podUID, adapterRegistrationUnknown)
	}
	t.mu.Unlock()
}

func (t *adapterWarmCopyTracker) MarkAllUnknown() {
	t.mu.Lock()
	for name, pods := range t.registrations {
		for uid := range pods {
			t.setLocked(name, uid, adapterRegistrationUnknown)
		}
	}
	t.mu.Unlock()
}

func (t *adapterWarmCopyTracker) RemovePod(podUID types.UID) {
	t.mu.Lock()
	t.removePodLocked(podUID)
	t.mu.Unlock()
}

func (t *adapterWarmCopyTracker) RemovePodsNotIn(activePods map[types.UID]struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for name, pods := range t.registrations {
		for uid, state := range pods {
			if _, active := activePods[uid]; active {
				continue
			}
			if state == adapterRegistrationPresent {
				t.decrementWarmCopyLocked(name)
			}
			delete(pods, uid)
		}
		if len(pods) == 0 {
			delete(t.registrations, name)
			delete(t.warmCopies, name)
		}
	}
}

func (t *adapterWarmCopyTracker) WarmCopyCount(adapterName string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.warmCopies[adapterName]
}

func (t *adapterWarmCopyTracker) Registration(adapterName string, podUID types.UID) (adapterRegistrationState, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pods, ok := t.registrations[adapterName]
	if !ok {
		return adapterRegistrationUnknown, false
	}
	state, ok := pods[podUID]
	return state, ok
}

func (t *adapterWarmCopyTracker) KnownAdapters() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	names := make([]string, 0, len(t.registrations))
	for name := range t.registrations {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (t *adapterWarmCopyTracker) setLocked(adapterName string, podUID types.UID, state adapterRegistrationState) {
	pods, ok := t.registrations[adapterName]
	if !ok {
		pods = make(map[types.UID]adapterRegistrationState)
		t.registrations[adapterName] = pods
	}
	previous, found := pods[podUID]
	if found && previous == adapterRegistrationPresent && state != adapterRegistrationPresent {
		t.decrementWarmCopyLocked(adapterName)
	} else if (!found || previous != adapterRegistrationPresent) && state == adapterRegistrationPresent {
		t.warmCopies[adapterName]++
	}
	pods[podUID] = state
}

func (t *adapterWarmCopyTracker) decrementWarmCopyLocked(adapterName string) {
	if t.warmCopies[adapterName] <= 1 {
		delete(t.warmCopies, adapterName)
		return
	}
	t.warmCopies[adapterName]--
}

func (t *adapterWarmCopyTracker) removePodLocked(podUID types.UID) {
	for name, pods := range t.registrations {
		if state, ok := pods[podUID]; ok {
			if state == adapterRegistrationPresent {
				t.decrementWarmCopyLocked(name)
			}
			delete(pods, podUID)
		}
		if len(pods) == 0 {
			delete(t.registrations, name)
			delete(t.warmCopies, name)
		}
	}
}

type adapterLifecycleState struct {
	serviceUID types.UID
	tracker    *adapterWarmCopyTracker
}

var adapterLifecycleStates = struct {
	sync.Mutex
	byService map[types.NamespacedName]*adapterLifecycleState
}{byService: make(map[types.NamespacedName]*adapterLifecycleState)}

func adapterLifecycleTrackerFor(llmSvc *v1alpha2.LLMInferenceService) *adapterWarmCopyTracker {
	key := client.ObjectKeyFromObject(llmSvc)
	adapterLifecycleStates.Lock()
	defer adapterLifecycleStates.Unlock()
	state := adapterLifecycleStates.byService[key]
	if state == nil || state.serviceUID != llmSvc.UID {
		state = &adapterLifecycleState{serviceUID: llmSvc.UID, tracker: newAdapterWarmCopyTracker()}
		adapterLifecycleStates.byService[key] = state
	}
	return state.tracker
}

func forgetAdapterLifecycleState(llmSvc *v1alpha2.LLMInferenceService) {
	if llmSvc == nil {
		return
	}
	adapterLifecycleStates.Lock()
	delete(adapterLifecycleStates.byService, client.ObjectKeyFromObject(llmSvc))
	adapterLifecycleStates.Unlock()
}

type adapterWarmTarget struct {
	name        string
	minReplicas *int32
	aliases     []string
}

func adapterWarmTargets(llmSvc *v1alpha2.LLMInferenceService) []adapterWarmTarget {
	if llmSvc.Spec.Model.LoRA == nil {
		return nil
	}
	targets := make([]adapterWarmTarget, 0, len(llmSvc.Spec.Model.LoRA.Adapters))
	for _, adapter := range llmSvc.Spec.Model.LoRA.Adapters {
		if adapter.Name == nil || *adapter.Name == "" {
			continue
		}
		name := *adapter.Name
		targets = append(targets, adapterWarmTarget{
			name:        name,
			minReplicas: adapter.MinReplicas,
			aliases:     []string{name, fullyQualifiedModelName(llmSvc.Namespace, name)},
		})
	}
	return targets
}

func (r *LLMISVCReconciler) reconcileAdapterLifecycle(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ctrl.Result {
	if llmSvc.DeletionTimestamp != nil {
		forgetAdapterLifecycleState(llmSvc)
		llmSvc.MarkLoRAAdaptersWarmUnset()
		return ctrl.Result{}
	}

	targets := adapterWarmTargets(llmSvc)
	if llmSvc.Spec.Model.LoRA == nil {
		forgetAdapterLifecycleState(llmSvc)
		llmSvc.MarkLoRAAdaptersWarmUnset()
		return ctrl.Result{}
	}

	tracker := adapterLifecycleTrackerFor(llmSvc)
	knownAdapters := tracker.KnownAdapters()
	for _, target := range targets {
		knownAdapters = append(knownAdapters, target.name)
	}
	slices.Sort(knownAdapters)
	knownAdapters = slices.Compact(knownAdapters)
	if len(knownAdapters) == 0 {
		forgetAdapterLifecycleState(llmSvc)
		llmSvc.MarkLoRAAdaptersWarmUnset()
		return ctrl.Result{}
	}

	pods := &corev1.PodList{}
	selector := GetWorkloadLabelSelector(llmSvc.ObjectMeta, &llmSvc.Spec)
	if err := r.List(ctx, pods, client.InNamespace(llmSvc.Namespace), client.MatchingLabels(selector)); err != nil {
		tracker.MarkAllUnknown()
		log.FromContext(ctx).Error(err, "Failed to list workload pods for adapter registration polling")
		markAdapterWarmTargetCondition(llmSvc, targets, tracker, true)
		return ctrl.Result{RequeueAfter: adapterLifecyclePollInterval}
	}

	scheme, port, httpClient, clientErr := r.adapterModelAPIClient(ctx, llmSvc)
	if httpClient != nil {
		defer httpClient.CloseIdleConnections()
	}
	if clientErr != nil {
		log.FromContext(ctx).Error(clientErr, "Failed to prepare adapter model API client")
		markAdapterPodsUnknown(pods.Items, knownAdapters, tracker)
		markAdapterWarmTargetCondition(llmSvc, targets, tracker, true)
		return ctrl.Result{RequeueAfter: adapterLifecyclePollInterval}
	}

	fetchModels := func(ctx context.Context, pod *corev1.Pod) ([]string, error) {
		endpoint := adapterModelsEndpoint(scheme, pod.Status.PodIP, port)
		return fetchAdapterModelIDs(ctx, httpClient, endpoint)
	}
	observationErrors := observeAdapterPods(ctx, llmSvc.Namespace, pods.Items, targets, knownAdapters, tracker, fetchModels)
	for _, observationErr := range observationErrors {
		log.FromContext(ctx).V(1).Info("Could not observe adapter registrations on pod", "pod", observationErr.pod, "error", observationErr.err)
	}
	markAdapterWarmTargetCondition(llmSvc, targets, tracker, len(observationErrors) > 0)

	return ctrl.Result{RequeueAfter: adapterLifecyclePollInterval}
}

func (r *LLMISVCReconciler) adapterModelAPIClient(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (string, int32, *http.Client, error) {
	scheme := "http"
	port := defaultModelServerPort
	service := &corev1.Service{}
	serviceKey := client.ObjectKey{Namespace: llmSvc.Namespace, Name: workloadServiceName(llmSvc)}
	if err := r.Get(ctx, serviceKey, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return "", 0, nil, fmt.Errorf("failed to get workload service %s: %w", serviceKey, err)
		}
	} else if len(service.Spec.Ports) > 0 {
		servicePort := service.Spec.Ports[0]
		if servicePort.Name == "https" || servicePort.AppProtocol != nil && *servicePort.AppProtocol == "https" {
			scheme = "https"
		}
		if servicePort.TargetPort.Type == intstr.Int && servicePort.TargetPort.IntVal > 0 {
			port = servicePort.TargetPort.IntVal
		} else if servicePort.Port > 0 {
			port = servicePort.Port
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if scheme == "https" {
		secret := r.getExistingSelfSignedCertificate(ctx, llmSvc)
		if secret == nil {
			return "", 0, nil, errors.New("workload TLS certificate secret is not available")
		}
		ca := secret.Data["ca.crt"]
		if len(ca) == 0 {
			ca = secret.Data["tls.crt"]
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			return "", 0, nil, errors.New("workload TLS certificate secret has no usable CA certificate")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}

	return scheme, port, &http.Client{Transport: transport, Timeout: adapterModelsRequestTimeout}, nil
}

type adapterModelListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func adapterModelsEndpoint(scheme, podIP string, port int32) string {
	return scheme + "://" + net.JoinHostPort(podIP, strconv.Itoa(int(port))) + "/v1/models"
}

func fetchAdapterModelIDs(ctx context.Context, httpClient *http.Client, endpoint string) ([]string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, adapterModelsRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET /v1/models request: %w", err)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GET /v1/models failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("GET /v1/models returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAdapterModelsResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read GET /v1/models response: %w", err)
	}
	if len(body) > maxAdapterModelsResponseSize {
		return nil, fmt.Errorf("GET /v1/models response exceeds %d bytes", maxAdapterModelsResponseSize)
	}
	var modelList adapterModelListResponse
	if err := json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to decode GET /v1/models response: %w", err)
	}
	ids := make([]string, 0, len(modelList.Data))
	for _, model := range modelList.Data {
		if model.ID != "" {
			ids = append(ids, model.ID)
		}
	}
	return ids, nil
}

type adapterObservationError struct {
	pod string
	err error
}

func observeAdapterPods(
	ctx context.Context,
	namespace string,
	pods []corev1.Pod,
	targets []adapterWarmTarget,
	knownAdapters []string,
	tracker *adapterWarmCopyTracker,
	fetchModels func(context.Context, *corev1.Pod) ([]string, error),
) []adapterObservationError {
	activePods := make(map[types.UID]struct{}, len(pods))
	observationErrors := make([]adapterObservationError, 0)
	knownSet := make(map[string]struct{}, len(knownAdapters)+len(targets))
	for _, name := range knownAdapters {
		knownSet[name] = struct{}{}
	}
	for _, target := range targets {
		knownSet[target.name] = struct{}{}
	}

	for i := range pods {
		pod := &pods[i]
		if pod.UID == "" {
			continue
		}
		activePods[pod.UID] = struct{}{}
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
			tracker.MarkPodUnknown(pod.UID, knownAdapters...)
			observationErrors = append(observationErrors, adapterObservationError{
				pod: pod.Namespace + "/" + pod.Name,
				err: errors.New("pod is not running or has no PodIP"),
			})
			continue
		}

		modelIDs, err := fetchModels(ctx, pod)
		if err != nil {
			tracker.MarkPodUnknown(pod.UID, knownAdapters...)
			observationErrors = append(observationErrors, adapterObservationError{pod: pod.Namespace + "/" + pod.Name, err: err})
			continue
		}
		registeredAdapters := registeredAdapterNames(namespace, targets, modelIDs, knownSet)
		tracker.UpdatePodSnapshot(pod.UID, knownAdapters, registeredAdapters)
	}
	tracker.RemovePodsNotIn(activePods)
	return observationErrors
}

func registeredAdapterNames(namespace string, targets []adapterWarmTarget, modelIDs []string, knownAdapters map[string]struct{}) []string {
	aliasToAdapter := make(map[string]string, len(targets)*2)
	for _, target := range targets {
		for _, alias := range target.aliases {
			aliasToAdapter[alias] = target.name
		}
	}
	qualifiedPrefix := "publishers/" + namespace + "/models/"
	registered := make(map[string]struct{})
	for _, id := range modelIDs {
		if adapter, ok := aliasToAdapter[id]; ok {
			registered[adapter] = struct{}{}
			continue
		}
		if name, ok := strings.CutPrefix(id, qualifiedPrefix); ok {
			if _, known := knownAdapters[name]; known {
				registered[name] = struct{}{}
			}
			continue
		}
		if _, known := knownAdapters[id]; known {
			registered[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(registered))
	for name := range registered {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}

func markAdapterPodsUnknown(pods []corev1.Pod, knownAdapters []string, tracker *adapterWarmCopyTracker) {
	activePods := make(map[types.UID]struct{}, len(pods))
	for i := range pods {
		if pods[i].UID == "" {
			continue
		}
		activePods[pods[i].UID] = struct{}{}
		tracker.MarkPodUnknown(pods[i].UID, knownAdapters...)
	}
	tracker.RemovePodsNotIn(activePods)
}

func markAdapterWarmTargetCondition(
	llmSvc *v1alpha2.LLMInferenceService,
	targets []adapterWarmTarget,
	tracker *adapterWarmCopyTracker,
	observationFailed bool,
) {
	configured := false
	for _, target := range targets {
		if target.minReplicas == nil {
			continue
		}
		configured = true
		warmCopies := tracker.WarmCopyCount(target.name)
		if int64(warmCopies) >= int64(*target.minReplicas) {
			continue
		}
		reason := "WarmTargetNotMet"
		if observationFailed {
			reason = "AdapterRegistrationUnknown"
		}
		llmSvc.MarkLoRAAdaptersWarmNotReady(
			reason,
			"LoRA adapter %q has %d warm copies; minReplicas is %d",
			target.name,
			warmCopies,
			*target.minReplicas,
		)
		return
	}
	if !configured {
		llmSvc.MarkLoRAAdaptersWarmUnset()
		return
	}
	llmSvc.MarkLoRAAdaptersWarmReady()
}
