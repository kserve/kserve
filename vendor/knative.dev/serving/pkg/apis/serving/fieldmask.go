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
	corev1 "k8s.io/api/core/v1"
)

// VolumeMask performs a _shallow_ copy of the Kubernetes Volume object to a new
// Kubernetes Volume object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func VolumeMask(in *corev1.Volume) *corev1.Volume {
	if in == nil {
		return nil
	}

	out := new(corev1.Volume)

	// Allowed fields
	out.Name = in.Name
	out.VolumeSource = in.VolumeSource

	return out
}

// VolumeSourceMask performs a _shallow_ copy of the Kubernetes VolumeSource object to a new
// Kubernetes VolumeSource object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func VolumeSourceMask(in *corev1.VolumeSource) *corev1.VolumeSource {
	if in == nil {
		return nil
	}

	out := new(corev1.VolumeSource)

	// Allowed fields
	out.Secret = in.Secret
	out.ConfigMap = in.ConfigMap
	out.Projected = in.Projected

	// Too many disallowed fields to list

	return out
}

// VolumeProjectionMask performs a _shallow_ copy of the Kubernetes VolumeProjection
// object to a new Kubernetes VolumeProjection object bringing over only the fields allowed
// in the Knative API. This does not validate the contents or the bounds of the provided fields.
func VolumeProjectionMask(in *corev1.VolumeProjection) *corev1.VolumeProjection {
	if in == nil {
		return nil
	}

	out := new(corev1.VolumeProjection)

	// Allowed fields
	out.Secret = in.Secret
	out.ConfigMap = in.ConfigMap

	// Disallowed fields
	// This list is unnecessary, but added here for clarity
	out.DownwardAPI = nil
	out.ServiceAccountToken = nil

	return out
}

// ConfigMapProjectionMask performs a _shallow_ copy of the Kubernetes ConfigMapProjection
// object to a new Kubernetes ConfigMapProjection object bringing over only the fields allowed
// in the Knative API. This does not validate the contents or the bounds of the provided fields.
func ConfigMapProjectionMask(in *corev1.ConfigMapProjection) *corev1.ConfigMapProjection {
	if in == nil {
		return nil
	}

	out := new(corev1.ConfigMapProjection)

	// Allowed fields
	out.LocalObjectReference = in.LocalObjectReference
	out.Items = in.Items
	out.Optional = in.Optional

	return out
}

// SecretProjectionMask performs a _shallow_ copy of the Kubernetes SecretProjection
// object to a new Kubernetes SecretProjection object bringing over only the fields allowed
// in the Knative API. This does not validate the contents or the bounds of the provided fields.
func SecretProjectionMask(in *corev1.SecretProjection) *corev1.SecretProjection {
	if in == nil {
		return nil
	}

	out := new(corev1.SecretProjection)

	// Allowed fields
	out.LocalObjectReference = in.LocalObjectReference
	out.Items = in.Items
	out.Optional = in.Optional

	return out
}

// KeyToPathMask performs a _shallow_ copy of the Kubernetes KeyToPath
// object to a new Kubernetes KeyToPath object bringing over only the fields allowed
// in the Knative API. This does not validate the contents or the bounds of the provided fields.
func KeyToPathMask(in *corev1.KeyToPath) *corev1.KeyToPath {
	if in == nil {
		return nil
	}

	out := new(corev1.KeyToPath)

	// Allowed fields
	out.Key = in.Key
	out.Path = in.Path
	out.Mode = in.Mode

	return out
}

