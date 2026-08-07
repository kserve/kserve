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


"""
REST entrypoint for the SentimentTransformer.

Original example can be found here: https://github.com/brettmthompson/sentiment_transformer
"""

import argparse
import kserve
from custom_transformer.sentiment_transformer import (
    add_transformer_args,
    build_transformer,
)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
    add_transformer_args(parser)
    args, _ = parser.parse_known_args()

    transformer = build_transformer(args)
    kserve.ModelServer().start([transformer])
