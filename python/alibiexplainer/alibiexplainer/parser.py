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
import logging
import os
from alibiexplainer.explainer import ExplainerMethod  # pylint:disable=no-name-in-module

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)

DEFAULT_EXPLAINER_NAME = "explainer"
ENV_STORAGE_URI = "STORAGE_URI"


class GroupedAction(argparse.Action):  # pylint:disable=too-few-public-methods
    def __call__(self, theparser, namespace, values, option_string=None):
        group, dest = self.dest.split(".", 2)
        groupspace = getattr(namespace, group, argparse.Namespace())
        setattr(groupspace, dest, values)
        setattr(namespace, group, groupspace)


def str2bool(v):
    if isinstance(v, bool):
        return v
    if v.lower() in ("yes", "true", "t", "y", "1"):
        return True
    if v.lower() in ("no", "false", "f", "n", "0"):
        return False
    raise argparse.ArgumentTypeError("Boolean value expected.")


def addCommonParserArgs(parser):
    parser.add_argument(
        "--threshold",
        type=float,
        action=GroupedAction,
        dest="explainer.threshold",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--delta",
        type=float,
        action=GroupedAction,
        dest="explainer.delta",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--tau",
        type=float,
        action=GroupedAction,
        dest="explainer.tau",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--batch_size",
        type=int,
        action=GroupedAction,
        dest="explainer.batch_size",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--coverage_samples",
        type=int,
        action=GroupedAction,
        dest="explainer.coverage_samples",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--beam_size",
        type=int,
        action=GroupedAction,
        dest="explainer.beam_size",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--stop_on_first",
        type=str2bool,
        action=GroupedAction,
        dest="explainer.stop_on_first",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--max_anchor_size",
        type=int,
        action=GroupedAction,
        dest="explainer.max_anchor_size",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--max_samples_start",
        type=int,
        action=GroupedAction,
        dest="explainer.max_samples_start",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--n_covered_ex",
        type=int,
        action=GroupedAction,
        dest="explainer.n_covered_ex",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--binary_cache_size",
        type=int,
        action=GroupedAction,
        dest="explainer.binary_cache_size",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--cache_margin",
        type=int,
        action=GroupedAction,
        dest="explainer.cache_margin",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--verbose",
        type=str2bool,
        action=GroupedAction,
        dest="explainer.verbose",
        default=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--verbose_every",
        type=int,
        action=GroupedAction,
        dest="explainer.verbose_every",
        default=argparse.SUPPRESS,
    )


def parse_args(sys_args):
    parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
    parser.add_argument(
        "--model_name",
        default=DEFAULT_EXPLAINER_NAME,
        help="The name of model explainer.",
    )
    parser.add_argument(
        "--predictor_host", help="The host for the predictor", required=True
    )
    parser.add_argument(
        "--storage_uri",
        help="The URI of a pretrained explainer",
        default=os.environ.get(ENV_STORAGE_URI),
    )
    subparsers = parser.add_subparsers(help="sub-command help", dest="command")

    # Anchor Tabular Arguments
    parser_anchor_tabular = subparsers.add_parser(str(ExplainerMethod.anchor_tabular))
    addCommonParserArgs(parser_anchor_tabular)

    # Anchor Text Arguments
    parser_anchor_text = subparsers.add_parser(str(ExplainerMethod.anchor_text))
    parser_anchor_text.add_argument(
        "--use_unk",
        type=str2bool,
        action=GroupedAction,
        dest="explainer.use_unk",
        default=argparse.SUPPRESS,
    )
    parser_anchor_text.add_argument(
        "--use_similarity_proba",
        type=str2bool,
        action=GroupedAction,
        dest="explainer.use_similarity_proba",
        default=argparse.SUPPRESS,
    )
    parser_anchor_text.add_argument(
        "--sample_proba",
        type=float,
        action=GroupedAction,
        dest="explainer.sample_proba",
        default=argparse.SUPPRESS,
    )
    parser_anchor_text.add_argument(
        "--top_n",
        type=int,
        action=GroupedAction,
        dest="explainer.top_n",
        default=argparse.SUPPRESS,
    )
    parser_anchor_text.add_argument(
        "--temperature",
        type=float,
        action=GroupedAction,
        dest="explainer.temperature",
        default=argparse.SUPPRESS,
    )
    addCommonParserArgs(parser_anchor_text)

    # Anchor Images Arguments
    parser_anchor_images = subparsers.add_parser(str(ExplainerMethod.anchor_images))
    parser_anchor_images.add_argument(
        "--p_sample",
        type=float,
        action=GroupedAction,
        dest="explainer.p_sample",
        default=argparse.SUPPRESS,
    )
    addCommonParserArgs(parser_anchor_images)

    args, _ = parser.parse_known_args(sys_args)

    argdDict = vars(args).copy()
    if "explainer" in argdDict:
        extra = vars(args.explainer)
    else:
        extra = {}
    logging.info("Extra args: %s", extra)
    return args, extra
