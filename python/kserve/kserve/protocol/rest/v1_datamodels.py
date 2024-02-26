# Copyright 2024 The KServe Authors.
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

from pydantic import BaseModel, root_validator


class PredictRequest(BaseModel):
    instances: list

    # @validator("instances", pre=True, always=True)
    @root_validator(pre=True)
    def convert_inputs_to_instances(cls, values):
        # Check for both "instances" and "inputs" keys
        if "instances" in values and "inputs" in values:
            raise ValueError("Both 'instances' and 'inputs' keys are present. Only one is allowed.")
        elif "inputs" in values:
            values["instances"] = values["inputs"]
        return values


class PredictResponse(BaseModel):
    predictions: list
