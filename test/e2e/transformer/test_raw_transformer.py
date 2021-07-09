# Copyright 2021 kubeflow.org.
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

import os
import json
import requests
import time
import logging
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1beta1PredictorSpec
from kfserving import V1beta1TransformerSpec
from kfserving import V1beta1TorchServeSpec
from kfserving import V1beta1InferenceServiceSpec
from kfserving import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
from kubernetes.client import V1EnvVar
from ..common.utils import get_cluster_ip
from ..common.utils import KFSERVING_TEST_NAMESPACE
logging.basicConfig(level=logging.INFO)
KFServing = KFServingClient(
    config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_transformer():
    service_name = 'raw'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri='gs://kfserving-examples/models/torchserve/image_classifier',
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '2Gi'},
                limits={'cpu': '100m', 'memory': '2Gi'}
            )
        ),
    )
    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[V1Container(
            image='kfserving/torchserve-image-transformer:latest',
            name='kfserving-container',
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '2Gi'},
                limits={'cpu': '100m', 'memory': '2Gi'}),
            env=[V1EnvVar(name="STORAGE_URI", value="gs://kfserving-examples/models/torchserve/image_classifier")])]
    )

    annotations = dict()
    annotations['serving.kubeflow.org/raw'] = 'true'
    annotations['kubernetes.io/ingress.class'] = 'istio'
    isvc = V1beta1InferenceService(api_version=constants.KFSERVING_V1BETA1,
                                   kind=constants.KFSERVING_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KFSERVING_TEST_NAMESPACE, annotations=annotations),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer))

    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(
            service_name, namespace=KFSERVING_TEST_NAMESPACE)
    except RuntimeError as e:
        raise e

    time.sleep(30)

    isvc = KFServing.get(
        service_name,
        namespace=KFSERVING_TEST_NAMESPACE,
    )

    cluster_ip = get_cluster_ip()
    logging.info("clusterip = %s", cluster_ip)

    host = isvc["status"]["url"]
    host = host[host.rfind('/')+1:]
    url = 'http://{}/v1/models/mnist:predict'.format(cluster_ip)
    logging.info("url = %s ", url)
    headers = {"Host": host}
    data_str = '{"instances": [{"data": "iVBORw0KGgoAAAANSUhEUgAAABwAAAAcCAAAAABXZoBIAAAAw0lE\
    QVR4nGNgGFggVVj4/y8Q2GOR83n+58/fP0DwcSqmpNN7oOTJw6f+/H2pjUU2JCSEk0EWqN0cl828e/FIxvz9/9cCh1\
        zS5z9/G9mwyzl/+PNnKQ45nyNAr9ThMHQ/UG4tDofuB4bQIhz6fIBenMWJQ+7Vn7+zeLCbKXv6z59NOPQVgsIcW\
            4QA9YFi6wNQLrKwsBebW/68DJ388Nun5XFocrqvIFH59+XhBAxThTfeB0r+vP/QHbuDCgr2JmOXoSsAAKK7b\
                U3vISS4AAAAAElFTkSuQmCC", "target": 0}]}'
    res = requests.post(url, data_str, headers=headers)
    logging.info("res.text = %s", res.text)
    preds = json.loads(res.content.decode("utf-8"))
    assert(preds["predictions"] == [2])

    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
