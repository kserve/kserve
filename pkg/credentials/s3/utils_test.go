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
		annotations map[string]string
		secret      *map[string][]byte
		config      S3Config
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
		"AllSecret": {
			secret: &map[string][]byte{
				S3UseHttps:             []byte("0"),
				S3Endpoint:             []byte("s3.aws.com"),
				S3VerifySSL:            []byte("0"),
				AWSAnonymousCredential: []byte("true"),
				AWSRegion:              []byte("us-east-2"),
				S3UseVirtualBucket:     []byte("true"),
				S3UseAccelerate:        []byte("true"),
				AWSCABundle:            []byte("value"),
				AWSCABundleConfigMap:   []byte("value"),
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
		"AllS3Config": {
			config: S3Config{
				S3Endpoint:               "s3.aws.com",
				S3UseHttps:               "0",
				S3Region:                 "us-east-2",
				S3VerifySSL:              "0",
				S3UseVirtualBucket:       "true",
				S3UseAccelerate:          "true",
				S3UseAnonymousCredential: "true",
				S3CABundleConfigMap:      "value",
				S3CABundle:               "value",
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
		"AnnotationsSecretAndS3Config": {
			annotations: map[string]string{
				InferenceServiceS3SecretEndpointAnnotation:    "s3.aws.com-annotation",
				InferenceServiceS3SecretRegionAnnotation:      "us-east-2-annotation",
				InferenceServiceS3SecretSSLAnnotation:         "0-annotation",
				InferenceServiceS3UseVirtualBucketAnnotation:  "true-annotation",
				InferenceServiceS3UseAccelerateAnnotation:     "true-annotation",
				InferenceServiceS3UseAnonymousCredential:      "true-annotation",
				InferenceServiceS3CABundleAnnotation:          "value-annotation",
				InferenceServiceS3CABundleConfigMapAnnotation: "value-annotation",
			},
			secret: &map[string][]byte{
				S3Endpoint:             []byte("s3.aws.com-secret"),
				S3VerifySSL:            []byte("0-secret"),
				AWSAnonymousCredential: []byte("true-secret"),
				AWSRegion:              []byte("us-east-2-secret"),
				S3UseVirtualBucket:     []byte("true-secret"),
				S3UseAccelerate:        []byte("true-secret"),
				AWSCABundle:            []byte("value-secret"),
				AWSCABundleConfigMap:   []byte("value-secret"),
			},
			config: S3Config{
				S3Endpoint:               "s3.aws.com-config",
				S3Region:                 "us-east-2-config",
				S3VerifySSL:              "0-config",
				S3UseVirtualBucket:       "true-config",
				S3UseAccelerate:          "true-config",
				S3UseAnonymousCredential: "true-config",
				S3CABundleConfigMap:      "value-config",
				S3CABundle:               "value-config",
			},
			expected: []corev1.EnvVar{
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com-annotation",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "https://s3.aws.com-annotation",
				},
				{
					Name:  S3VerifySSL,
					Value: "0-annotation",
				},
				{
					Name:  AWSAnonymousCredential,
					Value: "true-annotation",
				},
				{
					Name:  AWSRegion,
					Value: "us-east-2-annotation",
				},
				{
					Name:  S3UseVirtualBucket,
					Value: "true-annotation",
				},
				{
					Name:  S3UseAccelerate,
					Value: "true-annotation",
				},
				{
					Name:  AWSCABundle,
					Value: "value-annotation",
				},
				{
					Name:  AWSCABundleConfigMap,
					Value: "value-annotation",
				},
			},
		},
		"SecretAndS3Config": {
			secret: &map[string][]byte{
				S3Endpoint:             []byte("s3.aws.com-secret"),
				S3VerifySSL:            []byte("0-secret"),
				AWSAnonymousCredential: []byte("true-secret"),
				AWSRegion:              []byte("us-east-2-secret"),
				S3UseVirtualBucket:     []byte("true-secret"),
				S3UseAccelerate:        []byte("true-secret"),
				AWSCABundle:            []byte("value-secret"),
				AWSCABundleConfigMap:   []byte("value-secret"),
			},
			config: S3Config{
				S3Endpoint:               "s3.aws.com-config",
				S3Region:                 "us-east-2-config",
				S3VerifySSL:              "0-config",
				S3UseVirtualBucket:       "true-config",
				S3UseAccelerate:          "true-config",
				S3UseAnonymousCredential: "true-config",
				S3CABundleConfigMap:      "value-config",
				S3CABundle:               "value-config",
			},
			expected: []corev1.EnvVar{
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com-secret",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "https://s3.aws.com-secret",
				},
				{
					Name:  S3VerifySSL,
					Value: "0-secret",
				},
				{
					Name:  AWSAnonymousCredential,
					Value: "true-secret",
				},
				{
					Name:  AWSRegion,
					Value: "us-east-2-secret",
				},
				{
					Name:  S3UseVirtualBucket,
					Value: "true-secret",
				},
				{
					Name:  S3UseAccelerate,
					Value: "true-secret",
				},
				{
					Name:  AWSCABundle,
					Value: "value-secret",
				},
				{
					Name:  AWSCABundleConfigMap,
					Value: "value-secret",
				},
			},
		},
	}
	for name, scenario := range scenarios {
		envs := BuildS3EnvVars(scenario.annotations, scenario.secret, &scenario.config)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
