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


"""
Combined Transformer for sentiment analysis models
Handles both preprocessing (tokenization) and postprocessing (converting logits to sentiment labels)

Original example can be found here: https://github.com/brettmthompson/sentiment_transformer
"""

import argparse
import logging
from typing import Dict, List
import kserve
from kserve import InferRequest, InferInput, InferResponse, PredictorConfig
from transformers import AutoTokenizer
import numpy as np

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class SentimentTransformer(kserve.Model):
    def __init__(
        self,
        name: str,
        tokenizer_name: str,
        predictor_config: PredictorConfig,
        sentiment_labels: List[str] = None,
        max_length: int = 128,
        input_names: List[str] = None,
        output_name: str = "predict",
        include_star_rating: bool = False,
    ):
        super().__init__(name)

        # Store predictor config as instance variables (not relying on context)
        self.predictor_config_override = predictor_config

        # Validate tokenizer_name
        if not tokenizer_name:
            raise ValueError("tokenizer_name cannot be empty")

        # Validate max_length
        if max_length <= 0:
            raise ValueError("max_length must be a positive integer")

        # Validate output_name
        if not output_name:
            raise ValueError("output_name cannot be empty")

        # Set defaults and validate
        self.sentiment_labels = sentiment_labels or ["negative", "positive"]
        if len(self.sentiment_labels) < 2:
            raise ValueError(
                "sentiment_labels must contain at least 2 labels for classification"
            )

        self.input_names = input_names or ["input_ids", "attention_mask"]
        if not self.input_names:
            raise ValueError("input_names cannot be empty")

        self.tokenizer_name = tokenizer_name
        self.max_length = max_length
        self.output_name = output_name
        self.include_star_rating = include_star_rating
        self.ready = False

        # Load tokenizer
        try:
            logger.info(f"Loading tokenizer for {tokenizer_name}")
            self.tokenizer = AutoTokenizer.from_pretrained(tokenizer_name)
            self.ready = True
            logger.info("Tokenizer loaded successfully")
        except Exception as e:
            logger.error(f"Failed to load tokenizer: {e}")
            raise

    @property
    def predictor_config(self):
        """Override parent's predictor_config to return instance-based config instead of ContextVar"""
        return self.predictor_config_override

    def _tokenize(self, texts: List[str]) -> List[InferInput]:
        """Tokenize a list of texts and return V2 protocol InferInputs."""
        encoded = self.tokenizer(
            texts,
            padding=True,
            truncation=True,
            max_length=self.max_length,
            return_tensors="np",
        )

        infer_inputs = []
        for name in self.input_names:
            if name in encoded:
                infer_input = InferInput(
                    name=name,
                    shape=list(encoded[name].shape),
                    datatype="INT64",
                    data=encoded[name].tolist(),
                )
                infer_inputs.append(infer_input)
            else:
                logger.warning(
                    f"Input '{name}' not found in tokenizer output, skipping"
                )

        if not infer_inputs:
            raise ValueError(
                f"None of the specified input_names {self.input_names} found in tokenizer output"
            )

        logger.info(
            f"Prepared {len(infer_inputs)} inputs: {[inp.name for inp in infer_inputs]}"
        )
        return infer_inputs

    def preprocess(self, payload: Dict, headers: Dict = None) -> InferRequest:
        """
        Preprocess the input text by tokenizing it.

        Expected input format:
        {
            "texts": ["This is a great product!", "I'm very disappointed"]
        }

        Returns V2 protocol format.
        """
        try:
            logger.info(f"Received payload: {payload}")

            # Extract texts from payload
            texts = payload.get("texts", [])

            if not texts:
                raise ValueError("No texts found in payload")

            if not isinstance(texts, list):
                raise ValueError("'texts' must be a list of strings")

            logger.info(f"Processing {len(texts)} text(s)")

            infer_inputs = self._tokenize(texts)

            logger.info(f"Request Headers: {headers}")

            return InferRequest(model_name=self.name, infer_inputs=infer_inputs)

        except Exception as e:
            logger.error(f"Preprocessing failed: {e}")
            raise

    def postprocess(self, response: InferResponse, headers: Dict = None) -> Dict:
        """
        Postprocess the model outputs to extract sentiment labels and confidence scores.

        Expected input from predictor: InferResponse (V2 protocol)

        Returns:
        {
            "predictions": [
                {
                    "sentiment": "positive",
                    "confidence": 0.95,
                    "all_scores": {
                        "negative": 0.05,
                        "positive": 0.95
                    },
                    "star_rating": 5  # Only included if include_star_rating=True
                }
            ]
        }
        """
        try:
            logger.info(f"Received response from predictor: {response}")

            # Extract outputs from InferResponse
            outputs = response.outputs

            if not outputs:
                raise ValueError("No outputs found in predictor response")

            # Find the logits output by specified name
            output_data = None

            for output in outputs:
                if output.name == self.output_name and output.data:
                    output_data = output
                    logger.info(f"Found output '{self.output_name}'")
                    break

            if output_data is None:
                raise ValueError(
                    f"Output '{self.output_name}' not found in predictor response"
                )

            # Convert to numpy array and reshape using the shape from InferResponse
            predictions = np.array(output_data.data).reshape(output_data.shape)
            logger.info(f"Predictions shape: {predictions.shape}")

            # Validate predictions shape
            if len(predictions.shape) != 2:
                raise ValueError(
                    f"Expected 2D predictions (batch_size, num_classes), got shape {predictions.shape}"
                )

            # Process each prediction
            results = []

            for logits in predictions:
                # Convert logits to probabilities
                probs = self._softmax_1d(logits)

                # Get the predicted class
                predicted_idx = int(np.argmax(probs))
                confidence = float(probs[predicted_idx])

                # Map index to label
                # Protect against unconfigured sentiment classes
                sentiment_label = (
                    self.sentiment_labels[predicted_idx]
                    if predicted_idx < len(self.sentiment_labels)
                    else f"class_{predicted_idx}"
                )

                # Create all_scores dictionary
                all_scores = {
                    label: float(probs[i]) if i < len(probs) else 0.0
                    for i, label in enumerate(self.sentiment_labels)
                }

                result = {
                    "sentiment": sentiment_label,
                    "confidence": round(confidence, 4),
                    "all_scores": {k: round(v, 4) for k, v in all_scores.items()},
                }

                # Add star rating only if enabled and labels contain negative/positive
                if self.include_star_rating:
                    if self._has_negative_positive_labels():
                        result["star_rating"] = self._calculate_star_rating(
                            sentiment_label, confidence
                        )
                    else:
                        logger.warning(
                            f"Star rating requested but sentiment_labels {self.sentiment_labels} "
                            "do not contain 'negative' and 'positive' labels. Skipping star_rating."
                        )

                results.append(result)

            logger.info(f"Processed {len(results)} prediction(s)")

            return {"predictions": results}

        except Exception as e:
            logger.error(f"Postprocessing failed: {e}")
            raise

    @staticmethod
    def _softmax_1d(x: np.ndarray) -> np.ndarray:
        """Apply softmax to 1D array (single prediction) to convert logits to probabilities"""
        exp_x = np.exp(x - np.max(x))
        return exp_x / exp_x.sum()

    def _has_negative_positive_labels(self) -> bool:
        """Check if sentiment labels contain both 'negative' and 'positive'"""
        labels_lower = [label.lower() for label in self.sentiment_labels]
        has_negative = any(
            "negative" in label or "neg" in label for label in labels_lower
        )
        has_positive = any(
            "positive" in label or "pos" in label for label in labels_lower
        )
        return has_negative and has_positive

    def _calculate_star_rating(self, sentiment_label: str, confidence: float) -> int:
        """
        Calculate 1-5 star rating based on sentiment and confidence.
        """
        label_lower = sentiment_label.lower()

        if label_lower not in ["positive", "pos", "negative", "neg"]:
            raise ValueError(
                "star rating can be calculated from positive or negative labels only"
            )

        # Check for uncertain/neutral prediction (scores between 0.4-0.6)
        if 0.4 <= confidence <= 0.6:
            return 3

        # Determine if positive or negative
        is_positive = "positive" in label_lower or "pos" in label_lower

        # Map confidence to stars
        if is_positive:
            return 5 if confidence >= 0.8 else 4
        else:  # negative
            return 1 if confidence >= 0.8 else 2


