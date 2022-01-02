/*
Copyright 2021 The KServe Authors.

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

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
)

// ***This is a copy of kubernetes corev1.PodSpec to make field Containers optional***
// PodSpec on InferenceService is for a custom component or an escape hatch for builtin component, so Containers field
// needs to be optional

// PodSpec is a description of a pod.
type PodSpec struct {
	// List of volumes that can be mounted by containers belonging to the pod.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge,retainKeys
	Volumes []v1.Volume `json:"volumes,omitempty" patchStrategy:"merge,retainKeys" patchMergeKey:"name" protobuf:"bytes,1,rep,name=volumes"`
	// List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started. If any
	// init container fails, the pod is considered to have failed and is handled according
	// to its restartPolicy. The name for an init container or normal container must be
	// unique among all containers.
	// Init containers may not have Lifecycle actions, Readiness probes, Liveness probes, or Startup probes.
	// The resourceRequirements of an init container are taken into account during scheduling
	// by finding the highest request/limit for each resource type, and then using the max of
	// of that value or the sum of the normal containers. Limits are applied to init containers
	// in a similar fashion.
	// Init containers cannot currently be added or removed.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +patchMergeKey=name
	// +patchStrategy=merge
	InitContainers []v1.Container `json:"initContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,20,rep,name=initContainers"`
	// List of containers belonging to the pod.
	// Containers cannot currently be added or removed.
	// There must be at least one container in a Pod.
	// Cannot be updated.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []v1.Container `json:"containers,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=containers"`
	// List of ephemeral containers run in this pod. Ephemeral containers may be run in an existing
	// pod to perform user-initiated actions such as debugging. This list cannot be specified when
	// creating a pod, and it cannot be modified by updating the pod spec. In order to add an
	// ephemeral container to an existing pod, use the pod's ephemeralcontainers subresource.
	// This field is alpha-level and is only honored by servers that enable the EphemeralContainers feature.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +kubebuilder:pruning:PreserveUnknownFields
	EphemeralContainers []v1.EphemeralContainer `json:"ephemeralContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,34,rep,name=ephemeralContainers"`
	// Restart policy for all containers within the pod.
	// One of Always, OnFailure, Never.
	// Default to Always.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy
	// +optional
	RestartPolicy v1.RestartPolicy `json:"restartPolicy,omitempty" protobuf:"bytes,3,opt,name=restartPolicy,casttype=RestartPolicy"`
	// Optional duration in seconds the pod needs to terminate gracefully. May be decreased in delete request.
	// Value must be non-negative integer. The value zero indicates delete immediately.
	// If this value is nil, the default grace period will be used instead.
	// The grace period is the duration in seconds after the processes running in the pod are sent
	// a termination signal and the time when the processes are forcibly halted with a kill signal.
	// Set this value longer than the expected cleanup time for your process.
	// Defaults to 30 seconds.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty" protobuf:"varint,4,opt,name=terminationGracePeriodSeconds"`
	// Optional duration in seconds the pod may be active on the node relative to
	// StartTime before the system will actively try to mark it failed and kill associated containers.
	// Value must be a positive integer.
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,5,opt,name=activeDeadlineSeconds"`
	// Set DNS policy for the pod.
	// Defaults to "ClusterFirst".
	// Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'.
	// DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy.
	// To have DNS options set along with hostNetwork, you have to specify DNS policy
	// explicitly to 'ClusterFirstWithHostNet'.
	// +optional
	DNSPolicy v1.DNSPolicy `json:"dnsPolicy,omitempty" protobuf:"bytes,6,opt,name=dnsPolicy,casttype=DNSPolicy"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// ServiceAccountName is the name of the ServiceAccount to use to run this pod.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty" protobuf:"bytes,8,opt,name=serviceAccountName"`
	// DeprecatedServiceAccount is a depreciated alias for ServiceAccountName.
	// Deprecated: Use serviceAccountName instead.
	// +k8s:conversion-gen=false
	// +optional
	DeprecatedServiceAccount string `json:"serviceAccount,omitempty" protobuf:"bytes,9,opt,name=serviceAccount"`
	// AutomountServiceAccountToken indicates whether a service account token should be automatically mounted.
	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty" protobuf:"varint,21,opt,name=automountServiceAccountToken"`

	// NodeName is a request to schedule this pod onto a specific node. If it is non-empty,
	// the scheduler simply schedules this pod onto that node, assuming that it fits resource
	// requirements.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
	// Host networking requested for this pod. Use the host's network namespace.
	// If this option is set, the ports that will be used must be specified.
	// Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty" protobuf:"varint,11,opt,name=hostNetwork"`
	// Use the host's pid namespace.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostPID bool `json:"hostPID,omitempty" protobuf:"varint,12,opt,name=hostPID"`
	// Use the host's ipc namespace.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostIPC bool `json:"hostIPC,omitempty" protobuf:"varint,13,opt,name=hostIPC"`
	// Share a single process namespace between all of the containers in a pod.
	// When this is set containers will be able to view and signal processes from other containers
	// in the same pod, and the first process in each container will not be assigned PID 1.
	// HostPID and ShareProcessNamespace cannot both be set.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	ShareProcessNamespace *bool `json:"shareProcessNamespace,omitempty" protobuf:"varint,27,opt,name=shareProcessNamespace"`
	// SecurityContext holds pod-level security attributes and common container settings.
	// Optional: Defaults to empty.  See type description for default values of each field.
	// +optional
	SecurityContext *v1.PodSecurityContext `json:"securityContext,omitempty" protobuf:"bytes,14,opt,name=securityContext"`
	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use. For example,
	// in the case of docker, only DockerConfig type secrets are honored.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,15,rep,name=imagePullSecrets"`
	// Specifies the hostname of the Pod
	// If not specified, the pod's hostname will be set to a system-defined value.
	// +optional
	Hostname string `json:"hostname,omitempty" protobuf:"bytes,16,opt,name=hostname"`
	// If specified, the fully qualified Pod hostname will be "<hostname>.<subdomain>.<pod namespace>.svc.<cluster domain>".
	// If not specified, the pod will not have a domainname at all.
	// +optional
	Subdomain string `json:"subdomain,omitempty" protobuf:"bytes,17,opt,name=subdomain"`
	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
	// If specified, the pod will be dispatched by specified scheduler.
	// If not specified, the pod will be dispatched by default scheduler.
	// +optional
	SchedulerName string `json:"schedulerName,omitempty" protobuf:"bytes,19,opt,name=schedulerName"`
	// If specified, the pod's tolerations.
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty" protobuf:"bytes,22,opt,name=tolerations"`
	// HostAliases is an optional list of hosts and IPs that will be injected into the pod's hosts
	// file if specified. This is only valid for non-hostNetwork pods.
	// +optional
	// +patchMergeKey=ip
	// +patchStrategy=merge
	HostAliases []v1.HostAlias `json:"hostAliases,omitempty" patchStrategy:"merge" patchMergeKey:"ip" protobuf:"bytes,23,rep,name=hostAliases"`
	// If specified, indicates the pod's priority. "system-node-critical" and
	// "system-cluster-critical" are two special keywords which indicate the
	// highest priorities with the former being the highest priority. Any other
	// name must be defined by creating a PriorityClass object with that name.
	// If not specified, the pod priority will be default or zero if there is no
	// default.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty" protobuf:"bytes,24,opt,name=priorityClassName"`
	// The priority value. Various system components use this field to find the
	// priority of the pod. When Priority Admission Controller is enabled, it
	// prevents users from setting this field. The admission controller populates
	// this field from PriorityClassName.
	// The higher the value, the higher the priority.
	// +optional
	Priority *int32 `json:"priority,omitempty" protobuf:"bytes,25,opt,name=priority"`
	// Specifies the DNS parameters of a pod.
	// Parameters specified here will be merged to the generated DNS
	// configuration based on DNSPolicy.
	// +optional
	DNSConfig *v1.PodDNSConfig `json:"dnsConfig,omitempty" protobuf:"bytes,26,opt,name=dnsConfig"`
	// If specified, all readiness gates will be evaluated for pod readiness.
	// A pod is ready when all its containers are ready AND
	// all conditions specified in the readiness gates have status equal to "True"
	// More info: https://git.k8s.io/enhancements/keps/sig-network/0007-pod-ready%2B%2B.md
	// +optional
	ReadinessGates []v1.PodReadinessGate `json:"readinessGates,omitempty" protobuf:"bytes,28,opt,name=readinessGates"`
	// RuntimeClassName refers to a RuntimeClass object in the node.k8s.io group, which should be used
	// to run this pod.  If no RuntimeClass resource matches the named class, the pod will not be run.
	// If unset or empty, the "legacy" RuntimeClass will be used, which is an implicit class with an
	// empty definition that uses the default runtime handler.
	// More info: https://git.k8s.io/enhancements/keps/sig-node/runtime-class.md
	// This is a beta feature as of Kubernetes v1.14.
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty" protobuf:"bytes,29,opt,name=runtimeClassName"`
	// EnableServiceLinks indicates whether information about services should be injected into pod's
	// environment variables, matching the syntax of Docker links.
	// Optional: Defaults to true.
	// +optional
	EnableServiceLinks *bool `json:"enableServiceLinks,omitempty" protobuf:"varint,30,opt,name=enableServiceLinks"`
	// PreemptionPolicy is the Policy for preempting pods with lower priority.
	// One of Never, PreemptLowerPriority.
	// Defaults to PreemptLowerPriority if unset.
	// This field is beta-level, gated by the NonPreemptingPriority feature-gate.
	// +optional
	PreemptionPolicy *v1.PreemptionPolicy `json:"preemptionPolicy,omitempty" protobuf:"bytes,31,opt,name=preemptionPolicy"`
	// Overhead represents the resource overhead associated with running a pod for a given RuntimeClass.
	// This field will be autopopulated at admission time by the RuntimeClass admission controller. If
	// the RuntimeClass admission controller is enabled, overhead must not be set in Pod create requests.
	// The RuntimeClass admission controller will reject Pod create requests which have the overhead already
	// set. If RuntimeClass is configured and selected in the PodSpec, Overhead will be set to the value
	// defined in the corresponding RuntimeClass, otherwise it will remain unset and treated as zero.
	// More info: https://git.k8s.io/enhancements/keps/sig-node/20190226-pod-overhead.md
	// This field is alpha-level as of Kubernetes v1.16, and is only honored by servers that enable the PodOverhead feature.
	// +optional
	Overhead v1.ResourceList `json:"overhead,omitempty" protobuf:"bytes,32,opt,name=overhead"`
	// TopologySpreadConstraints describes how a group of pods ought to spread across topology
	// domains. Scheduler will schedule pods in a way which abides by the constraints.
	// All topologySpreadConstraints are ANDed.
	// +optional
	// +patchMergeKey=topologyKey
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" patchStrategy:"merge" patchMergeKey:"topologyKey" protobuf:"bytes,33,opt,name=topologySpreadConstraints"`
	// If true the pod's hostname will be configured as the pod's FQDN, rather than the leaf name (the default).
	// In Linux containers, this means setting the FQDN in the hostname field of the kernel (the nodename field of struct utsname).
	// In Windows containers, this means setting the registry value of hostname for the registry key HKEY_LOCAL_MACHINE\\SYSTEM\\CurrentControlSet\\Services\\Tcpip\\Parameters to FQDN.
	// If a pod does not have FQDN, this has no effect.
	// Default to false.
	// +optional
	SetHostnameAsFQDN *bool `json:"setHostnameAsFQDN,omitempty" protobuf:"varint,35,opt,name=setHostnameAsFQDN"`
}
