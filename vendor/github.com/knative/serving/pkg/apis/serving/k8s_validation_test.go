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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	"github.com/knative/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestContainerValidation(t *testing.T) {
	bidir := corev1.MountPropagationBidirectional

	tests := []struct {
		name    string
		c       corev1.Container
		want    *apis.FieldError
		volumes sets.String
	}{{
		name: "empty container",
		c:    corev1.Container{},
		want: apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "valid container",
		c: corev1.Container{
			Image: "foo",
		},
		want: nil,
	}, {
		name: "invalid container image",
		c: corev1.Container{
			Image: "foo:bar:baz",
		},
		want: &apis.FieldError{
			Message: "Failed to parse image reference",
			Paths:   []string{"image"},
			Details: "image: \"foo:bar:baz\", error: could not parse reference",
		},
	}, {
		name: "has a name",
		c: corev1.Container{
			Name:  "foo",
			Image: "foo",
		},
		want: apis.ErrDisallowedFields("name"),
	}, {
		name: "has resources",
		c: corev1.Container{
			Image: "foo",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceName("memory"): resource.MustParse("250M"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceName("cpu"): resource.MustParse("25m"),
				},
			},
		},
		want: nil,
	}, {
		name: "has no container ports set",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{},
		},
		want: nil,
	}, {
		name: "has valid user port http1",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name:          "http1",
				ContainerPort: 8081,
			}},
		},
		want: nil,
	}, {
		name: "has valid user port h2c",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name:          "h2c",
				ContainerPort: 8081,
			}},
		},
		want: nil,
	}, {
		name: "has more than one ports with valid names",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name:          "h2c",
				ContainerPort: 8080,
			}, {
				Name:          "http1",
				ContainerPort: 8181,
			}},
		},
		want: &apis.FieldError{
			Message: "More than one container port is set",
			Paths:   []string{"ports"},
			Details: "Only a single port is allowed",
		},
	}, {
		name: "has container port value too large",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 65536,
			}},
		},
		want: apis.ErrOutOfBoundsValue(65536, 1, 65535, "ports.containerPort"),
	}, {
		name: "has an empty port set",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{}},
		},
		want: apis.ErrOutOfBoundsValue(0, 1, 65535, "ports.containerPort"),
	}, {
		name: "has more than one unnamed port",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8080,
			}, {
				ContainerPort: 8181,
			}},
		},
		want: &apis.FieldError{
			Message: "More than one container port is set",
			Paths:   []string{"ports"},
			Details: "Only a single port is allowed",
		},
	}, {
		name: "has tcp protocol",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: 8080,
			}},
		},
		want: nil,
	}, {
		name: "has invalid protocol",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Protocol:      "tdp",
				ContainerPort: 8080,
			}},
		},
		want: apis.ErrInvalidValue("tdp", "ports.protocol"),
	}, {
		name: "has host port",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				HostPort:      80,
				ContainerPort: 8080,
			}},
		},
		want: apis.ErrDisallowedFields("ports.hostPort"),
	}, {
		name: "has host ip",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				HostIP:        "127.0.0.1",
				ContainerPort: 8080,
			}},
		},
		want: apis.ErrDisallowedFields("ports.hostIP"),
	}, {
		name: "port conflicts with queue proxy admin",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8022,
			}},
		},
		want: apis.ErrInvalidValue(8022, "ports.containerPort"),
	}, {
		name: "port conflicts with queue proxy",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8012,
			}},
		},
		want: apis.ErrInvalidValue(8012, "ports.containerPort"),
	}, {
		name: "port conflicts with queue proxy metrics",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 9090,
			}},
		},
		want: apis.ErrInvalidValue(9090, "ports.containerPort"),
	}, {
		name: "has invalid port name",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name:          "foobar",
				ContainerPort: 8080,
			}},
		},
		want: &apis.FieldError{
			Message: fmt.Sprintf("Port name %v is not allowed", "foobar"),
			Paths:   []string{"ports"},
			Details: "Name must be empty, or one of: 'h2c', 'http1'",
		},
	}, {
		name: "has unknown volumeMounts",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				Name:             "the-name",
				SubPath:          "oops",
				MountPropagation: &bidir,
			}},
		},
		want: (&apis.FieldError{
			Message: "volumeMount has no matching volume",
			Paths:   []string{"name"},
		}).ViaFieldIndex("volumeMounts", 0).Also(
			apis.ErrMissingField("readOnly").ViaFieldIndex("volumeMounts", 0)).Also(
			apis.ErrMissingField("mountPath").ViaFieldIndex("volumeMounts", 0)).Also(
			apis.ErrDisallowedFields("subPath").ViaFieldIndex("volumeMounts", 0)).Also(
			apis.ErrDisallowedFields("mountPropagation").ViaFieldIndex("volumeMounts", 0)),
	}, {
		name: "missing known volumeMounts",
		c: corev1.Container{
			Image: "foo",
		},
		volumes: sets.NewString("the-name"),
		want: &apis.FieldError{
			Message: "volumes not mounted: [the-name]",
			Paths:   []string{"volumeMounts"},
		},
	}, {
		name: "has known volumeMounts",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "/mount/path",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
	}, {
		name: "has known volumeMounts, but at reserved path",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "//var//log//",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
		want: (&apis.FieldError{
			Message: `mountPath "/var/log" is a reserved path`,
			Paths:   []string{"mountPath"},
		}).ViaFieldIndex("volumeMounts", 0),
	}, {
		name: "has known volumeMounts, bad mountPath",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "not/absolute",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
		want:    apis.ErrInvalidValue("not/absolute", "volumeMounts[0].mountPath"),
	}, {
		name: "has lifecycle",
		c: corev1.Container{
			Image:     "foo",
			Lifecycle: &corev1.Lifecycle{},
		},
		want: apis.ErrDisallowedFields("lifecycle"),
	}, {
		name: "has known volumeMount twice",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "/mount/path",
				Name:      "the-name",
				ReadOnly:  true,
			}, {
				MountPath: "/another/mount/path",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
	}, {
		name: "valid with probes (no port)",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				},
			},
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{},
				},
			},
		},
		want: nil,
	}, {
		name: "invalid readiness http probe (has port)",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
						Port: intstr.FromInt(8080),
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("readinessProbe.httpGet.port"),
	}, {
		name: "disallowed security context field",
		c: corev1.Container{
			Image: "foo",
			SecurityContext: &corev1.SecurityContext{
				RunAsGroup: ptr.Int64(10),
			},
		},
		want: apis.ErrDisallowedFields("securityContext.runAsGroup"),
	}, {
		name: "too large uid",
		c: corev1.Container{
			Image: "foo",
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: ptr.Int64(math.MaxInt32 + 1),
			},
		},
		want: apis.ErrOutOfBoundsValue(int64(math.MaxInt32+1), 0, math.MaxInt32, "securityContext.runAsUser"),
	}, {
		name: "negative uid",
		c: corev1.Container{
			Image: "foo",
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: ptr.Int64(-10),
			},
		},
		want: apis.ErrOutOfBoundsValue(-10, 0, math.MaxInt32, "securityContext.runAsUser"),
	}, {
		name: "envFrom - None of",
		c: corev1.Container{
			Image:   "foo",
			EnvFrom: []corev1.EnvFromSource{{}},
		},
		want: apis.ErrMissingOneOf("envFrom.configMapRef", "envFrom.secretRef"),
	}, {
		name: "envFrom - Multiple",
		c: corev1.Container{
			Image: "foo",
			EnvFrom: []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "ConfigMapName",
					},
				},
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "SecretName",
					},
				},
			}},
		},
		want: apis.ErrMultipleOneOf("envFrom.configMapRef", "envFrom.secretRef"),
	}, {
		name: "envFrom - Secret",
		c: corev1.Container{
			Image: "foo",
			EnvFrom: []corev1.EnvFromSource{{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "SecretName",
					},
				},
			}},
		},
		want: nil,
	}, {
		name: "envFrom - ConfigMap",
		c: corev1.Container{
			Image: "foo",
			EnvFrom: []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "ConfigMapName",
					},
				},
			}},
		},
		want: nil,
	}, {
		name: "termination message policy",
		c: corev1.Container{
			Image:                    "foo",
			TerminationMessagePolicy: corev1.TerminationMessagePolicy("Not a Policy"),
		},
		want: apis.ErrInvalidValue(corev1.TerminationMessagePolicy("Not a Policy"), "terminationMessagePolicy"),
	}, {
		name: "disallowed envvarsource",
		c: corev1.Container{
			Image: "foo",
			Env: []corev1.EnvVar{{
				Name: "Foo",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "/v1",
					},
				},
			}},
		},
		want: apis.ErrDisallowedFields("env[0].valueFrom.fieldRef"),
	}, {
		name: "invalid liveness tcp probe (has port)",
		c: corev1.Container{
			Image: "foo",
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromString("http"),
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("livenessProbe.tcpSocket.port"),
	}, {
		name: "disallowed container fields",
		c: corev1.Container{
			Image:     "foo",
			Name:      "fail",
			Stdin:     true,
			StdinOnce: true,
			TTY:       true,
			Lifecycle: &corev1.Lifecycle{},
			VolumeDevices: []corev1.VolumeDevice{{
				Name:       "disallowed",
				DevicePath: "/",
			}},
		},
		want: apis.ErrDisallowedFields("lifecycle").Also(
			apis.ErrDisallowedFields("name")).Also(
			apis.ErrDisallowedFields("stdin")).Also(
			apis.ErrDisallowedFields("stdinOnce")).Also(
			apis.ErrDisallowedFields("tty")).Also(
			apis.ErrDisallowedFields("volumeDevices")),
	}, {
		name: "invalid liveness tcp probe (has port)",
		c: corev1.Container{
			Image: "foo",
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromString("http"),
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("livenessProbe.tcpSocket.port"),
	}, {
		name: "has numerous problems",
		c: corev1.Container{
			Name:      "foo",
			Lifecycle: &corev1.Lifecycle{},
		},
		want: apis.ErrDisallowedFields("name", "lifecycle").Also(
			&apis.FieldError{
				Message: "Failed to parse image reference",
				Paths:   []string{"image"},
				Details: "image: \"\", error: could not parse reference",
			},
		),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ValidateContainer(test.c, test.volumes)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("ValidateContainer (-want, +got) = %v", diff)
			}
		})
	}
}

func TestVolumeValidation(t *testing.T) {
	tests := []struct {
		name string
		v    corev1.Volume
		want *apis.FieldError
	}{{
		name: "just name",
		v: corev1.Volume{
			Name: "foo",
		},
		want: apis.ErrMissingOneOf("secret", "configMap"),
	}, {
		name: "secret volume",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
				},
			},
		},
	}, {
		name: "configMap volume",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "foo"},
				},
			},
		},
	}, {
		name: "emptyDir volume",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		want: apis.ErrMissingOneOf("secret", "configMap").Also(apis.ErrDisallowedFields("emptyDir")),
	}, {
		name: "no volume source",
		v: corev1.Volume{
			Name: "foo",
		},
		want: apis.ErrMissingOneOf("secret", "configMap"),
	}, {
		name: "no name",
		v: corev1.Volume{
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
				},
			},
		},
		want: apis.ErrMissingField("name"),
	}, {
		name: "bad name",
		v: corev1.Volume{
			Name: "@@@",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
				},
			},
		},
		want: apis.ErrInvalidValue("@@@", "name"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := validateVolume(test.v)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateVolume (-want, +got) = %v", diff)
			}
		})
	}
}
