# Copyright 2022 The KServe Authors.
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

from kserve import InferRequest, InferInput, InferenceServerClient
import json
import base64

client = InferenceServerClient(url="localhost:8081", verbose=True)
json_file = open("./input.json")
data = json.load(json_file)
infer_input = InferInput(name="input-0", shape=[1], datatype="BYTES", data=[base64.b64decode(data["instances"][0]["image"]["b64"])])
request = InferRequest(infer_inputs=[infer_input], model_name="custom-model")
res = client.infer(infer_request=request)
print(res)

