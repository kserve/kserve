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
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func feastKFService() KFService {
	kfservice := KFService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: KFServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					Tensorflow: &TensorflowSpec{StorageURI: "gs://testbucket/testmodel"},
				},
				Transformer: &TransformerSpec{
					Feast: &FeastTransformerSpec{
						FeastURL:   "https://feast.svc",
						DataType:   TensorProto,
						EntityIds:  []string{"e1"},
						FeatureIds: []string{"f1"},
					},
				},
			},
		},
	}
	kfservice.Default()
	return kfservice
}

func TestFeastValidation(t *testing.T) {
	invalidFeastURLKfsvc := feastKFService()
	invalidFeastURLKfsvc.Spec.Default.Transformer.Feast.FeastURL = "invalidurl"

	invalidDataTypeKfsvc := feastKFService()
	invalidDataTypeKfsvc.Spec.Default.Transformer.Feast.DataType = "unknown type"

	emptyEntityIdsKfsvc := feastKFService()
	emptyEntityIdsKfsvc.Spec.Default.Transformer.Feast.EntityIds = []string{}

	emptyFeatureIdsKfsvc := feastKFService()
	emptyFeatureIdsKfsvc.Spec.Default.Transformer.Feast.FeatureIds = []string{}

	scenarios := map[string]struct {
		configMapData map[string]string
		kfsvc         KFService
		matcher       types.GomegaMatcher
	}{
		"ValidFeastTransformer": {
			kfsvc:   feastKFService(),
			matcher: gomega.Succeed(),
		},
		"InvalidFeastURL": {
			kfsvc:   invalidFeastURLKfsvc,
			matcher: gomega.MatchError(InvalidFeastURLError),
		},
		"InvalidFeastDataType": {
			kfsvc:   invalidDataTypeKfsvc,
			matcher: gomega.MatchError(InvalidFeastDataTypeError),
		},
		"EmptyEntityIds": {
			kfsvc:   emptyEntityIdsKfsvc,
			matcher: gomega.MatchError(InvalidEntityIdsError),
		},
		"EmptyFeatureIdsError": {
			kfsvc:   emptyFeatureIdsKfsvc,
			matcher: gomega.MatchError(InvalidFeatureIdsError),
		},
	}

	for name, scenario := range scenarios {
		g := gomega.NewGomegaWithT(t)
		g.Expect(scenario.kfsvc.ValidateCreate()).Should(
			scenario.matcher, fmt.Sprintf("Failed scenario %s, failed expectation for ValidateCreate().", name))
	}
}
