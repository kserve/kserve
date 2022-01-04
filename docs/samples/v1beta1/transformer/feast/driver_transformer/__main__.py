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
from .driver_transformer import DriverTransformer

import logging
logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)

DEFAULT_MODEL_NAME = "sklearn-driver-transformer"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--predictor_host",
    help="The URL for the model predict function", required=True
)
parser.add_argument(
    "--model_name", default=DEFAULT_MODEL_NAME,
    help='The name that the model is served under.')
parser.add_argument(
    "--feast_serving_url",
    type=str,
    help="The url of the Feast feature server.", required=True)
parser.add_argument(
    "--entity_ids",
    type=str, nargs="+",
    help="A list of entity ids to use as keys in the feature store.",
    required=True)
parser.add_argument(
    "--feature_refs",
    type=str, nargs="+",
    help="A list of features to retrieve from the feature store.",
    required=True)


args, _ = parser.parse_known_args()

if __name__ == "__main__":
    transformer = DriverTransformer(
        name=args.model_name,
        predictor_host=args.predictor_host,
        feast_serving_url=args.feast_serving_url,
        entity_ids=args.entity_ids,
        feature_refs=args.feature_refs)
    server = kserve.ModelServer()
    server.start(models=[transformer])
