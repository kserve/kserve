import argparse
import logging
import kserve
from aixserver.explainer import ExplainerMethod

DEFAULT_MODEL_NAME = "aixserver"
# DEFAULT_NUM_SAMPLES = "1000"
DEFAULT_SEGMENTATION_ALGORITHM = "quickshift"
# DEFAULT_TOP_LABELS = "10"
# DEFAULT_MIN_WEIGHT = "0.01"
DEFAULT_POSITIVE_ONLY = "true"


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


def addCommonArgs(parser):
    parser.add_argument(
        '--num_samples',
        type=int,
        action=GroupedAction,
        dest="explainer.num_samples",
        default=argparse.SUPPRESS,
        help='The number of samples the explainer is allowed to take.')
    parser.add_argument(
        '--top_labels',
        type=int,
        action=GroupedAction,
        dest="explainer.top_labels",
        default=argparse.SUPPRESS,
        help='The number of most likely classifications to return.')


def parseArgs(sys_args):
    parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
    parser.add_argument(
        '--model_name',
        default=DEFAULT_MODEL_NAME,
        help='The name that the model is served under.'
    )
    parser.add_argument(
        '--predictor_host',
        help='The host for the predictor.',
        required=True)

    subparsers = parser.add_subparsers(help="sub-command help", dest="command")
    # Image Arg
    parser_image = subparsers.add_parser(str(ExplainerMethod.lime_images))
    parser_image.add_argument(
        '--segmentation_algorithm',
        action=GroupedAction,
        dest="explainer.segmentation_algorithm",
        default=DEFAULT_SEGMENTATION_ALGORITHM,
        help='The algorithm used for segmentation.'
    )
    parser_image.add_argument(
        '--min_weight',
        type=float,
        action=GroupedAction,
        dest="explainer.min_weight",
        default=argparse.SUPPRESS,
        help='The minimum weight needed by a pixel to be considered useful as an explanation.'
    )
    parser_image.add_argument(
        '--positive_only',
        type=str2bool,
        action=GroupedAction,
        dest="explainer.positive_only",
        default=argparse.SUPPRESS,
        help='Whether or not to show only the explanations that positively indicate a classification.'
    )
    addCommonArgs(parser_image)

    # Text Arg
    parser_text = subparsers.add_parser(str(ExplainerMethod.lime_text))

    addCommonArgs(parser_text)
    # Tabular Arg
    parser_tabular = subparsers.add_parser(str(ExplainerMethod.lime_tabular))
    # parser_tabular.add_argument(
    #     '--num_features',
    #     action=GroupedAction,
    #     dest="explainer.num_features",
    #     default=argparse.SUPPRESS,
    #     required=True
    # )
    # parser_tabular.add_argument(
    #     '--feature_names',
    #     action=GroupedAction,
    #     dest="explainer.feature_names",
    #     default=argparse.SUPPRESS,
    #     required=True
    # )
    # parser_tabular.add_argument(
    #     '--categorical_features',
    #     action=GroupedAction,
    #     dest="explainer.categorical_features",
    #     default=argparse.SUPPRESS,
    #     required=True
    # )
    # parser_tabular.add_argument(
    #     '--categorical_names',
    #     action=GroupedAction,
    #     dest="explainer.categorical_names",
    #     default=argparse.SUPPRESS,
    #     required=True
    # )
    # parser_tabular.add_argument(
    #     '--class_names',
    #     action=GroupedAction,
    #     dest="explainer.class_names",
    #     default=argparse.SUPPRESS,
    #     required=True
    # )
    # parser_tabular.add_argument(
    #     '--training_data',
    #     action=GroupedAction,
    #     dest="explainer.training_data",
    #     default=argparse.SUPPRESS,
    #     required=True
    # )
    addCommonArgs(parser_tabular)

    args, _ = parser.parse_known_args(sys_args)

    argdDict = vars(args).copy()
    if "explainer" in argdDict:
        extra = vars(args.explainer)
    else:
        extra = {}
    logging.info("Extra args: %s", extra)
    return args, extra
