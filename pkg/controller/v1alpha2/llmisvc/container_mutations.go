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
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// ContainerMutation modifies a container in-place.
type ContainerMutation func(*corev1.Container)

// withArg appends an arg if not already present.
func withArg(arg string) ContainerMutation {
	return func(c *corev1.Container) {
		if !slices.Contains(c.Args, arg) {
			c.Args = append(c.Args, arg)
		}
	}
}

// withEnvVar sets an env var to a literal value. If the env var already exists,
// the entire EnvVar struct is replaced - this intentionally clears ValueFrom
// to prevent an invalid state where both Value and ValueFrom are set
// (which Kubernetes rejects).
func withEnvVar(name, value string) ContainerMutation {
	return func(c *corev1.Container) {
		idx := slices.IndexFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == name })
		if idx >= 0 {
			c.Env[idx] = corev1.EnvVar{Name: name, Value: value}
		} else {
			c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
		}
	}
}

// withVolumeMount adds a VolumeMount if not already present (by mount path).
func withVolumeMount(vm corev1.VolumeMount) ContainerMutation {
	return func(c *corev1.Container) {
		for _, existing := range c.VolumeMounts {
			if existing.MountPath == vm.MountPath {
				return
			}
		}
		c.VolumeMounts = append(c.VolumeMounts, vm)
	}
}

const (
	servedByEnvVar             = "KSERVE_SERVED_BY"
	servedByMiddlewareClassVar = "KSERVE_MIDDLEWARE_CLASS"
	servedByMiddlewareClass    = "kserve.served_by.ServedByMiddleware"
	servedByContainerName      = "main"
	servedByConfigMapName      = "kserve-served-by-middleware"
	servedByVolumeName         = "kserve-middleware"
	servedByMountPath          = "/opt/kserve-middleware/kserve"
	servedByPythonPath         = "/opt/kserve-middleware"
	servedByPythonPathVar      = "PYTHONPATH"
)

// servedByMiddlewareSource is the Python ASGI middleware that adds x-served-by
// response headers. Embedded here so the controller can deploy it via ConfigMap
// to any vLLM image without requiring the kserve SDK to be installed.
const servedByMiddlewareSource = `import os
import re

_HEADER_NAME = b"x-served-by"
_ENV_VAR = "KSERVE_SERVED_BY"
_DNS_SAFE = re.compile(r"^[a-z0-9]([a-z0-9.\-]*[a-z0-9])?$")


def _validate(raw):
    if not raw:
        return None
    value = raw.strip().lower()
    if not value or len(value) > 128 or not _DNS_SAFE.match(value):
        return None
    return value.encode("ascii")


class ServedByMiddleware:
    def __init__(self, app):
        self.app = app
        self._value = _validate(os.environ.get(_ENV_VAR))

    def __call__(self, scope, receive, send):
        if scope.get("type") != "http" or self._value is None:
            return self.app(scope, receive, send)
        value = self._value

        async def send_with_header(message):
            if message.get("type") == "http.response.start":
                headers = [
                    (k, v) for k, v in message.get("headers", [])
                    if k.lower() != _HEADER_NAME
                ]
                headers.append((_HEADER_NAME, value))
                message = {**message, "headers": headers}
            await send(message)

        return self.app(scope, receive, send_with_header)
`

