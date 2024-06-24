/*
Copyright 2023 The KServe Authors.

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

package service

import (
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var (
	pboolT = true
	pboolF = false
)

func TestCreateServiceRawServiceConfigEmpty(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{}
	// nothing expected
	runTestServiceCreate(serviceConfig, "", t)
}

func TestCreateServiceRawServiceAndConfigNil(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{}
	serviceConfig = nil
	// no service means empty
	runTestServiceCreate(serviceConfig, "", t)
}

func TestCreateServiceRawFalseAndConfigTrue(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{
		ServiceClusterIPNone: &pboolT,
	}
	runTestServiceCreate(serviceConfig, corev1.ClusterIPNone, t)
}

func TestCreateServiceRawTrueAndConfigFalse(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{
		ServiceClusterIPNone: &pboolF,
	}
	runTestServiceCreate(serviceConfig, "", t)
}

func TestCreateServiceRawFalseAndConfigNil(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{
		ServiceClusterIPNone: nil,
	}
	runTestServiceCreate(serviceConfig, "", t)
}

func TestCreateServiceRawTrueAndConfigNil(t *testing.T) {
	serviceConfig := &v1beta1.ServiceConfig{
		ServiceClusterIPNone: nil,
	}
	// service is there, but no property, should be empty
	runTestServiceCreate(serviceConfig, "", t)
}

func runTestServiceCreate(serviceConfig *v1beta1.ServiceConfig, expectedClusterIP string, t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{}
	podSpec := &corev1.PodSpec{}

	service := createService(componentMeta, componentExt, podSpec, serviceConfig)
	assert.Equal(t, componentMeta, service.ObjectMeta, "Expected ObjectMeta to be equal")
	assert.Equal(t, map[string]string{"app": "isvc.test-service"}, service.Spec.Selector, "Expected Selector to be equal")
	assert.Equal(t, expectedClusterIP, service.Spec.ClusterIP, "Expected ClusterIP to be equal")
}
