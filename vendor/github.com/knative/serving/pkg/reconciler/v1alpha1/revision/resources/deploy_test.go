/*
Copyright 2018 The Knative Authors

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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/ptr"
	"github.com/knative/pkg/system"
	_ "github.com/knative/pkg/system/testing"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	one int32 = 1

	defaultUserContainer = &corev1.Container{
		Name:                     UserContainerName,
		Image:                    "busybox",
		Ports:                    buildContainerPorts(v1alpha1.DefaultUserPort),
		VolumeMounts:             []corev1.VolumeMount{varLogVolumeMount},
		Lifecycle:                userLifecycle,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		Stdin:                    false,
		TTY:                      false,
		Env: []corev1.EnvVar{{
			Name:  "PORT",
			Value: "8080",
		}, {
			Name:  "K_REVISION",
			Value: "bar",
		}, {
			Name:  "K_CONFIGURATION",
			Value: "cfg",
		}, {
			Name:  "K_SERVICE",
			Value: "svc",
		}},
	}

	defaultQueueContainer = &corev1.Container{
		Name:           QueueContainerName,
		Resources:      queueResources,
		Ports:          queuePorts,
		ReadinessProbe: queueReadinessProbe,
		Env: []corev1.EnvVar{{
			Name:  "SERVING_NAMESPACE",
			Value: "foo", // matches namespace
		}, {
			Name:  "SERVING_SERVICE",
			Value: "svc", // matches service name
		}, {
			Name: "SERVING_CONFIGURATION",
			// No OwnerReference
		}, {
			Name:  "SERVING_REVISION",
			Value: "bar", // matches name
		}, {
			Name:  "SERVING_AUTOSCALER",
			Value: "autoscaler", // no autoscaler configured.
		}, {
			Name:  "SERVING_AUTOSCALER_PORT",
			Value: "8080",
		}, {
			Name:  "CONTAINER_CONCURRENCY",
			Value: "0",
		}, {
			Name:  "REVISION_TIMEOUT_SECONDS",
			Value: "45",
		}, {
			Name: "SERVING_POD",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		}, {
			Name: "SERVING_POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
			},
		}, {
			Name: "SERVING_LOGGING_CONFIG",
			// No logging configuration
		}, {
			Name: "SERVING_LOGGING_LEVEL",
			// No logging level
		}, {
			Name:  "SERVING_REQUEST_LOG_TEMPLATE",
			Value: "",
		}, {
			Name:  "SERVING_REQUEST_METRICS_BACKEND",
			Value: "",
		}, {
			Name:  "USER_PORT",
			Value: "8080",
		}, {
			Name:  "SYSTEM_NAMESPACE",
			Value: system.Namespace(),
		}},
	}

	defaultFluentdContainer = &corev1.Container{
		Name:      FluentdContainerName,
		Image:     "indiana:jones",
		Resources: fluentdResources,
		Env: []corev1.EnvVar{{
			Name:  "FLUENTD_ARGS",
			Value: "--no-supervisor -q",
		}, {
			Name:  "SERVING_CONTAINER_NAME",
			Value: UserContainerName,
		}, {
			Name: "SERVING_CONFIGURATION",
			// No owner reference
		}, {
			Name:  "SERVING_REVISION",
			Value: "bar",
		}, {
			Name:  "SERVING_NAMESPACE",
			Value: "foo",
		}, {
			Name: "SERVING_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		}},
		VolumeMounts: fluentdVolumeMounts,
	}

	defaultPodSpec = &corev1.PodSpec{
		Volumes:                       []corev1.Volume{varLogVolume},
		TerminationGracePeriodSeconds: refInt64(45),
	}

	defaultDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
			Name:      "bar-deployment",
			Labels: map[string]string{
				serving.RevisionLabelKey: "bar",
				serving.RevisionUID:      "1234",
				AppLabelKey:              "bar",
			},
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         v1alpha1.SchemeGroupVersion.String(),
				Kind:               "Revision",
				Name:               "bar",
				UID:                "1234",
				Controller:         ptr.Bool(true),
				BlockOwnerDeletion: ptr.Bool(true),
			}},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					serving.RevisionUID: "1234",
				},
			},
			ProgressDeadlineSeconds: &ProgressDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						serving.RevisionLabelKey: "bar",
						serving.RevisionUID:      "1234",
						AppLabelKey:              "bar",
					},
					Annotations: map[string]string{
						sidecarIstioInjectAnnotation: "true",
					},
				},
				// Spec: filled in by makePodSpec
			},
		},
	}

	defaultRevision = &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
			Name:      "bar",
			UID:       "1234",
			Labels: map[string]string{
				serving.ConfigurationLabelKey: "cfg",
				serving.ServiceLabelKey:       "svc",
			},
		},
		Spec: v1alpha1.RevisionSpec{
			Container: &corev1.Container{
				Image: "busybox",
			},
			RevisionSpec: v1beta1.RevisionSpec{
				TimeoutSeconds: ptr.Int64(45),
			},
		},
	}
)

func refInt64(num int64) *int64 {
	return &num
}

type containerOption func(*corev1.Container)
type podSpecOption func(*corev1.PodSpec)
type deploymentOption func(*appsv1.Deployment)
type revisionOption func(*v1alpha1.Revision)

func container(container *corev1.Container, opts ...containerOption) corev1.Container {
	for _, option := range opts {
		option(container)
	}
	return *container
}

func userContainer(opts ...containerOption) corev1.Container {
	return container(defaultUserContainer.DeepCopy(), opts...)
}

func queueContainer(opts ...containerOption) corev1.Container {
	return container(defaultQueueContainer.DeepCopy(), opts...)
}

func fluentdContainer(opts ...containerOption) corev1.Container {
	return container(defaultFluentdContainer.DeepCopy(), opts...)
}

func withEnvVar(name, value string) containerOption {
	return func(container *corev1.Container) {
		for i, envVar := range container.Env {
			if envVar.Name == name {
				container.Env[i].Value = value
				return
			}
		}

		container.Env = append(container.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}
}

func withReadinessProbe(handler corev1.Handler) containerOption {
	return func(container *corev1.Container) {
		container.ReadinessProbe = &corev1.Probe{Handler: handler}
	}
}

func withHTTPReadinessProbe(port int) containerOption {
	return withReadinessProbe(corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Port: intstr.FromInt(port),
			Path: "/",
		},
	})
}

func withExecReadinessProbe(command []string) containerOption {
	return withReadinessProbe(corev1.Handler{
		Exec: &corev1.ExecAction{
			Command: command,
		},
	})
}

func withLivenessProbe(handler corev1.Handler) containerOption {
	return func(container *corev1.Container) {
		container.LivenessProbe = &corev1.Probe{Handler: handler}
	}
}

func withPrependedVolumeMounts(volumeMounts ...corev1.VolumeMount) containerOption {
	return func(c *corev1.Container) {
		c.VolumeMounts = append(volumeMounts, c.VolumeMounts...)
	}
}

func podSpec(containers []corev1.Container, opts ...podSpecOption) *corev1.PodSpec {
	podSpec := defaultPodSpec.DeepCopy()
	podSpec.Containers = containers

	for _, option := range opts {
		option(podSpec)
	}

	return podSpec
}

func withAppendedVolumes(volumes ...corev1.Volume) podSpecOption {
	return func(ps *corev1.PodSpec) {
		ps.Volumes = append(ps.Volumes, volumes...)
	}
}

func deployment(opts ...deploymentOption) *appsv1.Deployment {
	deployment := defaultDeployment.DeepCopy()
	for _, option := range opts {
		option(deployment)
	}
	return deployment
}

func revision(opts ...revisionOption) *v1alpha1.Revision {
	revision := defaultRevision.DeepCopy()
	for _, option := range opts {
		option(revision)
	}
	return revision
}

func withContainerConcurrency(cc v1beta1.RevisionContainerConcurrencyType) revisionOption {
	return func(revision *v1alpha1.Revision) {
		revision.Spec.ContainerConcurrency = cc
	}
}

func withoutLabels(revision *v1alpha1.Revision) {
	revision.ObjectMeta.Labels = map[string]string{}
}

func withOwnerReference(name string) revisionOption {
	return func(revision *v1alpha1.Revision) {
		revision.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion:         v1alpha1.SchemeGroupVersion.String(),
			Kind:               "Configuration",
			Name:               name,
			Controller:         ptr.Bool(true),
			BlockOwnerDeletion: ptr.Bool(true),
		}}
	}
}

func TestMakePodSpec(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		lc   *logging.Config
		oc   *config.Observability
		ac   *autoscaler.Config
		cc   *config.Controller
		want *corev1.PodSpec
	}{{
		name: "user-defined user port, queue proxy have PORT env",
		rev: revision(
			withContainerConcurrency(1),
			func(revision *v1alpha1.Revision) {
				revision.Spec.GetContainer().Ports = []corev1.ContainerPort{{
					ContainerPort: 8888,
				}}
			},
		),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				func(container *corev1.Container) {
					container.Ports[0].ContainerPort = 8888
				},
				withEnvVar("PORT", "8888"),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "1"),
				withEnvVar("USER_PORT", "8888"),
			),
		}),
	}, {
		name: "volumes passed through",
		rev: revision(
			withContainerConcurrency(1),
			func(revision *v1alpha1.Revision) {
				revision.Spec.GetContainer().Ports = []corev1.ContainerPort{{
					ContainerPort: 8888,
				}}
				revision.Spec.GetContainer().VolumeMounts = []corev1.VolumeMount{{
					Name:      "asdf",
					MountPath: "/asdf",
				}}
				revision.Spec.Volumes = []corev1.Volume{{
					Name: "asdf",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "asdf",
						},
					},
				}}
			},
		),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				func(container *corev1.Container) {
					container.Ports[0].ContainerPort = 8888
				},
				withEnvVar("PORT", "8888"),
				withPrependedVolumeMounts(corev1.VolumeMount{
					Name:      "asdf",
					MountPath: "/asdf",
				}),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "1"),
				withEnvVar("USER_PORT", "8888"),
			),
		}, withAppendedVolumes(corev1.Volume{
			Name: "asdf",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "asdf",
				},
			},
		})),
	}, {
		name: "simple concurrency=single no owner",
		rev:  revision(withContainerConcurrency(1)),
		lc:   &logging.Config{},
		oc:   &config.Observability{},
		ac:   &autoscaler.Config{},
		cc:   &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "1"),
			),
		}),
	}, {
		name: "simple concurrency=single no owner digest resolved",
		rev: revision(
			withContainerConcurrency(1),
			func(revision *v1alpha1.Revision) {
				revision.Status = v1alpha1.RevisionStatus{
					ImageDigest: "busybox@sha256:deadbeef",
				}
			},
		),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(func(container *corev1.Container) {
				container.Image = "busybox@sha256:deadbeef"
			}),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "1"),
			),
		}),
	}, {
		name: "simple concurrency=single with owner",
		rev: revision(
			withContainerConcurrency(1),
			withOwnerReference("parent-config"),
		),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(),
			queueContainer(
				withEnvVar("SERVING_CONFIGURATION", "parent-config"),
				withEnvVar("CONTAINER_CONCURRENCY", "1"),
			),
		}),
	}, {
		name: "simple concurrency=multi http readiness probe",
		rev: revision(func(revision *v1alpha1.Revision) {
			container(revision.Spec.GetContainer(),
				withHTTPReadinessProbe(v1alpha1.DefaultUserPort),
			)
		}),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				withHTTPReadinessProbe(v1alpha1.RequestQueuePort),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "0"),
			),
		}),
	}, {
		name: "concurrency=multi, readinessprobe=shell",
		rev: revision(func(revision *v1alpha1.Revision) {
			container(revision.Spec.GetContainer(),
				withExecReadinessProbe(
					[]string{"echo", "hello"},
				),
			)
		}),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				withExecReadinessProbe(
					[]string{"echo", "hello"},
				),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "0"),
			),
		}),
	}, {
		name: "concurrency=multi, readinessprobe=http",
		rev: revision(func(revision *v1alpha1.Revision) {
			container(revision.Spec.GetContainer(),
				withReadinessProbe(corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				}),
			)
		}),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				withHTTPReadinessProbe(v1alpha1.RequestQueuePort),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "0"),
			),
		}),
	}, {
		name: "concurrency=multi, livenessprobe=tcp",
		rev: revision(func(revision *v1alpha1.Revision) {
			container(revision.Spec.GetContainer(),
				withLivenessProbe(corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{},
				}),
			)
		}),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				withLivenessProbe(corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(v1alpha1.DefaultUserPort),
					},
				}),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "0"),
			),
		}),
	}, {
		name: "with /var/log collection",
		rev:  revision(withContainerConcurrency(1)),
		lc:   &logging.Config{},
		oc: &config.Observability{
			EnableVarLogCollection: true,
			FluentdSidecarImage:    "indiana:jones",
		},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec(
			[]corev1.Container{
				userContainer(),
				queueContainer(
					withEnvVar("CONTAINER_CONCURRENCY", "1"),
				),
				fluentdContainer(),
			},
			func(podSpec *corev1.PodSpec) {
				podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
					Name: fluentdConfigMapVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "bar-fluentd",
							},
						},
					},
				})
			},
		),
	}, {
		name: "complex pod spec",
		rev: revision(
			withContainerConcurrency(1),
			func(revision *v1alpha1.Revision) {
				revision.ObjectMeta.Labels = map[string]string{}
				revision.Spec.GetContainer().Command = []string{"/bin/bash"}
				revision.Spec.GetContainer().Args = []string{"-c", "echo Hello world"}
				container(revision.Spec.GetContainer(),
					withEnvVar("FOO", "bar"),
					withEnvVar("BAZ", "blah"),
				)
				revision.Spec.GetContainer().Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("666Mi"),
						corev1.ResourceCPU:    resource.MustParse("666m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("888Mi"),
						corev1.ResourceCPU:    resource.MustParse("888m"),
					},
				}
				revision.Spec.GetContainer().TerminationMessagePolicy = corev1.TerminationMessageReadFile
			},
		),
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: podSpec([]corev1.Container{
			userContainer(
				func(container *corev1.Container) {
					container.Command = []string{"/bin/bash"}
					container.Args = []string{"-c", "echo Hello world"}
					container.Env = append([]corev1.EnvVar{{
						Name:  "FOO",
						Value: "bar",
					}, {
						Name:  "BAZ",
						Value: "blah",
					}}, container.Env...)
					container.Resources = corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("666Mi"),
							corev1.ResourceCPU:    resource.MustParse("666m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("888Mi"),
							corev1.ResourceCPU:    resource.MustParse("888m"),
						},
					}
					container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
				},
				withEnvVar("K_CONFIGURATION", ""),
				withEnvVar("K_SERVICE", ""),
			),
			queueContainer(
				withEnvVar("CONTAINER_CONCURRENCY", "1"),
				withEnvVar("SERVING_SERVICE", ""),
			),
		}),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quantityComparer := cmp.Comparer(func(x, y resource.Quantity) bool {
				return x.Cmp(y) == 0
			})

			got := makePodSpec(test.rev, test.lc, test.oc, test.ac, test.cc)
			if diff := cmp.Diff(test.want, got, quantityComparer); diff != "" {
				t.Errorf("makePodSpec (-want, +got) = %v", diff)
			}
		})

		t.Run(test.name+"(podspec)", func(t *testing.T) {
			quantityComparer := cmp.Comparer(func(x, y resource.Quantity) bool {
				return x.Cmp(y) == 0
			})

			// Same test, but via podspec.
			test.rev.Spec.Containers = []corev1.Container{
				*test.rev.Spec.Container,
			}
			test.rev.Spec.Container = nil

			got := makePodSpec(test.rev, test.lc, test.oc, test.ac, test.cc)
			if diff := cmp.Diff(test.want, got, quantityComparer); diff != "" {
				t.Errorf("makePodSpec (-want, +got) = %v", diff)
			}
		})
	}
}

func TestMakeDeployment(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		lc   *logging.Config
		nc   *network.Config
		oc   *config.Observability
		ac   *autoscaler.Config
		cc   *config.Controller
		want *appsv1.Deployment
	}{{
		name: "simple concurrency=single no owner",
		rev: revision(
			withoutLabels,
			withContainerConcurrency(1),
		),
		lc:   &logging.Config{},
		nc:   &network.Config{},
		oc:   &config.Observability{},
		ac:   &autoscaler.Config{},
		cc:   &config.Controller{},
		want: deployment(),
	}, {
		name: "simple concurrency=multi with owner",
		rev: revision(
			withoutLabels,
			withOwnerReference("parent-config"),
		),
		lc:   &logging.Config{},
		nc:   &network.Config{},
		oc:   &config.Observability{},
		ac:   &autoscaler.Config{},
		cc:   &config.Controller{},
		want: deployment(),
	}, {
		name: "simple concurrency=multi with outbound IP range configured",
		rev:  revision(withoutLabels),
		lc:   &logging.Config{},
		nc: &network.Config{
			IstioOutboundIPRanges: "*",
		},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: deployment(func(deploy *appsv1.Deployment) {
			deploy.Spec.Template.ObjectMeta.Annotations["traffic.sidecar.istio.io/includeOutboundIPRanges"] = "*"
		}),
	}, {
		name: "simple concurrency=multi with outbound IP range override",
		rev: revision(
			withoutLabels,
			func(revision *v1alpha1.Revision) {
				revision.ObjectMeta.Annotations = map[string]string{
					IstioOutboundIPRangeAnnotation: "10.4.0.0/14,10.7.240.0/20",
				}
			},
		),
		lc: &logging.Config{},
		nc: &network.Config{
			IstioOutboundIPRanges: "*",
		},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		want: deployment(func(deploy *appsv1.Deployment) {
			deploy.ObjectMeta.Annotations[IstioOutboundIPRangeAnnotation] = "10.4.0.0/14,10.7.240.0/20"
			deploy.Spec.Template.ObjectMeta.Annotations["traffic.sidecar.istio.io/includeOutboundIPRanges"] = "10.4.0.0/14,10.7.240.0/20"
		}),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Tested above so that we can rely on it here for brevity.
			test.want.Spec.Template.Spec = *makePodSpec(test.rev, test.lc, test.oc, test.ac, test.cc)
			got := MakeDeployment(test.rev, test.lc, test.nc, test.oc, test.ac, test.cc)
			if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(resource.Quantity{})); diff != "" {
				t.Errorf("MakeDeployment (-want, +got) = %v", diff)
			}
		})
	}
}
