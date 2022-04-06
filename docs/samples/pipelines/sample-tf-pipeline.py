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

import kfp.compiler as compiler
import kfp.dsl as dsl
from kfp import components

# kfserving_op = components.load_component_from_url('https://raw.githubusercontent.com/kubeflow/pipelines/'
#                                                  'master/components/kubeflow/kfserving/component.yaml')
kserve_op = components.load_component_from_url('https://raw.githubusercontent.com/kubeflow/pipelines/'
                                               'master/components/kserve/component.yaml')


@dsl.pipeline(
    name='KServe pipeline',
    description='A pipeline for KServe.'
)
def kservePipeline(
        action='apply',
        model_name='tensorflow-sample',
        model_uri='gs://kfserving-examples/models/tensorflow/flowers',
        namespace='anonymous',
        framework='tensorflow'):
    kserve_op(action=action,
              model_name=model_name,
              model_uri=model_uri,
              namespace=namespace,
              framework=framework)


if __name__ == '__main__':
    compiler.Compiler().compile(kservePipeline, __file__ + '.tar.gz')
