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
gRPC entrypoint for the SentimentTransformer.

Original example can be found here: https://github.com/brettmthompson/sentiment_transformer
"""

import argparse
import logging
from typing import Dict
from kserve import (
    InferRequest,
    ModelServer,
    model_server,
    logging as kserve_logging,
)
from custom_transformer.sentiment_transformer import (
    SentimentTransformer,
    add_transformer_args,
)

logger = logging.getLogger(__name__)


class SentimentTransformerGrpc(SentimentTransformer):
    """gRPC variant of SentimentTransformer.

    In gRPC mode, preprocess receives an InferRequest directly (V2 protocol)
    rather than a V1 JSON dict. The request is expected to contain a single
    input with BYTES datatype carrying the raw text strings.
    """

    def preprocess(
        self, request: InferRequest, headers: Dict[str, str] = None
    ) -> InferRequest:
        """Extract texts from a V2 InferRequest and tokenize them."""
        try:
            texts = [
                t.decode("utf-8") if isinstance(t, bytes) else str(t)
                for t in request.inputs[0].data
            ]

            if not texts:
                raise ValueError("No texts found in gRPC request")

            logger.info(f"Processing {len(texts)} text(s) via gRPC")

            infer_inputs = self._tokenize(texts)
            return InferRequest(model_name=self.name, infer_inputs=infer_inputs)

        except Exception as e:
            logger.error(f"gRPC preprocessing failed: {e}")
            raise


def build_grpc_transformer(args) -> SentimentTransformerGrpc:
    """Build a SentimentTransformerGrpc from parsed CLI arguments."""
    from kserve import PredictorConfig

    labels = args.sentiment_labels.split(",")
    inputs = args.input_names.split(",")

    predictor_config = PredictorConfig(
        predictor_host=args.predictor_host,
        predictor_protocol="grpc-v2",
        predictor_use_ssl=args.predictor_use_ssl,
        predictor_request_timeout_seconds=args.predictor_request_timeout_seconds,
        predictor_request_retries=args.predictor_request_retries,
        predictor_health_check=args.enable_predictor_health_check,
    )

    return SentimentTransformerGrpc(
        name=args.model_name,
        tokenizer_name=args.tokenizer_name,
        predictor_config=predictor_config,
        sentiment_labels=labels,
        max_length=args.max_length,
        input_names=inputs,
        output_name=args.output_name,
        include_star_rating=args.include_star_rating,
    )


parser = argparse.ArgumentParser(parents=[model_server.parser])
add_transformer_args(parser)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        kserve_logging.configure_logging(args.log_config_file)
    transformer = build_grpc_transformer(args)
    ModelServer(workers=1).start([transformer])