// injectServedByMiddleware adds the x-served-by response header middleware
// when opted in via the serving.kserve.io/enable-served-by-header annotation.
//
// It ensures a ConfigMap with the middleware Python code exists in the namespace,
// mounts it into the container, and sets env vars so the bash entrypoint template
// can conditionally add --middleware at startup.
//
// When the annotation is absent or "false", none of the above is injected.
// The deployment spec is rebuilt from template on every reconcile, so removal
// is implicit - the volume, mount, and env vars simply aren't added.
// The ConfigMap is left in the namespace (inert when not mounted).
func (r *LLMISVCReconciler) injectServedByMiddleware(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) error {
	if llmSvc.Annotations[constants.LLMServedByAnnotationKey] != "true" {
		llmSvc.MarkRequestAttributionReadyUnset()
		return nil
	}

	container := utils.GetContainerWithName(podSpec, servedByContainerName)
	if container == nil {
		return nil
	}

	// Proactive Rust frontend detection. The controller can only see literal env
	// vars in the pod spec - ValueFrom, Dockerfile ENV, and entrypoint-set values
	// are invisible here. The bash template guards those cases at startup by
	// checking VLLM_USE_RUST_FRONTEND before adding --middleware.
	for _, env := range container.Env {
		if env.Name == "VLLM_USE_RUST_FRONTEND" && env.Value == "1" {
			llmSvc.MarkRequestAttributionNotReady("UnsupportedRuntime",
				"Rust frontend does not support --middleware; x-served-by header will not be added")
			return nil
		}
	}

	ensured, err := r.ensureServedByConfigMap(ctx, llmSvc.Namespace)
	if err != nil {
		return fmt.Errorf("failed to ensure served-by middleware ConfigMap: %w", err)
	}
	if !ensured {
		return nil
	}

	addVolume(podSpec, corev1.Volume{
		Name: servedByVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: servedByConfigMapName},
			},
		},
	})

	mutations := []ContainerMutation{
		withEnvVar(servedByEnvVar, llmSvc.Name),
		withEnvVar(servedByMiddlewareClassVar, servedByMiddlewareClass),
		withEnvVar(servedByPythonPathVar, servedByPythonPath+":$("+servedByPythonPathVar+")"),
		withVolumeMount(corev1.VolumeMount{
			Name:      servedByVolumeName,
			MountPath: servedByMountPath,
			ReadOnly:  true,
		}),
	}
	for _, m := range mutations {
		m(container)
	}

	llmSvc.MarkRequestAttributionReady()
	return nil
}

const servedByManagedByValue = "kserve-llmisvc-controller"

// ensureServedByConfigMap creates the middleware ConfigMap as immutable.
// If the content is stale (controller upgrade), the old ConfigMap is deleted
// and recreated. Immutability prevents namespace actors with ConfigMap write
// access from replacing the middleware with arbitrary code.
// Returns (true, nil) when the ConfigMap is ready, (false, nil) when creation
// was skipped (namespace terminating or name conflict), or (false, err) on failure.
func (r *LLMISVCReconciler) ensureServedByConfigMap(ctx context.Context, namespace string) (bool, error) {
	immutable := true
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      servedByConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": servedByManagedByValue,
			},
		},
		Immutable: &immutable,
		Data: map[string]string{
			"__init__.py":  "",
			"served_by.py": servedByMiddlewareSource,
		},
	}

	existing := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return false, fmt.Errorf("failed to get served-by middleware ConfigMap: %w", err)
		}
		if err := r.Create(ctx, desired); err != nil {
			if apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
				return false, nil
			}
			if createErr := client.IgnoreAlreadyExists(err); createErr != nil {
				return false, fmt.Errorf("failed to create served-by middleware ConfigMap: %w", createErr)
			}
			return true, nil
		}
		return true, nil
	}

	if existing.Data["served_by.py"] == servedByMiddlewareSource {
		return true, nil
	}

	if existing.Labels["app.kubernetes.io/managed-by"] != servedByManagedByValue {
		log.FromContext(ctx).Info("served-by ConfigMap exists but is not controller-managed, skipping injection",
			"configmap", servedByConfigMapName, "namespace", namespace)
		return false, nil
	}

	// Content mismatch (controller upgrade) - delete and recreate since
	// immutable ConfigMaps can't be updated.
	if err := r.Delete(ctx, existing); err != nil {
		return false, fmt.Errorf("failed to delete stale served-by middleware ConfigMap: %w", err)
	}
	if err := r.Create(ctx, desired); err != nil {
		if apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
			return false, nil
		}
		if createErr := client.IgnoreAlreadyExists(err); createErr != nil {
			return false, fmt.Errorf("failed to recreate served-by middleware ConfigMap: %w", createErr)
		}
	}
	return true, nil
}

// addVolume adds a volume to the pod spec if not already present (by name).
func addVolume(podSpec *corev1.PodSpec, vol corev1.Volume) {
	for _, existing := range podSpec.Volumes {
		if existing.Name == vol.Name {
			return
		}
	}
	podSpec.Volumes = append(podSpec.Volumes, vol)
}
