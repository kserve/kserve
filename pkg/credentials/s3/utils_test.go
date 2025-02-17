/*
Copyright 2022 The KServe Authors.

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

package s3

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildS3EnvVars(t *testing.T) {
	scenarios := map[string]struct {
		config      S3Config
		annotations map[string]string
		expected    []corev1.EnvVar
	}{
		"S3Endpoint": {
			annotations: map[string]string{
				InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
			},
			expected: []corev1.EnvVar{
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "https://s3.aws.com",
				},
			},
		},
		"AllAnnotations": {
			annotations: map[string]string{
				InferenceServiceS3SecretEndpointAnnotation:    "s3.aws.com",
				InferenceServiceS3SecretRegionAnnotation:      "us-east-2",
				InferenceServiceS3SecretSSLAnnotation:         "0",
				InferenceServiceS3SecretHttpsAnnotation:       "0",
				InferenceServiceS3UseVirtualBucketAnnotation:  "true",
				InferenceServiceS3UseAccelerateAnnotation:     "true",
				InferenceServiceS3UseAnonymousCredential:      "true",
				InferenceServiceS3CABundleAnnotation:          "value",
				InferenceServiceS3CABundleConfigMapAnnotation: "value",
			},
			expected: []corev1.EnvVar{
				{
					Name:  S3UseHttps,
					Value: "0",
				},
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "http://s3.aws.com",
				},
				{
					Name:  S3VerifySSL,
					Value: "0",
				},
				{
					Name:  AWSAnonymousCredential,
					Value: "true",
				},
				{
					Name:  AWSRegion,
					Value: "us-east-2",
				},
				{
					Name:  S3UseVirtualBucket,
					Value: "true",
				},
				{
					Name:  S3UseAccelerate,
					Value: "true",
				},
				{
					Name:  AWSCABundle,
					Value: "value",
				},
				{
					Name:  AWSCABundleConfigMap,
					Value: "value",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		envs := BuildS3EnvVars(scenario.annotations, &scenario.config)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
