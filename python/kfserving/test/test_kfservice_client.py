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
    "api_version": "serving.kubeflow.org/v1alpha1",
    "kind": "KFService",
    "metadata": {
        "name": "flower-sample",
        "namespace": "kubeflow"
    },
    "spec": {
        "default": {
            "tensorflow": {
                "model_uri": "gs://kfserving-samples/models/tensorflow/flowers"
            }
        }
    }
}
 '''

def generate_kfservice():
    default_model_spec = V1alpha2ModelSpec(tensorflow=V1alpha2TensorflowSpec(
        model_uri='gs://kfserving-samples/models/tensorflow/flowers'))

    kfsvc = V1alpha2KFService(api_version='serving.kubeflow.org/v1alpha1',
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
