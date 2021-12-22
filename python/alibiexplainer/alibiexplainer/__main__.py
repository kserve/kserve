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

import dill
import kserve
import logging
import os
import sys
from alibiexplainer import AlibiExplainer
from alibiexplainer.explainer import ExplainerMethod  # pylint:disable=no-name-in-module
from alibiexplainer.parser import parse_args

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)

EXPLAINER_FILENAME = "explainer.dill"


def main():
    args, extra = parse_args(sys.argv[1:])
    # Pretrained Alibi explainer

    alibi_model = None
    if args.storage_uri is not None:
        alibi_model = os.path.join(
            kserve.Storage.download(args.storage_uri), EXPLAINER_FILENAME
        )
        with open(alibi_model, "rb") as f:
            logging.info("Loading Alibi model")
            alibi_model = dill.load(f)

    explainer = AlibiExplainer(
        args.model_name,
        args.predictor_host,
        ExplainerMethod(args.command),
        extra,
        alibi_model,
    )
    explainer.load()
    kserve.ModelServer().start(models=[explainer], nest_asyncio=True)


if __name__ == "__main__":
    main()
