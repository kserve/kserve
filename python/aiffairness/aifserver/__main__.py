# Copyright 2021 The KServe Authors.
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

import argparse
import kserve
import json

from .model import AIFModel

DEFAULT_MODEL_NAME = "aifserver"


parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])

parser.add_argument('--model_name',
                    default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')

parser.add_argument('--predictor_host',
                    help='The host for the predictor.',
                    required=True)

# Parameters for describing the model being used for aif360
# ie. feature / label names,
# Arguments with nargs='+' take a list of 1 or more
# ie '... --feature_names age sex credit_history ...'
parser.add_argument('--feature_names', nargs='+', required=True)

parser.add_argument('--label_names', nargs='+', required=True)

parser.add_argument('--favorable_label', type=float, required=True)

parser.add_argument('--unfavorable_label', type=float, required=True)

# type=json.loads parses the string from json to a python dict
parser.add_argument('--privileged_groups',
                    type=json.loads,
                    help='Privileged groups.',
                    nargs='+',
                    required=True)

parser.add_argument('--unprivileged_groups',
                    help='Unprivileged groups.',
                    type=json.loads,
                    nargs='+',
                    required=True)


args, _ = parser.parse_known_args()

if __name__ == "__main__":
    model = AIFModel(
        name=args.model_name,
        predictor_host=args.predictor_host,
        feature_names=args.feature_names,
        label_names=args.label_names,
        favorable_label=args.favorable_label,
        unfavorable_label=args.unfavorable_label,
        privileged_groups=args.privileged_groups,
        unprivileged_groups=args.unprivileged_groups
    )
    model.load()
    kserve.ModelServer().start([model], nest_asyncio=True)
