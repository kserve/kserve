# Copyright 2026 The KServe Authors.
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

"""Pass-through transformer for testing the normal ISVC HTTP tracing path."""

import argparse

from kserve import Model, ModelServer, logging, model_server


class TraceTransformer(Model):
    def __init__(self, name: str):
        super().__init__(name)
        # Inherit Model.predict: tracing must use the production inference client,
        # not a test-specific HTTP client or manually injected trace headers.
        self.ready = True


def main():
    parser = argparse.ArgumentParser(parents=[model_server.parser])
    args, _ = parser.parse_known_args()
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    ModelServer().start([TraceTransformer(args.model_name)])


if __name__ == "__main__":
    main()
