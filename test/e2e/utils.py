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
import time
import logging
from kubernetes import client, config
from kfserving import KFServingClient

logging.basicConfig(level=logging.INFO)

KFServing = KFServingClient(config_file="~/.kube/config")

def wait_for_isvc_ready(name, namespace='kfserving-ci-e2e-test',
                             Timeout_seconds=600, debug=True):
    for _ in range(round(Timeout_seconds/10)):
        time.sleep(10)
        kfsvc_status = KFServing.get(name, namespace=namespace)
        status = 'Unknown'
        for condition in kfsvc_status['status'].get('conditions', {}):
            if condition.get('type', '') == 'Ready':
                status = condition.get('status', 'Unknown')
        if status == 'True':
            return
    if debug is True:
        logging.warning("Timeout to start the InferenceService %s.", name)
        logging.info("Getting the detailed InferenceService ...")
        logging.info(KFServing.get(name, namespace=namespace))
        get_pod_log(pod='kfserving-controller-manager-0',
                    namespace='kfserving-system',
                    container='manager')
    raise RuntimeError("Timeout to start the InferenceService %s. See above log for details.", name)


def get_pod_log(pod, namespace='kfserving-system', container=''):
    '''
    Note the arg container must be '' here, instead of None.
    Otherwise API read_namespaced_pod_log will fail if no specified container.
    '''
    logging.info("Getting logs of Pod %s ... ", pod)
    try:
        config.load_kube_config()
        core_api = client.CoreV1Api()
        pod_logs = core_api.read_namespaced_pod_log(pod, namespace, container=container)
        logging.info("The logs of Pod %s log:\n %s", pod, pod_logs)
    except client.rest.ApiException as e:
        logging.info("Exception when calling CoreV1Api->read_namespaced_pod_log: %s\n", e)
