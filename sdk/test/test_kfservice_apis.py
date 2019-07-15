from unittest.mock import patch

from kubernetes import client

from kfserving.models.v1alpha1_model_spec import V1alpha1ModelSpec
from kfserving.models.v1alpha1_tensorflow_spec import V1alpha1TensorflowSpec
from kfserving.models.v1alpha1_kf_service_spec import V1alpha1KFServiceSpec
from kfserving.models.v1alpha1_kf_service import V1alpha1KFService
from kfserving.api.kf_serving_api import KFServingApi

KFServing = KFServingApi()

def generate_kfservice():
    default_model_spec = V1alpha1ModelSpec(tensorflow=V1alpha1TensorflowSpec(
        model_uri='gs://kfserving-samples/models/tensorflow/flowers'))
    
    kfsvc = V1alpha1KFService(api_version='serving.kubeflow.org/v1alpha1',
                                  kind='KFService',
                                  metadata=client.V1ObjectMeta(name='flower-sample'),
                                  spec=V1alpha1KFServiceSpec(default=default_model_spec))
    return kfsvc

# Unit test for kfserving deploy api
def test_kfservice_api_deploy():
    unit_result = 'test_kfservice'
    with patch('kfserving.api.kf_serving_api.KFServingApi.deploy', return_value=unit_result):
        kfsvc = generate_kfservice()
        assert unit_result == KFServing.deploy(kfsvc, namespace='kubeflow')

# Unit test for kfserving get api
def test_kfservice_api_deploy():
    unit_result = 'test_kfservice'
    with patch('kfserving.api.kf_serving_api.KFServingApi.get', return_value=unit_result):
        assert unit_result == KFServing.get('flower-sample', namespace='kubeflow')

# Unit test for kfserving patch api
def test_kfservice_api_deploy():
    unit_result = 'test_kfservice'
    with patch('kfserving.api.kf_serving_api.KFServingApi.patch', return_value=unit_result):
        kfsvc = generate_kfservice()
        assert unit_result == KFServing.patch('flower-sample', kfsvc, namespace='kubeflow')

# Unit test for kfserving delete api
def test_kfservice_api_deploy():
    unit_result = 'test_kfservice'
    with patch('kfserving.api.kf_serving_api.KFServingApi.delete', return_value=unit_result):
        assert unit_result == KFServing.delete('flower-sample', namespace='kubeflow')
