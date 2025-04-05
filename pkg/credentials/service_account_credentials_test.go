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

package credentials

import (
	"testing"

	"github.com/onsi/gomega/types"

	"github.com/kserve/kserve/pkg/credentials/azure"
	"github.com/kserve/kserve/pkg/credentials/gcs"
	"github.com/kserve/kserve/pkg/credentials/hdfs"
	"github.com/kserve/kserve/pkg/credentials/s3"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

var configMap = &corev1.ConfigMap{
	Data: map[string]string{
		"credentials": `{
            "storageSecretNameAnnotation": "serving.kserve.io/storageSecretName",
            "storageSpecSecretName": "storage-secret",
			"gcs" : {"gcsCredentialFileName": "gcloud-application-credentials.json"},
			"s3" : {
				"s3AccessKeyIDName": "awsAccessKeyID",
				"s3SecretAccessKeyName": "awsSecretAccessKey",
				"s3Endpoint": "s3.amazonaws.com",
				"s3UseHttps": "1",
				"s3Region": "us-east-2",
				"s3VerifySSL": "1",
				"s3UseVirtualBucket": "",
				"s3UseAnonymousCredential": "false",
				"s3CABundle": ""
			}
		}`,
	},
}

func TestS3CredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "s3-secret",
				Namespace: "default",
			},
		},
	}
	existingS3Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s3-secret",
			Namespace: "default",
			Annotations: map[string]string{
				s3.InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
			},
		},
		Data: map[string][]byte{
			"awsAccessKeyID":     {},
			"awsSecretAccessKey": {},
		},
	}
	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Build s3 secrets envs": {
			serviceAccount: existingServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name: s3.AWSAccessKeyId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "s3-secret",
														},
														Key: "awsAccessKeyID",
													},
												},
											},
											{
												Name: s3.AWSSecretAccessKey,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "s3-secret",
														},
														Key: "awsSecretAccessKey",
													},
												},
											},
											{
												Name:  s3.S3Endpoint,
												Value: "s3.aws.com",
											},
											{
												Name:  s3.AWSEndpointUrl,
												Value: "https://s3.aws.com",
											},
											{
												Name:  s3.S3VerifySSL,
												Value: "1",
											},
											{
												Name:  s3.AWSAnonymousCredential,
												Value: "false",
											},
											{
												Name:  s3.AWSRegion,
												Value: "us-east-2",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(t.Context(), existingS3Secret)).NotTo(gomega.HaveOccurred())

		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil,
			scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(t.Context(), existingS3Secret)).NotTo(gomega.HaveOccurred())
	}
}

func TestS3CredentialBuilderWithStorageSecret(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
	}
	existingS3Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s3-secret",
			Namespace: "default",
			Annotations: map[string]string{
				s3.InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
			},
		},
		Data: map[string][]byte{
			"awsAccessKeyID":     {},
			"awsSecretAccessKey": {},
		},
	}
	scenarios := map[string]struct {
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Build s3 secrets envs": {
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name: s3.AWSAccessKeyId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "s3-secret",
														},
														Key: "awsAccessKeyID",
													},
												},
											},
											{
												Name: s3.AWSSecretAccessKey,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "s3-secret",
														},
														Key: "awsSecretAccessKey",
													},
												},
											},
											{
												Name:  s3.S3Endpoint,
												Value: "s3.aws.com",
											},
											{
												Name:  s3.AWSEndpointUrl,
												Value: "https://s3.aws.com",
											},
											{
												Name:  s3.S3VerifySSL,
												Value: "1",
											},
											{
												Name:  s3.AWSAnonymousCredential,
												Value: "false",
											},
											{
												Name:  s3.AWSRegion,
												Value: "us-east-2",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(t.Context(), existingS3Secret)).NotTo(gomega.HaveOccurred())
		annotations := map[string]string{
			"serving.kserve.io/storageSecretName": "s3-secret",
		}
		err := builder.CreateSecretVolumeAndEnv(existingServiceAccount.Namespace, annotations,
			existingServiceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(t.Context(), existingS3Secret)).NotTo(gomega.HaveOccurred())
	}
}