// PodSpecMask performs a _shallow_ copy of the Kubernetes PodSpec object to a new
// Kubernetes PodSpec object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func PodSpecMask(in *corev1.PodSpec) *corev1.PodSpec {
	if in == nil {
		return nil
	}

	out := new(corev1.PodSpec)

	// Allowed fields
	out.ServiceAccountName = in.ServiceAccountName
	out.Containers = in.Containers
	out.Volumes = in.Volumes

	// Disallowed fields
	// This list is unnecessary, but added here for clarity
	out.InitContainers = nil
	out.RestartPolicy = ""
	out.TerminationGracePeriodSeconds = nil
	out.ActiveDeadlineSeconds = nil
	out.DNSPolicy = ""
	out.NodeSelector = nil
	out.AutomountServiceAccountToken = nil
	out.NodeName = ""
	out.HostNetwork = false
	out.HostPID = false
	out.HostIPC = false
	out.ShareProcessNamespace = nil
	out.SecurityContext = nil
	out.ImagePullSecrets = nil
	out.Hostname = ""
	out.Subdomain = ""
	out.Affinity = nil
	out.SchedulerName = ""
	out.Tolerations = nil
	out.HostAliases = nil
	out.PriorityClassName = ""
	out.Priority = nil
	out.DNSConfig = nil
	out.ReadinessGates = nil
	out.RuntimeClassName = nil
	// TODO(mattmoor): Coming in 1.13: out.EnableServiceLinks = nil

	return out
}

// ContainerMask performs a _shallow_ copy of the Kubernetes Container object to a new
// Kubernetes Container object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ContainerMask(in *corev1.Container) *corev1.Container {
	if in == nil {
		return nil
	}

	out := new(corev1.Container)

	// Allowed fields
	out.Name = in.Name
	out.Args = in.Args
	out.Command = in.Command
	out.Env = in.Env
	out.WorkingDir = in.WorkingDir
	out.EnvFrom = in.EnvFrom
	out.Image = in.Image
	out.ImagePullPolicy = in.ImagePullPolicy
	out.LivenessProbe = in.LivenessProbe
	out.Ports = in.Ports
	out.ReadinessProbe = in.ReadinessProbe
	out.Resources = in.Resources
	out.SecurityContext = in.SecurityContext
	out.TerminationMessagePath = in.TerminationMessagePath
	out.TerminationMessagePolicy = in.TerminationMessagePolicy
	out.VolumeMounts = in.VolumeMounts

	// Disallowed fields
	// This list is unnecessary, but added here for clarity
	out.Lifecycle = nil
	out.Stdin = false
	out.StdinOnce = false
	out.TTY = false
	out.VolumeDevices = nil

	return out
}

// VolumeMountMask performs a _shallow_ copy of the Kubernetes VolumeMount object to a new
// Kubernetes VolumeMount object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func VolumeMountMask(in *corev1.VolumeMount) *corev1.VolumeMount {
	if in == nil {
		return nil
	}

	out := new(corev1.VolumeMount)

	// Allowed fields
	out.Name = in.Name
	out.ReadOnly = in.ReadOnly
	out.MountPath = in.MountPath
	out.SubPath = in.SubPath

	// Disallowed fields
	// This list is unnecessary, but added here for clarity
	out.MountPropagation = nil

	return out
}

// ProbeMask performs a _shallow_ copy of the Kubernetes Probe object to a new
// Kubernetes Probe object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ProbeMask(in *corev1.Probe) *corev1.Probe {
	if in == nil {
		return nil
	}
	out := new(corev1.Probe)

	// Allowed fields
	out.Handler = in.Handler
	out.InitialDelaySeconds = in.InitialDelaySeconds
	out.TimeoutSeconds = in.TimeoutSeconds
	out.PeriodSeconds = in.PeriodSeconds
	out.SuccessThreshold = in.SuccessThreshold
	out.FailureThreshold = in.FailureThreshold

	return out
}

// HandlerMask performs a _shallow_ copy of the Kubernetes Handler object to a new
// Kubernetes Handler object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func HandlerMask(in *corev1.Handler) *corev1.Handler {
	if in == nil {
		return nil
	}
	out := new(corev1.Handler)

	// Allowed fields
	out.Exec = in.Exec
	out.HTTPGet = in.HTTPGet
	out.TCPSocket = in.TCPSocket

	return out

}

