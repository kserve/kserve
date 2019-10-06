/*
Copyright 2019 kubeflow.org.

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

package v1alpha2

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	"testing"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestKFService(t *testing.T) {
	key := types.NamespacedName{
		Name:      "foo",
		Namespace: "default",
	}
	created := &KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: KFServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					DeploymentSpec: DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
			CanaryTrafficPercent: 20,
			Canary: &EndpointSpec{
				Predictor: PredictorSpec{
					DeploymentSpec: DeploymentSpec{
						MinReplicas: 1,
						MaxReplicas: 3,
					},
					Tensorflow: &TensorflowSpec{
						StorageURI:     "s3://test/mnist-2/export",
						RuntimeVersion: "1.13.0",
					},
				},
			},
		},
	}
	g := gomega.NewGomegaWithT(t)

	// Test Create
	fetched := &KFService{}
	g.Expect(c.Create(context.TODO(), created)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Get(context.TODO(), key, fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(fetched).To(gomega.Equal(created))

	// Test Updating the Labels
	updated := fetched.DeepCopy()
	updated.Labels = map[string]string{"hello": "world"}
	g.Expect(c.Update(context.TODO(), updated)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Get(context.TODO(), key, fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(fetched).To(gomega.Equal(updated))

	// Test status update
	statusUpdated := fetched.DeepCopy()
	statusUpdated.Status = KFServiceStatus{
		URL:           "example.dev.com",
		Traffic:       20,
		CanaryTraffic: 80,
		Default: &EndpointStatusMap{
			constants.Predictor: &StatusConfigurationSpec{
				Name:     "v1",
				Replicas: 2,
			},
		},
		Canary: &EndpointStatusMap{
			constants.Predictor: &StatusConfigurationSpec{
				Name:     "v2",
				Replicas: 3,
			},
		},
	}
	g.Expect(c.Update(context.TODO(), statusUpdated)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Get(context.TODO(), key, fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(fetched).To(gomega.Equal(statusUpdated))

	// Test Delete
	g.Expect(c.Delete(context.TODO(), fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Get(context.TODO(), key, fetched)).To(gomega.HaveOccurred())
}