def add_transformer_args(parser: argparse.ArgumentParser) -> None:
    """Add sentiment transformer CLI arguments to a parser."""
    parser.add_argument(
        "--tokenizer_name",
        help="Tokenizer name/path (HuggingFace Hub model ID or local directory path)",
        default="/app/tokenizer",
    )
    parser.add_argument(
        "--sentiment_labels",
        help="Comma-separated sentiment labels (must match model output classes)",
        default="negative,positive",
    )
    parser.add_argument(
        "--max_length",
        help="Maximum sequence length for tokenization",
        type=int,
        default=128,
    )
    parser.add_argument(
        "--input_names",
        help="Comma-separated list of tokenizer outputs to include (e.g., input_ids,attention_mask,token_type_ids)",
        default="input_ids,attention_mask",
    )
    parser.add_argument(
        "--output_name",
        help="Name of the model output to use for predictions",
        default="predict",
    )
    parser.add_argument(
        "--include_star_rating",
        help="Include star rating (1-5) in the response payload",
        action="store_true",
    )


def build_transformer(args) -> SentimentTransformer:
    """Build a SentimentTransformer from parsed CLI arguments."""
    labels = args.sentiment_labels.split(",")
    inputs = args.input_names.split(",")

    predictor_config = PredictorConfig(
        predictor_host=args.predictor_host,
        predictor_protocol="v2",
        predictor_use_ssl=args.predictor_use_ssl,
        predictor_request_timeout_seconds=args.predictor_request_timeout_seconds,
        predictor_request_retries=args.predictor_request_retries,
        predictor_health_check=args.enable_predictor_health_check,
    )

    return SentimentTransformer(
        name=args.model_name,
        tokenizer_name=args.tokenizer_name,
        predictor_config=predictor_config,
        sentiment_labels=labels,
        max_length=args.max_length,
        input_names=inputs,
        output_name=args.output_name,
        include_star_rating=args.include_star_rating,
    )
