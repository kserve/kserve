# Copyright 2019 kubeflow.org.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from unittest.mock import patch

from kubernetes import client

from kfserving import V1alpha2ModelSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2KFServiceSpec
from kfserving import V1alpha2KFService
from kfserving import KFServingClient

KFServing = KFServingClient()

mocked_unit_result = \
'''
{
    "api_version": "serving.kubeflow.org/v1alpha2",
    "kind": "KFService",
    "metadata": {
        "name": "flower-sample",
        "namespace": "kubeflow"
    },
    "spec": {
        "default": {
            "predictor": {
                "tensorflow": {
                    "storage_uri": "gs://kfserving-samples/models/tensorflow/flowers"
                }
            }
        }
    }
}
 '''

def generate_kfservice():
    default_model_spec = V1alpha2ModelSpec(tensorflow=V1alpha2TensorflowSpec(
        storage_uri='gs://kfserving-samples/models/tensorflow/flowers'))

    kfsvc = V1alpha2KFService(api_version='serving.kubeflow.org/v1alpha2',
                              kind='KFService',
                              metadata=client.V1ObjectMeta(name='flower-sample'),
                              spec=V1alpha2KFServiceSpec(default=default_model_spec))
    return kfsvc

def test_kfservice_client_creat():
    '''Unit test for kfserving create api'''
    with patch('kfserving.api.kf_serving_client.KFServingClient.create',
               return_value=mocked_unit_result):
        kfsvc = generate_kfservice()
        assert mocked_unit_result == KFServing.create(kfsvc, namespace='kubeflow')

def test_kfservice_client_get():
    '''Unit test for kfserving get api'''
    with patch('kfserving.api.kf_serving_client.KFServingClient.get',
               return_value=mocked_unit_result):
        assert mocked_unit_result == KFServing.get('flower-sample', namespace='kubeflow')

def test_kfservice_client_watch():
    '''Unit test for kfserving get api'''
    with patch('kfserving.api.kf_serving_client.KFServingClient.get',
               return_value=mocked_unit_result):
        assert mocked_unit_result == KFServing.get('flower-sample', namespace='kubeflow',
                                                   watch=True, timeout_seconds=120)

def test_kfservice_client_patch():
    '''Unit test for kfserving patch api'''
    with patch('kfserving.api.kf_serving_client.KFServingClient.patch',
               return_value=mocked_unit_result):
        kfsvc = generate_kfservice()
        assert mocked_unit_result == KFServing.patch('flower-sample', kfsvc, namespace='kubeflow')

def test_kfservice_client_delete():
    '''Unit test for kfserving delete api'''
    with patch('kfserving.api.kf_serving_client.KFServingClient.delete',
               return_value=mocked_unit_result):
        assert mocked_unit_result == KFServing.delete('flower-sample', namespace='kubeflow')
