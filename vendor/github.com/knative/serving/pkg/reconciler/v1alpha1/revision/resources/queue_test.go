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
	"strconv"
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
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/config"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMakeQueueContainer(t *testing.T) {
	tests := []struct {
		name     string
		rev      *v1alpha1.Revision
		lc       *logging.Config
		oc       *config.Observability
		ac       *autoscaler.Config
		cc       *config.Controller
		userport *corev1.ContainerPort
		want     *corev1.Container
	}{{
		name: "no owner no autoscaler single",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 1,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
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
				Value: "1",
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
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}, {
		name: "no owner no autoscaler single",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 1,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{
			QueueSidecarImage: "alpine",
		},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Image: "alpine",
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
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
				Value: "1",
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
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}, {
		name: "service name in labels",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
				Labels: map[string]string{
					serving.ServiceLabelKey: "svc",
				},
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 1,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{
			QueueSidecarImage: "alpine",
		},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Image: "alpine",
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
				Value: "1",
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
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		}}, {
		name: "config owner as env var, multi-concurrency",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "baz",
				Name:      "blah",
				UID:       "1234",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         v1alpha1.SchemeGroupVersion.String(),
					Kind:               "Configuration",
					Name:               "the-parent-config-name",
					Controller:         ptr.Bool(true),
					BlockOwnerDeletion: ptr.Bool(true),
				}},
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 0,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "baz", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
			}, {
				Name:  "SERVING_CONFIGURATION",
				Value: "the-parent-config-name",
			}, {
				Name:  "SERVING_REVISION",
				Value: "blah", // matches name
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
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}, {
		name: "logging configuration as env var",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "log",
				Name:      "this",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 0,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{
			LoggingConfig: "The logging configuration goes here",
			LoggingLevel: map[string]zapcore.Level{
				"queueproxy": zapcore.ErrorLevel,
			},
		},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "log", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
			}, {
				Name: "SERVING_CONFIGURATION",
				// No Configuration owner.
			}, {
				Name:  "SERVING_REVISION",
				Value: "this", // matches name
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
				Name:  "SERVING_LOGGING_CONFIG",
				Value: "The logging configuration goes here", // from logging config
			}, {
				Name:  "SERVING_LOGGING_LEVEL",
				Value: "error", // from logging config
			}, {
				Name:  "SERVING_REQUEST_LOG_TEMPLATE",
				Value: "",
			}, {
				Name:  "SERVING_REQUEST_METRICS_BACKEND",
				Value: "",
			}, {
				Name:  "USER_PORT",
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}, {
		name: "container concurrency 10",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 10,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
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
				Value: "10",
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
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}, {
		name: "request log as env var",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 0,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{RequestLogTemplate: "test template"},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
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
				Value: "test template",
			}, {
				Name:  "SERVING_REQUEST_METRICS_BACKEND",
				Value: "",
			}, {
				Name:  "USER_PORT",
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}, {
		name: "request metrics backend as env var",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "foo",
				Name:      "bar",
				UID:       "1234",
			},
			Spec: v1alpha1.RevisionSpec{
				RevisionSpec: v1beta1.RevisionSpec{
					ContainerConcurrency: 0,
					TimeoutSeconds:       ptr.Int64(45),
				},
			},
		},
		lc: &logging.Config{},
		oc: &config.Observability{
			RequestMetricsBackend: "prometheus",
		},
		ac: &autoscaler.Config{},
		cc: &config.Controller{},
		userport: &corev1.ContainerPort{
			Name:          userPortEnvName,
			ContainerPort: v1alpha1.DefaultUserPort,
		},
		want: &corev1.Container{
			// These are effectively constant
			Name:           QueueContainerName,
			Resources:      queueResources,
			Ports:          queuePorts,
			ReadinessProbe: queueReadinessProbe,
			// These changed based on the Revision and configs passed in.
			Env: []corev1.EnvVar{{
				Name:  "SERVING_NAMESPACE",
				Value: "foo", // matches namespace
			}, {
				Name:  "SERVING_SERVICE",
				Value: "", // not set in the labels
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
				Value: "prometheus",
			}, {
				Name:  "USER_PORT",
				Value: strconv.Itoa(v1alpha1.DefaultUserPort),
			}, {
				Name:  "SYSTEM_NAMESPACE",
				Value: system.Namespace(),
			}},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := makeQueueContainer(test.rev, test.lc, test.oc, test.ac, test.cc)
			if diff := cmp.Diff(test.want, got, cmpopts.IgnoreUnexported(resource.Quantity{})); diff != "" {
				t.Errorf("makeQueueContainer (-want, +got) = %v", diff)
			}
		})
	}
}
