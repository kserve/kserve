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

from alibiexplainer.parser import parse_args

PREDICTOR_HOST = "0.0.0.0:5000"
THRESHOLD = 0.9
DELTA = 0.2
TAU = 0.1
BATCH_SIZE = 100
COVERAGE_SAMPLES = 2
BEAM_SIZE = 10
STOP_ON_FIRST = True
MAX_ANCHOR_SIZE = 9
MAX_SAMPLES_START = 500
N_COVERED_EX = 2
BINARY_CACHE_SIZE = 256
CACHE_MARGIN = 75
VERBOSE = True
VERBOSE_EVERY = 2


def test_basic_args():
    args = ["--predictor_host", PREDICTOR_HOST]
    parser, _ = parse_args(args)
    assert parser.predictor_host == PREDICTOR_HOST


def test_shared_explainer_args():
    args = [
        "--predictor_host",
        PREDICTOR_HOST,
        "AnchorTabular",
        "--threshold",
        str(THRESHOLD),
        "--delta",
        str(DELTA),
        "--tau",
        str(TAU),
        "--batch_size",
        str(BATCH_SIZE),
        "--coverage_samples",
        str(COVERAGE_SAMPLES),
        "--beam_size",
        str(BEAM_SIZE),
        "--stop_on_first",
        str(STOP_ON_FIRST),
        "--max_anchor_size",
        str(MAX_ANCHOR_SIZE),
        "--max_samples_start",
        str(MAX_SAMPLES_START),
        "--n_covered_ex",
        str(N_COVERED_EX),
        "--binary_cache_size",
        str(BINARY_CACHE_SIZE),
        "--cache_margin",
        str(CACHE_MARGIN),
        "--verbose",
        str(VERBOSE),
        "--verbose_every",
        str(VERBOSE_EVERY),
    ]
    parser, _ = parse_args(args)
    assert parser.explainer.threshold == THRESHOLD
    assert parser.explainer.delta == DELTA
    assert parser.explainer.tau == TAU
    assert parser.explainer.batch_size == BATCH_SIZE
    assert parser.explainer.coverage_samples == COVERAGE_SAMPLES
    assert parser.explainer.beam_size == BEAM_SIZE
    assert parser.explainer.stop_on_first == STOP_ON_FIRST
    assert parser.explainer.max_anchor_size == MAX_ANCHOR_SIZE
    assert parser.explainer.max_samples_start == MAX_SAMPLES_START
    assert parser.explainer.n_covered_ex == N_COVERED_EX
    assert parser.explainer.binary_cache_size == BINARY_CACHE_SIZE
    assert parser.explainer.cache_margin == CACHE_MARGIN
    assert parser.explainer.verbose == VERBOSE
    assert parser.explainer.verbose_every == VERBOSE_EVERY


USE_UNK = True
USE_SIMILARITY_PROBA = True
SAMPLE_PROBA = 0.6
TOP_N = 4
TEMPERATURE = 0.2


def test_anchor_text_parser():
    args = [
        "--predictor_host",
        PREDICTOR_HOST,
        "AnchorText",
        "--use_unk",
        str(USE_UNK),
        "--use_similarity_proba",
        str(USE_SIMILARITY_PROBA),
        "--sample_proba",
        str(SAMPLE_PROBA),
        "--top_n",
        str(TOP_N),
        "--temperature",
        str(TEMPERATURE),
    ]
    parser, _ = parse_args(args)
    assert parser.predictor_host == PREDICTOR_HOST
    assert parser.explainer.use_unk == USE_UNK
    assert parser.explainer.use_similarity_proba == USE_SIMILARITY_PROBA
    assert parser.explainer.sample_proba == SAMPLE_PROBA
    assert parser.explainer.top_n == TOP_N
    assert parser.explainer.temperature == TEMPERATURE


P_SAMPLE = 0.15


def test_anchor_images_parser():
    args = [
        "--predictor_host",
        PREDICTOR_HOST,
        "AnchorImages",
        "--p_sample",
        str(P_SAMPLE),
    ]
    parser, _ = parse_args(args)
    assert parser.predictor_host == PREDICTOR_HOST
    assert parser.explainer.p_sample == P_SAMPLE
