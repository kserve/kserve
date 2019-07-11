/*
Copyright 2019 The Knative Authors

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

package serving

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/networking"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	minUserID = 0
	maxUserID = math.MaxInt32
)

var (
	reservedPaths = sets.NewString(
		"/",
		"/dev",
		"/dev/log", // Should be a domain socket
		"/tmp",
		"/var",
		"/var/log",
	)

	reservedContainerNames = sets.NewString(
		"queue-proxy",
	)

	reservedEnvVars = sets.NewString(
		"PORT",
		"K_SERVICE",
		"K_CONFIGURATION",
		"K_REVISION",
	)

	// The port is named "user-port" on the deployment, but a user cannot set an arbitrary name on the port
	// in Configuration. The name field is reserved for content-negotiation. Currently 'h2c' and 'http1' are
	// allowed.
	// https://github.com/knative/serving/blob/master/docs/runtime-contract.md#inbound-network-connectivity
	validPortNames = sets.NewString(
		"h2c",
		"http1",
		"",
	)
)

func ValidateVolumes(vs []corev1.Volume) (sets.String, *apis.FieldError) {
	volumes := sets.NewString()
	var errs *apis.FieldError
	for i, volume := range vs {
		if volumes.Has(volume.Name) {
			errs = errs.Also((&apis.FieldError{
				Message: fmt.Sprintf("duplicate volume name %q", volume.Name),
				Paths:   []string{"name"},
			}).ViaIndex(i))
		}
		errs = errs.Also(validateVolume(volume).ViaIndex(i))
		volumes.Insert(volume.Name)
	}
	return volumes, errs
}

func validateVolume(volume corev1.Volume) *apis.FieldError {
	errs := apis.CheckDisallowedFields(volume, *VolumeMask(&volume))
	if volume.Name == "" {
		errs = apis.ErrMissingField("name")
	} else if len(validation.IsDNS1123Label(volume.Name)) != 0 {
		errs = apis.ErrInvalidValue(volume.Name, "name")
	}

	vs := volume.VolumeSource
	errs = errs.Also(apis.CheckDisallowedFields(vs, *VolumeSourceMask(&vs)))
	specified := []string{}
	if vs.Secret != nil {
		specified = append(specified, "secret")
	}
	if vs.ConfigMap != nil {
		specified = append(specified, "configMap")
	}
	if vs.Projected != nil {
		specified = append(specified, "projected")
		for i, proj := range vs.Projected.Sources {
			errs = errs.Also(validateProjectedVolumeSource(proj).ViaFieldIndex("projected", i))
		}
	}
	if len(specified) == 0 {
		errs = errs.Also(apis.ErrMissingOneOf("secret", "configMap", "projected"))
	} else if len(specified) > 1 {
		errs = errs.Also(apis.ErrMultipleOneOf(specified...))
	}

	return errs
}

func validateProjectedVolumeSource(vp corev1.VolumeProjection) *apis.FieldError {
	errs := apis.CheckDisallowedFields(vp, *VolumeProjectionMask(&vp))
	specified := []string{}
	if vp.Secret != nil {
		specified = append(specified, "secret")
		errs = errs.Also(validateSecretProjection(vp.Secret).ViaField("secret"))
	}
	if vp.ConfigMap != nil {
		specified = append(specified, "configMap")
		errs = errs.Also(validateConfigMapProjection(vp.ConfigMap).ViaField("configMap"))
	}
	if len(specified) == 0 {
		errs = errs.Also(apis.ErrMissingOneOf("secret", "configMap"))
	} else if len(specified) > 1 {
		errs = errs.Also(apis.ErrMultipleOneOf(specified...))
	}
	return errs
}

func validateConfigMapProjection(cmp *corev1.ConfigMapProjection) *apis.FieldError {
	errs := apis.CheckDisallowedFields(*cmp, *ConfigMapProjectionMask(cmp))
	errs = errs.Also(apis.CheckDisallowedFields(
		cmp.LocalObjectReference, *LocalObjectReferenceMask(&cmp.LocalObjectReference)))
	if cmp.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	}
	for i, item := range cmp.Items {
		errs = errs.Also(apis.CheckDisallowedFields(item, *KeyToPathMask(&item)).ViaIndex(i))
	}
	return errs
}

func validateSecretProjection(sp *corev1.SecretProjection) *apis.FieldError {
	errs := apis.CheckDisallowedFields(*sp, *SecretProjectionMask(sp))
	errs = errs.Also(apis.CheckDisallowedFields(
		sp.LocalObjectReference, *LocalObjectReferenceMask(&sp.LocalObjectReference)))
	if sp.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	}
	for i, item := range sp.Items {
		errs = errs.Also(apis.CheckDisallowedFields(item, *KeyToPathMask(&item)).ViaIndex(i))
	}
	return errs
}

func validateEnvValueFrom(source *corev1.EnvVarSource) *apis.FieldError {
	if source == nil {
		return nil
	}
	return apis.CheckDisallowedFields(*source, *EnvVarSourceMask(source))
}

func validateEnvVar(env corev1.EnvVar) *apis.FieldError {
	errs := apis.CheckDisallowedFields(env, *EnvVarMask(&env))

	if env.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	} else if reservedEnvVars.Has(env.Name) {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("%q is a reserved environment variable", env.Name),
			Paths:   []string{"name"},
		})
	}

	return errs.Also(validateEnvValueFrom(env.ValueFrom).ViaField("valueFrom"))
}

func validateEnv(envVars []corev1.EnvVar) *apis.FieldError {
	var errs *apis.FieldError
	for i, env := range envVars {
		errs = errs.Also(validateEnvVar(env).ViaIndex(i))
	}
	return errs
}

func validateEnvFrom(envFromList []corev1.EnvFromSource) *apis.FieldError {
	var errs *apis.FieldError
	for i, envFrom := range envFromList {
		errs = errs.Also(apis.CheckDisallowedFields(envFrom, *EnvFromSourceMask(&envFrom)).ViaIndex(i))

		cm := envFrom.ConfigMapRef
		sm := envFrom.SecretRef
		if sm != nil {
			errs = errs.Also(apis.CheckDisallowedFields(*sm, *SecretEnvSourceMask(sm)).ViaIndex(i))
			errs = errs.Also(apis.CheckDisallowedFields(
				sm.LocalObjectReference, *LocalObjectReferenceMask(&sm.LocalObjectReference))).ViaIndex(i).ViaField("secretRef")
		}

		if cm != nil {
			errs = errs.Also(apis.CheckDisallowedFields(*cm, *ConfigMapEnvSourceMask(cm)).ViaIndex(i))
			errs = errs.Also(apis.CheckDisallowedFields(
				cm.LocalObjectReference, *LocalObjectReferenceMask(&cm.LocalObjectReference))).ViaIndex(i).ViaField("configMapRef")
		}
		if cm != nil && sm != nil {
			errs = errs.Also(apis.ErrMultipleOneOf("configMapRef", "secretRef"))
		} else if cm == nil && sm == nil {
			errs = errs.Also(apis.ErrMissingOneOf("configMapRef", "secretRef"))
		}
	}
	return errs
}

func ValidatePodSpec(ps corev1.PodSpec) *apis.FieldError {
	// This is inlined, and so it makes for a less meaningful
	// error message.
	// if equality.Semantic.DeepEqual(ps, corev1.PodSpec{}) {
	// 	return apis.ErrMissingField(apis.CurrentField)
	// }

	errs := apis.CheckDisallowedFields(ps, *PodSpecMask(&ps))

	volumes, err := ValidateVolumes(ps.Volumes)
	if err != nil {
		errs = errs.Also(err.ViaField("volumes"))
	}

	switch len(ps.Containers) {
	case 0:
		errs = errs.Also(apis.ErrMissingField("containers"))
	case 1:
		errs = errs.Also(ValidateContainer(ps.Containers[0], volumes).
			ViaFieldIndex("containers", 0))
	default:
		errs = errs.Also(apis.ErrMultipleOneOf("containers"))
	}
	return errs
}

func ValidateContainer(container corev1.Container, volumes sets.String) *apis.FieldError {
	if equality.Semantic.DeepEqual(container, corev1.Container{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}

	errs := apis.CheckDisallowedFields(container, *ContainerMask(&container))

	if reservedContainerNames.Has(container.Name) {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("%q is a reserved container name", container.Name),
			Paths:   []string{"name"},
		})
	}

	// Env
	errs = errs.Also(validateEnv(container.Env).ViaField("env"))
	// EnvFrom
	errs = errs.Also(validateEnvFrom(container.EnvFrom).ViaField("envFrom"))
	// Image
	if container.Image == "" {
		errs = errs.Also(apis.ErrMissingField("image"))
	} else if _, err := name.ParseReference(container.Image, name.WeakValidation); err != nil {
		fe := &apis.FieldError{
			Message: "Failed to parse image reference",
			Paths:   []string{"image"},
			Details: fmt.Sprintf("image: %q, error: %v", container.Image, err),
		}
		errs = errs.Also(fe)
	}
	// Liveness Probes
	errs = errs.Also(validateProbe(container.LivenessProbe).ViaField("livenessProbe"))
	// Ports
	errs = errs.Also(validateContainerPorts(container.Ports).ViaField("ports"))
	// Readiness Probes
	errs = errs.Also(validateProbe(container.ReadinessProbe).ViaField("readinessProbe"))
	// Resources
	errs = errs.Also(validateResources(&container.Resources).ViaField("resources"))
	// SecurityContext
	errs = errs.Also(validateSecurityContext(container.SecurityContext).ViaField("securityContext"))
	// TerminationMessagePolicy
	switch container.TerminationMessagePolicy {
	case corev1.TerminationMessageReadFile, corev1.TerminationMessageFallbackToLogsOnError, "":
	default:
		errs = errs.Also(apis.ErrInvalidValue(container.TerminationMessagePolicy, "terminationMessagePolicy"))
	}
	// VolumeMounts
	errs = errs.Also(validateVolumeMounts(container.VolumeMounts, volumes).ViaField("volumeMounts"))

	return errs
}

func validateResources(resources *corev1.ResourceRequirements) *apis.FieldError {
	if resources == nil {
		return nil
	}
	return apis.CheckDisallowedFields(*resources, *ResourceRequirementsMask(resources))
}

func validateSecurityContext(sc *corev1.SecurityContext) *apis.FieldError {
	if sc == nil {
		return nil
	}
	errs := apis.CheckDisallowedFields(*sc, *SecurityContextMask(sc))

	if sc.RunAsUser != nil {
		uid := *sc.RunAsUser
		if uid < minUserID || uid > maxUserID {
			errs = errs.Also(apis.ErrOutOfBoundsValue(uid, minUserID, maxUserID, "runAsUser"))
		}
	}
	return errs
}

func validateVolumeMounts(mounts []corev1.VolumeMount, volumes sets.String) *apis.FieldError {
	var errs *apis.FieldError
	// Check that volume mounts match names in "volumes", that "volumes" has 100%
	// coverage, and the field restrictions.
	seenName := sets.NewString()
	seenMountPath := sets.NewString()
	for i, vm := range mounts {
		errs = errs.Also(apis.CheckDisallowedFields(vm, *VolumeMountMask(&vm)).ViaIndex(i))
		// This effectively checks that Name is non-empty because Volume name must be non-empty.
		if !volumes.Has(vm.Name) {
			errs = errs.Also((&apis.FieldError{
				Message: "volumeMount has no matching volume",
				Paths:   []string{"name"},
			}).ViaIndex(i))
		}
		seenName.Insert(vm.Name)

		if vm.MountPath == "" {
			errs = errs.Also(apis.ErrMissingField("mountPath").ViaIndex(i))
		} else if reservedPaths.Has(filepath.Clean(vm.MountPath)) {
			errs = errs.Also((&apis.FieldError{
				Message: fmt.Sprintf("mountPath %q is a reserved path", filepath.Clean(vm.MountPath)),
				Paths:   []string{"mountPath"},
			}).ViaIndex(i))
		} else if !filepath.IsAbs(vm.MountPath) {
			errs = errs.Also(apis.ErrInvalidValue(vm.MountPath, "mountPath").ViaIndex(i))
		} else if seenMountPath.Has(filepath.Clean(vm.MountPath)) {
			errs = errs.Also(apis.ErrInvalidValue(
				fmt.Sprintf("%q must be unique", vm.MountPath), "mountPath").ViaIndex(i))
		}
		seenMountPath.Insert(filepath.Clean(vm.MountPath))

		if !vm.ReadOnly {
			errs = errs.Also(apis.ErrMissingField("readOnly").ViaIndex(i))
		}

	}

	if missing := volumes.Difference(seenName); missing.Len() > 0 {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("volumes not mounted: %v", missing.List()),
			Paths:   []string{apis.CurrentField},
		})
	}
	return errs
}

func validateContainerPorts(ports []corev1.ContainerPort) *apis.FieldError {
	if len(ports) == 0 {
		return nil
	}

	var errs *apis.FieldError

	// user can set container port which names "user-port" to define application's port.
	// Queue-proxy will use it to send requests to application
	// if user didn't set any port, it will set default port user-port=8080.
	if len(ports) > 1 {
		errs = errs.Also(&apis.FieldError{
			Message: "More than one container port is set",
			Paths:   []string{apis.CurrentField},
			Details: "Only a single port is allowed",
		})
	}

	userPort := ports[0]

	errs = errs.Also(apis.CheckDisallowedFields(userPort, *ContainerPortMask(&userPort)))

	// Only allow empty (defaulting to "TCP") or explicit TCP for protocol
	if userPort.Protocol != "" && userPort.Protocol != corev1.ProtocolTCP {
		errs = errs.Also(apis.ErrInvalidValue(userPort.Protocol, "protocol"))
	}

	// Don't allow userPort to conflict with QueueProxy sidecar
	if userPort.ContainerPort == networking.BackendHTTPPort ||
		userPort.ContainerPort == networking.BackendHTTP2Port ||
		userPort.ContainerPort == networking.QueueAdminPort ||
		userPort.ContainerPort == networking.AutoscalingQueueMetricsPort ||
		userPort.ContainerPort == networking.UserQueueMetricsPort {
		errs = errs.Also(apis.ErrInvalidValue(userPort.ContainerPort, "containerPort"))
	}

	if userPort.ContainerPort < 1 || userPort.ContainerPort > 65535 {
		errs = errs.Also(apis.ErrOutOfBoundsValue(userPort.ContainerPort,
			1, 65535, "containerPort"))
	}

	if !validPortNames.Has(userPort.Name) {
		errs = errs.Also(&apis.FieldError{
			Message: fmt.Sprintf("Port name %v is not allowed", ports[0].Name),
			Paths:   []string{apis.CurrentField},
			Details: "Name must be empty, or one of: 'h2c', 'http1'",
		})
	}

	return errs
}

func validateProbe(p *corev1.Probe) *apis.FieldError {
	if p == nil {
		return nil
	}
	errs := apis.CheckDisallowedFields(*p, *ProbeMask(p))

	h := p.Handler
	errs = errs.Also(apis.CheckDisallowedFields(h, *HandlerMask(&h)))

	switch {
	case h.HTTPGet != nil:
		errs = errs.Also(apis.CheckDisallowedFields(*h.HTTPGet, *HTTPGetActionMask(h.HTTPGet))).ViaField("httpGet")
	case h.TCPSocket != nil:
		errs = errs.Also(apis.CheckDisallowedFields(*h.TCPSocket, *TCPSocketActionMask(h.TCPSocket))).ViaField("tcpSocket")
	}
	return errs
}

func ValidateNamespacedObjectReference(p *corev1.ObjectReference) *apis.FieldError {
	if p == nil {
		return nil
	}
	errs := apis.CheckDisallowedFields(*p, *NamespacedObjectReferenceMask(p))

	if p.APIVersion == "" {
		errs = errs.Also(apis.ErrMissingField("apiVersion"))
	} else if verrs := validation.IsQualifiedName(p.APIVersion); len(verrs) != 0 {
		errs = errs.Also(apis.ErrInvalidValue(strings.Join(verrs, ", "), "apiVersion"))
	}
	if p.Kind == "" {
		errs = errs.Also(apis.ErrMissingField("kind"))
	} else if verrs := validation.IsCIdentifier(p.Kind); len(verrs) != 0 {
		errs = errs.Also(apis.ErrInvalidValue(strings.Join(verrs, ", "), "kind"))
	}
	if p.Name == "" {
		errs = errs.Also(apis.ErrMissingField("name"))
	} else if verrs := validation.IsDNS1123Label(p.Name); len(verrs) != 0 {
		errs = errs.Also(apis.ErrInvalidValue(strings.Join(verrs, ", "), "name"))
	}
	return errs
}
