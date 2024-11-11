# Copyright 2023 The KServe Authors.
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
from typing import Dict, Union

import numpy as np
import tokenization
import data_processing

import kserve
from kserve import (
    InferRequest,
    InferResponse,
    InferInput,
    model_server,
    ModelServer,
    logging,
)
from kserve.model import PredictorConfig


class Tokenizer(kserve.Model):
    def __init__(
        self,
        name: str,
        predictor_config: PredictorConfig,
    ):
        super().__init__(name, predictor_config)
        self.short_paragraph_text = (
            "The Apollo program was the third United States human spaceflight program. "
            "First conceived as a three-man spacecraft to follow the one-man Project Mercury "
            "which put the first Americans in space, Apollo was dedicated to President"
            " John F. Kennedy's national goal of landing a man on the Moon. The first manned "
            "flight of Apollo was in 1968. Apollo ran from 1961 to 1972 followed by "
            "the Apollo-Soyuz Test Project a joint Earth orbit mission with "
            "the Soviet Union in 1975."
        )
        self.tokenizer = tokenization.FullTokenizer(
            vocab_file=args.vocab_file, do_lower_case=True
        )
        self.ready = True

    def preprocess(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferRequest]:
        self.doc_tokens = data_processing.convert_doc_tokens(self.short_paragraph_text)
        self.features = data_processing.convert_examples_to_features(
            self.doc_tokens, payload["instances"][0], self.tokenizer, 128, 128, 64
        )
        unique_ids = np.zeros([1, 1], dtype=np.int32)
        segment_ids = self.features["segment_ids"].reshape(1, 128)
        input_ids = self.features["input_ids"].reshape(1, 128)
        input_mask = self.features["input_mask"].reshape(1, 128)
        infer_inputs = [
            InferInput(
                name="unique_ids",
                datatype="INT32",
                shape=list(unique_ids.shape),
                data=unique_ids,
            ),
            InferInput(
                name="segment_ids",
                datatype="INT32",
                shape=list(segment_ids.shape),
                data=segment_ids,
            ),
            InferInput(
                name="input_ids",
                datatype="INT32",
                shape=list(input_ids.shape),
                data=input_ids,
            ),
            InferInput(
                name="input_mask",
                datatype="INT32",
                shape=list(input_mask.shape),
                data=input_mask,
            ),
        ]
        return InferRequest(model_name=self.name, infer_inputs=infer_inputs)

    def postprocess(
        self, infer_response: Union[Dict, InferResponse], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:

        end_logits = infer_response.outputs[0].data
        start_logits = infer_response.outputs[1].data
        n_best_size = 20

        # The maximum length of an answer that can be generated. This is needed
        #  because the start and end predictions are not conditioned on one another
        max_answer_length = 30

        (prediction, nbest_json, scores_diff_json) = data_processing.get_predictions(
            self.doc_tokens,
            self.features,
            start_logits,
            end_logits,
            n_best_size,
            max_answer_length,
        )

        return {"predictions": prediction, "prob": nbest_json[0]["probability"] * 100.0}


parser = argparse.ArgumentParser(parents=[model_server.parser])
parser.add_argument("--vocab_file", help="The name of the vocab file.")
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = Tokenizer(
        args.model_name,
        PredictorConfig(
            args.predictor_host,
            args.predictor_protocol,
            args.predictor_use_ssl,
            args.predictor_request_timeout_seconds,
            args.predictor_request_retries,
            args.enable_predictor_health_check,
        ),
    )
    ModelServer().start([model])
