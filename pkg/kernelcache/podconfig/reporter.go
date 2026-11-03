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
package podconfig

import (
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/kernelcache/reporter"
)

const (
	reporterVolume = "mcv-reporter-access"
	reporterPath   = "/var/run/secrets/mcv-reporter"
)

// ApplyCaptureReporter mounts access for updating capture status.
func ApplyCaptureReporter(pod *corev1.PodSpec, container *corev1.Container, secretName string) error {
	if secretName == "" {
		return errors.New("capture reporter access Secret name is required")
	}
	volume := corev1.Volume{
		Name: reporterVolume,
		VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Optional:             ptr.To(false),
				Items:                []corev1.KeyToPath{{Key: reporter.AccessKey, Path: reporter.AccessKey}},
			},
		}, {
			ConfigMap: &corev1.ConfigMapProjection{
				LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"},
				Items:                []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}},
			},
		}}}},
	}
	for _, existing := range pod.Volumes {
		if existing.Name == reporterVolume {
			if !reflect.DeepEqual(existing, volume) {
				return fmt.Errorf("conflicting volume %q", reporterVolume)
			}
			container.Env = append(container.Env,
				corev1.EnvVar{Name: "MCV_REPORTER_ACCESS_FILE", Value: reporterPath + "/" + reporter.AccessKey},
				corev1.EnvVar{Name: "MCV_KUBERNETES_CA_FILE", Value: reporterPath + "/ca.crt"})
			return nil
		}
	}
	pod.Volumes = append(pod.Volumes, volume)
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: reporterVolume, MountPath: reporterPath, ReadOnly: true})
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "MCV_REPORTER_ACCESS_FILE", Value: reporterPath + "/" + reporter.AccessKey},
		corev1.EnvVar{Name: "MCV_KUBERNETES_CA_FILE", Value: reporterPath + "/ca.crt"})
	return nil
}