// ExecActionMask performs a _shallow_ copy of the Kubernetes ExecAction object to a new
// Kubernetes ExecAction object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ExecActionMask(in *corev1.ExecAction) *corev1.ExecAction {
	if in == nil {
		return nil
	}
	out := new(corev1.ExecAction)

	// Allowed fields
	out.Command = in.Command

	return out
}

// HTTPGetActionMask performs a _shallow_ copy of the Kubernetes HTTPGetAction object to a new
// Kubernetes HTTPGetAction object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func HTTPGetActionMask(in *corev1.HTTPGetAction) *corev1.HTTPGetAction {
	if in == nil {
		return nil
	}
	out := new(corev1.HTTPGetAction)

	// Allowed fields
	out.Host = in.Host
	out.Path = in.Path
	out.Scheme = in.Scheme
	out.HTTPHeaders = in.HTTPHeaders

	return out
}

// TCPSocketActionMask performs a _shallow_ copy of the Kubernetes TCPSocketAction object to a new
// Kubernetes TCPSocketAction object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func TCPSocketActionMask(in *corev1.TCPSocketAction) *corev1.TCPSocketAction {
	if in == nil {
		return nil
	}
	out := new(corev1.TCPSocketAction)

	// Allowed fields
	out.Host = in.Host

	return out
}

// ContainerPortMask performs a _shallow_ copy of the Kubernetes ContainerPort object to a new
// Kubernetes ContainerPort object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ContainerPortMask(in *corev1.ContainerPort) *corev1.ContainerPort {
	if in == nil {
		return nil
	}

	out := new(corev1.ContainerPort)

	// Allowed fields
	out.ContainerPort = in.ContainerPort
	out.Name = in.Name
	out.Protocol = in.Protocol

	//Disallowed fields
	// This list is unnecessary, but added here for clarity
	out.HostIP = ""
	out.HostPort = 0

	return out
}

// EnvVarMask performs a _shallow_ copy of the Kubernetes EnvVar object to a new
// Kubernetes EnvVar object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func EnvVarMask(in *corev1.EnvVar) *corev1.EnvVar {
	if in == nil {
		return nil
	}

	out := new(corev1.EnvVar)

	// Allowed fields
	out.Name = in.Name
	out.Value = in.Value
	out.ValueFrom = in.ValueFrom

	return out
}

// EnvVarSourceMask performs a _shallow_ copy of the Kubernetes EnvVarSource object to a new
// Kubernetes EnvVarSource object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func EnvVarSourceMask(in *corev1.EnvVarSource) *corev1.EnvVarSource {
	if in == nil {
		return nil
	}

	out := new(corev1.EnvVarSource)

	// Allowed fields
	out.ConfigMapKeyRef = in.ConfigMapKeyRef
	out.SecretKeyRef = in.SecretKeyRef

	// Disallowed
	// This list is unnecessary, but added here for clarity
	out.FieldRef = nil
	out.ResourceFieldRef = nil

	return out
}

// LocalObjectReferenceMask performs a _shallow_ copy of the Kubernetes LocalObjectReference object to a new
// Kubernetes LocalObjectReference object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func LocalObjectReferenceMask(in *corev1.LocalObjectReference) *corev1.LocalObjectReference {
	if in == nil {
		return nil
	}

	out := new(corev1.LocalObjectReference)

	out.Name = in.Name

	return out
}

// ConfigMapKeySelectorMask performs a _shallow_ copy of the Kubernetes ConfigMapKeySelector object to a new
// Kubernetes ConfigMapKeySelector object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ConfigMapKeySelectorMask(in *corev1.ConfigMapKeySelector) *corev1.ConfigMapKeySelector {
	if in == nil {
		return nil
	}

	out := new(corev1.ConfigMapKeySelector)

	// Allowed fields
	out.Key = in.Key
	out.Optional = in.Optional
	out.LocalObjectReference = in.LocalObjectReference

	return out

}

