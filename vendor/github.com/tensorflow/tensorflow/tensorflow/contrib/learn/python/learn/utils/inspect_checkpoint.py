# Copyright 2016 The TensorFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ==============================================================================
"""A simple script for inspect checkpoint files (deprecated)."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import argparse
import sys

from tensorflow.contrib.framework.python.framework import checkpoint_utils
from tensorflow.python.platform import app

FLAGS = None


def print_tensors_in_checkpoint_file(file_name, tensor_name):
  """Prints tensors in a checkpoint file.

  If no `tensor_name` is provided, prints the tensor names and shapes
  in the checkpoint file.

  If `tensor_name` is provided, prints the content of the tensor.

  Args:
    file_name: Name of the checkpoint file.
    tensor_name: Name of the tensor in the checkpoint file to print.
  """
  try:
    if not tensor_name:
      variables = checkpoint_utils.list_variables(file_name)
      for name, shape in variables:
        print("%s\t%s" % (name, str(shape)))
    else:
      print("tensor_name: ", tensor_name)
      print(checkpoint_utils.load_variable(file_name, tensor_name))
  except Exception as e:  # pylint: disable=broad-except
    print(str(e))
    if "corrupted compressed block contents" in str(e):
      print("It's likely that your checkpoint file has been compressed "
            "with SNAPPY.")


def main(unused_argv):
  if not FLAGS.file_name:
    print("Usage: inspect_checkpoint --file_name=<checkpoint_file_name "
          "or directory> [--tensor_name=tensor_to_print]")
    sys.exit(1)
  else:
    print_tensors_in_checkpoint_file(FLAGS.file_name, FLAGS.tensor_name)


if __name__ == "__main__":
  parser = argparse.ArgumentParser()
  parser.register("type", "bool", lambda v: v.lower() == "true")
  parser.add_argument(
      "--file_name",
      type=str,
      default="",
      help="Checkpoint filename"
  )
  parser.add_argument(
      "--tensor_name",
      type=str,
      default="",
      help="Name of the tensor to inspect"
  )
  FLAGS, unparsed = parser.parse_known_args()
  app.run(main=main, argv=[sys.argv[0]] + unparsed)
