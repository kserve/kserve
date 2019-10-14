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
from kfserving import KFServingClient

KFServing = KFServingClient(load_kube_config=True)

def wait_for_kfservice_ready(name, namespace='kfserving-ci-e2e-test', Timeout_seconds=600):
    for _ in range(round(Timeout_seconds/10)):
        time.sleep(10)
        kfsvc_status = KFServing.get(name, namespace=namespace)
        for condition in kfsvc_status['status'].get('conditions', {}):
            if condition.get('type', '') == 'Ready':
                status = condition.get('status', 'Unknown')
        if status == 'True':
            return
    raise RuntimeError("Timeout to start the KFService.")