// SecretKeySelectorMask performs a _shallow_ copy of the Kubernetes SecretKeySelector object to a new
// Kubernetes SecretKeySelector object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func SecretKeySelectorMask(in *corev1.SecretKeySelector) *corev1.SecretKeySelector {
	if in == nil {
		return nil
	}

	out := new(corev1.SecretKeySelector)

	// Allowed fields
	out.Key = in.Key
	out.Optional = in.Optional
	out.LocalObjectReference = in.LocalObjectReference

	return out

}

// ConfigMapEnvSourceMask performs a _shallow_ copy of the Kubernetes ConfigMapEnvSource object to a new
// Kubernetes ConfigMapEnvSource object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ConfigMapEnvSourceMask(in *corev1.ConfigMapEnvSource) *corev1.ConfigMapEnvSource {
	if in == nil {
		return nil
	}

	out := new(corev1.ConfigMapEnvSource)

	// Allowed fields
	out.Optional = in.Optional
	out.LocalObjectReference = in.LocalObjectReference

	return out

}

// SecretEnvSourceMask performs a _shallow_ copy of the Kubernetes SecretEnvSource object to a new
// Kubernetes SecretEnvSource object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func SecretEnvSourceMask(in *corev1.SecretEnvSource) *corev1.SecretEnvSource {
	if in == nil {
		return nil
	}

	out := new(corev1.SecretEnvSource)

	// Allowed fields
	out.Optional = in.Optional
	out.LocalObjectReference = in.LocalObjectReference

	return out

}

// EnvFromSourceMask performs a _shallow_ copy of the Kubernetes EnvFromSource object to a new
// Kubernetes EnvFromSource object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func EnvFromSourceMask(in *corev1.EnvFromSource) *corev1.EnvFromSource {
	if in == nil {
		return nil
	}

	out := new(corev1.EnvFromSource)

	// Allowed fields
	out.Prefix = in.Prefix
	out.ConfigMapRef = in.ConfigMapRef
	out.SecretRef = in.SecretRef

	return out
}

// ResourceRequirementsMask performs a _shallow_ copy of the Kubernetes ResourceRequirements object to a new
// Kubernetes ResourceRequirements object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func ResourceRequirementsMask(in *corev1.ResourceRequirements) *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}

	out := new(corev1.ResourceRequirements)

	// Allowed fields
	out.Limits = in.Limits
	out.Requests = in.Requests

	return out

}

// SecurityContextMask performs a _shallow_ copy of the Kubernetes SecurityContext object to a new
// Kubernetes SecurityContext object bringing over only the fields allowed in the Knative API. This
// does not validate the contents or the bounds of the provided fields.
func SecurityContextMask(in *corev1.SecurityContext) *corev1.SecurityContext {
	if in == nil {
		return nil
	}

	out := new(corev1.SecurityContext)

	// Allowed fields
	out.RunAsUser = in.RunAsUser

	// Disallowed
	// This list is unnecessary, but added here for clarity
	out.Capabilities = nil
	out.Privileged = nil
	out.SELinuxOptions = nil
	out.RunAsGroup = nil
	out.RunAsNonRoot = nil
	out.ReadOnlyRootFilesystem = nil
	out.AllowPrivilegeEscalation = nil
	out.ProcMount = nil

	return out
}

// NamespacedObjectReferenceMask performs a _shallow_ copy of the Kubernetes ObjectReference
// object to a new Kubernetes ObjectReference object bringing over only the fields allowed in
// the Knative API. This does not validate the contents or the bounds of the provided fields.
func NamespacedObjectReferenceMask(in *corev1.ObjectReference) *corev1.ObjectReference {
	if in == nil {
		return nil
	}

	out := new(corev1.ObjectReference)

	// Allowed fields
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name

	// Disallowed
	// This list is unnecessary, but added here for clarity
	out.Namespace = ""
	out.FieldPath = ""
	out.ResourceVersion = ""
	out.UID = ""

	return out
}
