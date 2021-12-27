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
	"context"
	"testing"

	"github.com/kserve/kserve/pkg/credentials/azure"

	"github.com/google/go-cmp/cmp"
	"github.com/kserve/kserve/pkg/credentials/gcs"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

var configMap = &v1.ConfigMap{
	Data: map[string]string{
		"credentials": `{
			"gcs" : {"gcsCredentialFileName": "gcloud-application-credentials.json"},
			"s3" : {
				"s3AccessKeyIDName": "awsAccessKeyID",
				"s3SecretAccessKeyName": "awsSecretAccessKey"
			}
	    }`,
	},
}

func TestS3CredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingServiceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
		Secrets: []v1.ObjectReference{
			{
				Name:      "s3-secret",
				Namespace: "default",
			},
		},
	}
	existingS3Secret := &v1.Secret{
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
		serviceAccount        *v1.ServiceAccount
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Env: []v1.EnvVar{
											{
												Name: s3.AWSAccessKeyId,
												ValueFrom: &v1.EnvVarSource{
													SecretKeyRef: &v1.SecretKeySelector{
														LocalObjectReference: v1.LocalObjectReference{
															Name: "s3-secret",
														},
														Key: "awsAccessKeyID",
													},
												},
											},
											{
												Name: s3.AWSSecretAccessKey,
												ValueFrom: &v1.EnvVarSource{
													SecretKeyRef: &v1.SecretKeySelector{
														LocalObjectReference: v1.LocalObjectReference{
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

	builder := NewCredentialBulder(c, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(context.TODO(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(context.TODO(), existingS3Secret)).NotTo(gomega.HaveOccurred())

		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, scenario.serviceAccount.Name,
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
		g.Expect(c.Delete(context.TODO(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(context.TODO(), existingS3Secret)).NotTo(gomega.HaveOccurred())

	}
}

func TestGCSCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingServiceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
		Secrets: []v1.ObjectReference{
			{
				Name:      "user-gcp-sa",
				Namespace: "default",
			},
		},
	}
	existingGCSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-gcp-sa",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"gcloud-application-credentials.json": {},
		},
	}
	scenarios := map[string]struct {
		serviceAccount        *v1.ServiceAccount
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										VolumeMounts: []v1.VolumeMount{
											{
												Name:      gcs.GCSCredentialVolumeName,
												ReadOnly:  true,
												MountPath: gcs.GCSCredentialVolumeMountPath,
											},
										},
										Env: []v1.EnvVar{
											{
												Name:  gcs.GCSCredentialEnvKey,
												Value: gcs.GCSCredentialVolumeMountPath + "gcloud-application-credentials.json",
											},
										},
									},
								},
								Volumes: []v1.Volume{
									{
										Name: gcs.GCSCredentialVolumeName,
										VolumeSource: v1.VolumeSource{
											Secret: &v1.SecretVolumeSource{
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

	builder := NewCredentialBulder(c, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(context.TODO(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(context.TODO(), existingGCSSecret)).NotTo(gomega.HaveOccurred())

		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, scenario.serviceAccount.Name,
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
		g.Expect(c.Delete(context.TODO(), existingServiceAccount)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(context.TODO(), existingGCSSecret)).NotTo(gomega.HaveOccurred())

	}
}

func TestAzureCredentialBuilder(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	customOnlyServiceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-sa",
			Namespace: "default",
		},
		Secrets: []v1.ObjectReference{
			{
				Name:      "az-custom-secret",
				Namespace: "default",
			},
		},
	}
	customAzureSecret := &v1.Secret{
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
		serviceAccount        *v1.ServiceAccount
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
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
							PodSpec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Env: []v1.EnvVar{
											{
												Name: azure.AzureSubscriptionId,
												ValueFrom: &v1.EnvVarSource{
													SecretKeyRef: &v1.SecretKeySelector{
														LocalObjectReference: v1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureSubscriptionId,
													},
												},
											},
											{
												Name: azure.AzureTenantId,
												ValueFrom: &v1.EnvVarSource{
													SecretKeyRef: &v1.SecretKeySelector{
														LocalObjectReference: v1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureTenantId,
													},
												},
											},
											{
												Name: azure.AzureClientId,
												ValueFrom: &v1.EnvVarSource{
													SecretKeyRef: &v1.SecretKeySelector{
														LocalObjectReference: v1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureClientId,
													},
												},
											},
											{
												Name: azure.AzureClientSecret,
												ValueFrom: &v1.EnvVarSource{
													SecretKeyRef: &v1.SecretKeySelector{
														LocalObjectReference: v1.LocalObjectReference{
															Name: "az-custom-secret",
														},
														Key: azure.AzureClientSecret,
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

	g.Expect(c.Create(context.TODO(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Create(context.TODO(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())

	builder := NewCredentialBulder(c, configMap)
	for name, scenario := range scenarios {

		err := builder.CreateSecretVolumeAndEnv(scenario.serviceAccount.Namespace, scenario.serviceAccount.Name,
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

	g.Expect(c.Delete(context.TODO(), customAzureSecret)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Delete(context.TODO(), customOnlyServiceAccount)).NotTo(gomega.HaveOccurred())
}
