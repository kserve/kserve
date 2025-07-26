/*
Copyright 2025 The KServe Authors.

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

package llmisvc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc"

	kservetesting "github.com/kserve/kserve/pkg/testing"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestPresetFiles(t *testing.T) {
	presetsDir := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvc")

	llmSvc := llmisvc.LLMInferenceServiceSample()
	kserveSystemConfig := llmisvc.Config{
		SystemNamespace:         "kserve",
		IngressGatewayName:      "kserve-ingress-gateway",
		IngressGatewayNamespace: "kserve",
	}

	tt := map[string]struct {
		expected *v1alpha1.LLMInferenceServiceConfig
	}{
		"config-llm-decode-worker-data-parallel.yaml": {
			expected: &v1alpha1.LLMInferenceServiceConfig{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "serving.kserve.io/v1alpha1",
					Kind:       "LLMInferenceServiceConfig",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "kserve-config-llm-decode-worker-data-parallel",
				},
				Spec: v1alpha1.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha1.WorkloadSpec{
						Worker: &corev1.PodSpec{
							Volumes: []corev1.Volume{
								{
									Name: "home",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "dshm",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{
											Medium:    corev1.StorageMediumMemory,
											SizeLimit: ptr.To(resource.MustParse("1Gi")),
										},
									},
								},
								{
									Name: "model-cache",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name:         "tls-certs",
									VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "test-llm-preset-kserve-self-signed-certs"}},
								},
							},
							TerminationGracePeriodSeconds: ptr.To(int64(30)),
							InitContainers: []corev1.Container{
								{
									Name:  "llm-d-routing-sidecar",
									Image: "ghcr.io/llm-d/llm-d-routing-sidecar:v0.2.0",
									Args: []string{
										"--port=8000",
										"--vllm-port=8001",
										"--secure-proxy=true",
										"--cert-path=/etc/ssl/certs",
										"--decoder-use-tls=true",
										"--decoder-tls-insecure-skip-verify=true",
										"--prefiller-use-tls=true",
										"--prefiller-tls-insecure-skip-verify=true",
										"--enable-ssrf-protection=true",
									},
									Env: []corev1.EnvVar{
										{
											Name: "INFERENCE_POOL_NAMESPACE",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.namespace",
												},
											},
										},
									},
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 8000,
											Protocol:      corev1.ProtocolTCP,
										},
									},
									RestartPolicy:            ptr.To(corev1.ContainerRestartPolicyAlways),
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: "FallbackToLogsOnError",
									ImagePullPolicy:          "IfNotPresent",
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "tls-certs",
											ReadOnly:  true,
											MountPath: "/etc/ssl/certs",
										},
									},
									ReadinessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path:   "/health",
												Port:   intstr.FromInt32(8000),
												Scheme: corev1.URISchemeHTTPS,
											},
										},
										InitialDelaySeconds: 10,
										TimeoutSeconds:      5,
										PeriodSeconds:       10,
										FailureThreshold:    10,
									},
									LivenessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path:   "/health",
												Port:   intstr.FromInt32(8000),
												Scheme: corev1.URISchemeHTTPS,
											},
										},
										InitialDelaySeconds: 10,
										TimeoutSeconds:      10,
										PeriodSeconds:       10,
										FailureThreshold:    3,
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:    "main",
									Image:   "ghcr.io/llm-d/llm-d:v0.2.0",
									Command: []string{"/bin/sh", "-c"},
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 8001,
											Protocol:      corev1.ProtocolTCP,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "home",
											MountPath: "/home",
										},
										{
											Name:      "dshm",
											MountPath: "/dev/shm",
										},
										{
											Name:      "model-cache",
											MountPath: "/models",
										},
										{
											Name:      "tls-certs",
											ReadOnly:  true,
											MountPath: "/etc/ssl/certs",
										},
									},
									LivenessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path:   "/health",
												Port:   intstr.FromInt32(8001),
												Scheme: corev1.URISchemeHTTPS,
											},
										},
										InitialDelaySeconds: 120,
										PeriodSeconds:       10,
										TimeoutSeconds:      10,
										FailureThreshold:    3,
									},
									ReadinessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path:   "/health",
												Port:   intstr.FromInt32(8001),
												Scheme: corev1.URISchemeHTTPS,
											},
										},
										InitialDelaySeconds: 10,
										PeriodSeconds:       10,
										TimeoutSeconds:      5,
										FailureThreshold:    60,
									},
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										Capabilities: &corev1.Capabilities{
											Add: []corev1.Capability{
												"IPC_LOCK",
												"SYS_RAWIO",
											},
										},
									},
									Env: []corev1.EnvVar{
										{
											Name:  "HOME",
											Value: "/home",
										},
										{
											Name:  "VLLM_LOGGING_LEVEL",
											Value: "INFO",
										},
										{
											Name:  "HF_HUB_CACHE",
											Value: "/models",
										},
									},
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: "FallbackToLogsOnError",
									ImagePullPolicy:          "IfNotPresent",
									Stdin:                    true,
									TTY:                      true,
									Args: []string{`
START_RANK=$(( ${LWS_WORKER_INDEX:-0} * 2 ))
if [ "${LWS_WORKER_INDEX:-0}" -eq 0 ]; then
  #################
  # Leader-only launch
  #################
  vllm serve \
    llama \
    --port 8001 \
    --api-server-count 4 \
    --disable-log-requests \
--enable-expert-parallel \
--tensor-parallel-size 1 \
    --data-parallel-size 4 \
    --data-parallel-size-local 2 \
    --data-parallel-address $(LWS_LEADER_ADDRESS) \
    --data-parallel-rpc-port 5555 \
    --data-parallel-start-rank $START_RANK \
    --trust-remote-code \
    --enable-ssl-refresh \
    --ssl-certfile \
    /etc/ssl/certs/tls.crt \
    --ssl-keyfile \
    /etc/ssl/certs/tls.key
else
  #################
  # Worker-only launch
  #################
  vllm serve \
    llama \
    --port 8001 \
    --disable-log-requests \
--enable-expert-parallel \
--tensor-parallel-size 1 \
    --data-parallel-size 4 \
    --data-parallel-size-local 2 \
    --data-parallel-address $(LWS_LEADER_ADDRESS) \
    --data-parallel-rpc-port 5555 \
    --data-parallel-start-rank $START_RANK \
    --trust-remote-code \
    --headless \
    --enable-ssl-refresh \
    --ssl-certfile \
    /etc/ssl/certs/tls.crt \
    --ssl-keyfile \
    /etc/ssl/certs/tls.key
fi`},
								},
							},
						},
					},
				},
			},
		},
	}

	remaining := llmisvc.WellKnownDefaultConfigs.Clone()

	_ = filepath.Walk(presetsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Errorf("Failed to walk %s: %v", path, err)
			return err
		}

		filename := info.Name()
		if info.IsDir() || !strings.HasSuffix(filename, ".yaml") || !strings.HasPrefix(filename, "config-") {
			return nil
		}

		t.Run(filename, func(t *testing.T) {
			filePath := filepath.Join(presetsDir, filename)
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("Failed to read file %s: %v", filePath, err)
				return
			}

			config := loadConfig(t, data, filePath)

			name := config.ObjectMeta.Name
			if !llmisvc.WellKnownDefaultConfigs.Has(name) {
				t.Fatalf("Expected %s to exist in WellKnownDefaultConfigs %#v", name, llmisvc.WellKnownDefaultConfigs)
			}
			// Remove from the tracked set
			remaining = remaining.Delete(name)

			out, err := llmisvc.ReplaceVariables(llmSvc, config, &kserveSystemConfig)
			if err != nil {
				t.Errorf("ReplaceVariables() failed for %s: %v", filename, err)
			}

			// Verify the actual Spec rendered if provided for the found file.
			if tc, exist := tt[filename]; exist {
				if !equality.Semantic.DeepEqual(tc.expected, out) {
					diff := cmp.Diff(tc.expected, out)
					t.Errorf("ReplaceVariables() returned unexpected diff (-want +got):\n%s", diff)
				}
			}
		})

		return nil
	})

	if remaining.Len() > 0 {
		t.Errorf("Found %d remaining well-known-configs that are missing as manifest files: %#v", remaining.Len(), remaining)
	}
}

func loadConfig(t *testing.T, data []byte, filePath string) *v1alpha1.LLMInferenceServiceConfig {
	config := &v1alpha1.LLMInferenceServiceConfig{}
	if err := yaml.Unmarshal(data, config); err != nil {
		t.Errorf("Failed to unmarshal YAML from %s: %v", filePath, err)
		return nil
	}
	if err := yaml.Unmarshal(data, config); err != nil {
		t.Errorf("Failed to unmarshal YAML from %s: %v", filePath, err)
		return nil
	}

	expectedGroupVersion := v1alpha1.LLMInferenceServiceConfigGVK.GroupVersion().String()
	if config.APIVersion != expectedGroupVersion {
		t.Errorf("Expected APIVersion to be '%s', got '%s'", expectedGroupVersion, config.APIVersion)
	}

	expectedKind := v1alpha1.LLMInferenceServiceConfigGVK.Kind
	if config.Kind != expectedKind {
		t.Errorf("Expected Kind to be '%s', got %s", expectedKind, config.Kind)
	}

	if config.ObjectMeta.Name == "" {
		t.Error("Expected ObjectMeta.Name to be set")
	}

	return config
}
