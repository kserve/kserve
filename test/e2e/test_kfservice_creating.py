import time
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2InferenceService
from kubernetes.client import V1ResourceRequirements

api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION
KFServing = KFServingClient(load_kube_config=True)


def test_tensorflow_kfserving():
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            tensorflow=V1alpha2TensorflowSpec(
                storage_uri='gs://kfserving-samples/models/tensorflow/flowers',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '256Mi'},
                    limits={'cpu': '100m', 'memory': '256Mi'}))))

    isvc = V1alpha2InferenceService(api_version=api_version,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name='flower-sample', namespace='kfserving-ci-e2e-test'),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))

    KFServing.create(isvc)
    for _ in range(60):
        time.sleep(10)
        kfsvc_status = KFServing.get(
            'flower-sample', namespace='kfserving-ci-e2e-test')
        for condition in kfsvc_status['status'].get('conditions', {}):
            if condition.get('type', '') == 'Ready':
                status = condition.get('status', 'Unknown')
        if status == 'True':
            return

    raise RuntimeError("Timeout. Tensorflow KFService is not Ready in 10 minutes.")