func TestS3ServiceAccountCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			Annotations: map[string]string{
				s3.InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
				s3.InferenceServiceS3UseAnonymousCredential:   "true",
				AwsIrsaAnnotationKey:                          "arn:aws:iam::123456789012:role/s3access",
			},
		},
	}
	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Build s3 service account envs": {
			serviceAccount: existingServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name:  s3.S3Endpoint,
												Value: "s3.aws.com",
											},
											{
												Name:  s3.AWSEndpointUrl,
												Value: "https://s3.aws.com",
											},
											{
												Name:  s3.S3VerifySSL,
												Value: "1",
											},
											{
												Name:  s3.AWSAnonymousCredential,
												Value: "true",
											},
											{
												Name:  s3.AWSRegion,
												Value: "us-east-2",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())

		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil, scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
	}
}

func TestGCSCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "user-gcp-sa",
				Namespace: "default",
			},
		},
	}
	existingGCSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-gcp-sa",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"gcloud-application-credentials.json": {},
		},
	}
	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Build gcs secrets volume": {
			serviceAccount: existingServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      gcs.GCSCredentialVolumeName,
												ReadOnly:  true,
												MountPath: gcs.GCSCredentialVolumeMountPath,
											},
										},
										Env: []corev1.EnvVar{
											{
												Name:  gcs.GCSCredentialEnvKey,
												Value: gcs.GCSCredentialVolumeMountPath + "gcloud-application-credentials.json",
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: gcs.GCSCredentialVolumeName,
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "user-gcp-sa",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(t.Context(), existingGCSSecret)).NotTo(gomega.HaveOccurred())

		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil, scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(t.Context(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(t.Context(), existingGCSSecret)).NotTo(gomega.HaveOccurred())
	}
}

func TestLegacyAzureCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	customOnlyServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-sa",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "az-custom-secret",
				Namespace: "default",
			},
		},
	}
	customAzureSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "az-custom-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"AZ_SUBSCRIPTION_ID": {},
			"AZ_TENANT_ID":       {},
			"AZ_CLIENT_ID":       {},
			"AZ_CLIENT_SECRET":   {},
		},
	}

	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Custom Azure Secret": {
			serviceAccount: customOnlyServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name: azure.AzureSubscriptionId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.LegacyAzureSubscriptionId,
													},
												},
											},
											{
												Name: azure.AzureTenantId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.LegacyAzureTenantId,
													},
												},
											},
											{
												Name: azure.AzureClientId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.LegacyAzureClientId,
													},
												},
											},
											{
												Name: azure.AzureClientSecret,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.LegacyAzureClientSecret,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	g.Expect(c.Create(t.Context(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Create(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil, scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
	}

	g.Expect(c.Delete(t.Context(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Delete(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())
}

func TestHdfsCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	customOnlyServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-sa",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "hdfs-custom-secret",
				Namespace: "default",
			},
		},
	}
	customHdfsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hdfs-custom-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			hdfs.HdfsNamenode:      []byte("https://testdomain:port"),
			hdfs.HdfsRootPath:      []byte("/"),
			hdfs.KerberosPrincipal: []byte("account@REALM"),
			hdfs.KerberosKeytab:    []byte("AAA="),
		},
	}

	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Custom HDFS Secret": {
			serviceAccount: customOnlyServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      hdfs.HdfsVolumeName,
												ReadOnly:  true,
												MountPath: hdfs.MountPath,
											},
										},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: hdfs.HdfsVolumeName,
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "hdfs-custom-secret",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	g.Expect(c.Create(t.Context(), customHdfsSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Create(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil, scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
	}

	g.Expect(c.Delete(t.Context(), customHdfsSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Delete(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())
}

func TestAzureCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	customOnlyServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-sa",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "az-custom-secret",
				Namespace: "default",
			},
		},
	}
	customAzureSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "az-custom-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"AZURE_SUBSCRIPTION_ID":    {},
			"AZURE_TENANT_ID":          {},
			"AZURE_CLIENT_ID":          {},
			"AZURE_CLIENT_SECRET":      {},
			"AZURE_STORAGE_ACCESS_KEY": {},
		},
	}

	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Custom Azure Secret": {
			serviceAccount: customOnlyServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name: azure.AzureSubscriptionId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureSubscriptionId,
													},
												},
											},
											{
												Name: azure.AzureTenantId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureTenantId,
													},
												},
											},
											{
												Name: azure.AzureClientId,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureClientId,
													},
												},
											},
											{
												Name: azure.AzureClientSecret,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureClientSecret,
													},
												},
											},
											{
												Name: azure.AzureStorageAccessKey,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureStorageAccessKey,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	g.Expect(c.Create(t.Context(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Create(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil, scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
	}

	g.Expect(c.Delete(t.Context(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Delete(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())
}

func TestAzureStorageAccessKeyCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	customOnlyServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-sa",
			Namespace: "default",
		},
		Secrets: []corev1.ObjectReference{
			{
				Name:      "az-custom-secret",
				Namespace: "default",
			},
		},
	}
	customAzureSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "az-custom-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"AZURE_STORAGE_ACCESS_KEY": {},
		},
	}

	scenarios := map[string]struct {
		serviceAccount        *corev1.ServiceAccount
		inputConfiguration    *knservingv1.Configuration
		expectedConfiguration *knservingv1.Configuration
		shouldFail            bool
	}{
		"Custom Azure Secret": {
			serviceAccount: customOnlyServiceAccount,
			inputConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{},
								},
							},
						},
					},
				},
			},
			expectedConfiguration: &knservingv1.Configuration{
				Spec: knservingv1.ConfigurationSpec{
					Template: knservingv1.RevisionTemplateSpec{
						Spec: knservingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Env: []corev1.EnvVar{
											{
												Name: azure.AzureStorageAccessKey,
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureStorageAccessKey,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	g.Expect(c.Create(t.Context(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Create(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())

	builder := NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, nil, scenario.serviceAccount.Name,
			&scenario.inputConfiguration.Spec.Template.Spec.Containers[0],
			&scenario.inputConfiguration.Spec.Template.Spec.Volumes,
		)
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		// Validate
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.expectedConfiguration, scenario.inputConfiguration); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
		}
	}

	g.Expect(c.Delete(t.Context(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Delete(t.Context(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())
}

func TestCredentialBuilder_CreateStorageSpecSecretEnvs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	namespace := "default"
	builder := NewCredentialBuilder(c, clientset, configMap)

	scenarios := map[string]struct {
		secret            *corev1.Secret
		storageKey        string
		storageSecretName string
		overrideParams    map[string]string
		container         *corev1.Container
		shouldFail        bool
		matcher           types.GomegaMatcher
	}{
		"fail on storage secret name is empty": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind: "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: namespace,
				},
				Data: nil,
			},
			storageKey:        "",
			storageSecretName: "",
			overrideParams:    make(map[string]string),
			container:         &corev1.Container{},
			shouldFail:        true,
			matcher:           gomega.HaveOccurred(),
		},
		"storage spec with empty override params": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n      \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "", "bucket": ""},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: false,
			matcher: gomega.Equal(&corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_CONFIG",
						Value: "",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef:         nil,
							ResourceFieldRef: nil,
							ConfigMapKeyRef:  nil,
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "storage-secret",
								},
								Key:      "minio",
								Optional: nil,
							},
						},
					},
					{
						Name:  "STORAGE_OVERRIDE_CONFIG",
						Value: "{\"bucket\":\"\",\"type\":\"\"}",
					},
				},
			}),
		},
		"simple storage spec": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n      \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "s3", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: false,
			matcher: gomega.Equal(&corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_CONFIG",
						Value: "",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef:         nil,
							ResourceFieldRef: nil,
							ConfigMapKeyRef:  nil,
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "storage-secret",
								},
								Key:      "minio",
								Optional: nil,
							},
						},
					},
					{
						Name:  "STORAGE_OVERRIDE_CONFIG",
						Value: "{\"bucket\":\"test-bucket\",\"type\":\"s3\"}",
					},
				},
			}),
		},
		"wrong storage key": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n      \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "wrong-key",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "s3", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: true,
			matcher:    gomega.HaveOccurred(),
		},
		"default storage key": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{DefaultStorageSecretKey + "_s3": "{\n      \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "s3", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: false,
			matcher: gomega.Equal(&corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_CONFIG",
						Value: "",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef:         nil,
							ResourceFieldRef: nil,
							ConfigMapKeyRef:  nil,
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "storage-secret",
								},
								Key:      "default_s3",
								Optional: nil,
							},
						},
					},
					{
						Name:  "STORAGE_OVERRIDE_CONFIG",
						Value: "{\"bucket\":\"test-bucket\",\"type\":\"s3\"}",
					},
				},
			}),
		},
		"default storage key with empty storage type": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{DefaultStorageSecretKey: "{\n      \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: false,
			matcher: gomega.Equal(&corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_CONFIG",
						Value: "",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef:         nil,
							ResourceFieldRef: nil,
							ConfigMapKeyRef:  nil,
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "storage-secret",
								},
								Key:      "default",
								Optional: nil,
							},
						},
					},
					{
						Name:  "STORAGE_OVERRIDE_CONFIG",
						Value: "{\"bucket\":\"test-bucket\",\"type\":\"\"}",
					},
				},
			}),
		},
		"storage spec with uri scheme placeholder": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n      \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "s3", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"<scheme-placeholder>://models/example-model/",
					"/mnt/models/",
				},
			},
			shouldFail: false,
			matcher: gomega.Equal(&corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/example-model/",
					"/mnt/models/",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_CONFIG",
						Value: "",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef:         nil,
							ResourceFieldRef: nil,
							ConfigMapKeyRef:  nil,
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "storage-secret",
								},
								Key:      "minio",
								Optional: nil,
							},
						},
					},
					{
						Name:  "STORAGE_OVERRIDE_CONFIG",
						Value: "{\"bucket\":\"test-bucket\",\"type\":\"s3\"}",
					},
				},
			}),
		},
		"hdfs with uri scheme placeholder": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"hdfs": "{\n      \"type\": \"hdfs\",\n      \"access_key_id\": \"hdfs34\",\n      \"secret_access_key\": \"hdfs123\",\n      \"endpoint_url\": \"http://hdfs-service.kubeflow\",\n     \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "hdfs",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "hdfs", "bucket": ""},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"<scheme-placeholder>://models/example-model/",
					"/mnt/models/",
				},
			},
			shouldFail: false,
			matcher: gomega.Equal(&corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"hdfs://models/example-model/",
					"/mnt/models/",
				},
				Env: []corev1.EnvVar{
					{
						Name:  "STORAGE_CONFIG",
						Value: "",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef:         nil,
							ResourceFieldRef: nil,
							ConfigMapKeyRef:  nil,
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "storage-secret",
								},
								Key:      "hdfs",
								Optional: nil,
							},
						},
					},
					{
						Name:  "STORAGE_OVERRIDE_CONFIG",
						Value: "{\"bucket\":\"\",\"type\":\"hdfs\"}",
					},
				},
			}),
		},
		"unsupported storage type": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n     \"type\": \"gs\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"gs://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: true,
			matcher:    gomega.HaveOccurred(),
		},
		"secret data with syntax error": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n   {  \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: true,
			matcher:    gomega.HaveOccurred(),
		},
		"fail on storage type is empty": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n    \"type\": \"\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"test-bucket\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "", "bucket": "test-bucket"},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"s3://test-bucket/models/",
					"/mnt/models/",
				},
			},
			shouldFail: true,
			matcher:    gomega.HaveOccurred(),
		},
		"fail on bucket is empty on s3 storage": {
			secret: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-secret",
					Namespace: namespace,
				},
				StringData: map[string]string{"minio": "{\n    \"type\": \"s3\",\n      \"access_key_id\": \"minio\",\n      \"secret_access_key\": \"minio123\",\n      \"endpoint_url\": \"http://minio-service.kubeflow:9000\",\n      \"bucket\": \"\",\n      \"region\": \"us-south\"\n    }"},
			},
			storageKey:        "minio",
			storageSecretName: "storage-secret",
			overrideParams:    map[string]string{"type": "s3", "bucket": ""},
			container: &corev1.Container{
				Name:  "init-container",
				Image: "kserve/init-container:latest",
				Args: []string{
					"<scheme-placeholder>://models/example-model/",
					"/mnt/models/",
				},
			},
			shouldFail: true,
			matcher:    gomega.HaveOccurred(),
		},
	}

	for _, tc := range scenarios {
		if err := c.Create(t.Context(), tc.secret); err != nil {
			t.Errorf("Failed to create secret %s: %v", "storage-secret", err)
		}
		err := builder.CreateStorageSpecSecretEnvs(namespace, nil, tc.storageKey, tc.overrideParams, tc.container)
		if !tc.shouldFail {
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			g.Expect(tc.container).Should(tc.matcher)
		} else {
			g.Expect(err).To(tc.matcher)
		}
		if err := c.Delete(t.Context(), tc.secret); err != nil {
			t.Errorf("Failed to delete secret %s because of: %v", tc.secret.Name, err)
		}
	}
}
