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
"""Exports a toy linear regression inference graph.

Exports a TensorFlow graph to /tmp/half_plus_two/ based on the Exporter
format.

This graph calculates,
  y = a*x + b
where a and b are variables with a=0.5 and b=2.

Output from this program is typically used to exercise Session
loading and execution code.
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import argparse
import sys

import tensorflow as tf

from tensorflow.contrib.session_bundle import exporter

FLAGS = None


def Export(export_dir, use_checkpoint_v2):
  with tf.Session() as sess:
    # Make model parameters a&b variables instead of constants to
    # exercise the variable reloading mechanisms.
    a = tf.Variable(0.5, name="a")
    b = tf.Variable(2.0, name="b")

    # Create a placeholder for serialized tensorflow.Example messages to be fed.
    serialized_tf_example = tf.placeholder(tf.string, name="tf_example")

    # Parse the tensorflow.Example looking for a feature named "x" with a single
    # floating point value.
    feature_configs = {"x": tf.FixedLenFeature([1], dtype=tf.float32),}
    tf_example = tf.parse_example(serialized_tf_example, feature_configs)
    # Use tf.identity() to assign name
    x = tf.identity(tf_example["x"], name="x")

    # Calculate, y = a*x + b
    y = tf.add(tf.multiply(a, x), b, name="y")

    # Setup a standard Saver for our variables.
    save = tf.train.Saver(
        {
            "a": a,
            "b": b
        },
        sharded=True,
        write_version=tf.train.SaverDef.V2 if use_checkpoint_v2 else
        tf.train.SaverDef.V1)

    # asset_path contains the base directory of assets used in training (e.g.
    # vocabulary files).
    original_asset_path = tf.constant("/tmp/original/export/assets")
    # Ops reading asset files should reference the asset_path tensor
    # which stores the original asset path at training time and the
    # overridden assets directory at restore time.
    asset_path = tf.Variable(original_asset_path,
                             name="asset_path",
                             trainable=False,
                             collections=[])
    assign_asset_path = asset_path.assign(original_asset_path)

    # Use a fixed global step number.
    global_step_tensor = tf.Variable(123, name="global_step")

    # Create a RegressionSignature for our input and output.
    regression_signature = exporter.regression_signature(
        input_tensor=serialized_tf_example,
        # Use tf.identity here because we export two signatures here.
        # Otherwise only graph for one of the signatures will be loaded
        # (whichever is created first) during serving.
        output_tensor=tf.identity(y))
    named_graph_signature = {
        "inputs": exporter.generic_signature({"x": x}),
        "outputs": exporter.generic_signature({"y": y})
    }

    # Create two filename assets and corresponding tensors.
    # TODO(b/26254158) Consider adding validation of file existence as well as
    # hashes (e.g. sha1) for consistency.
    original_filename1 = tf.constant("hello1.txt")
    tf.add_to_collection(tf.GraphKeys.ASSET_FILEPATHS, original_filename1)
    filename1 = tf.Variable(original_filename1,
                            name="filename1",
                            trainable=False,
                            collections=[])
    assign_filename1 = filename1.assign(original_filename1)
    original_filename2 = tf.constant("hello2.txt")
    tf.add_to_collection(tf.GraphKeys.ASSET_FILEPATHS, original_filename2)
    filename2 = tf.Variable(original_filename2,
                            name="filename2",
                            trainable=False,
                            collections=[])
    assign_filename2 = filename2.assign(original_filename2)

    # Init op contains a group of all variables that we assign.
    init_op = tf.group(assign_asset_path, assign_filename1, assign_filename2)

    # CopyAssets is used as a callback during export to copy files to the
    # given export directory.
    def CopyAssets(filepaths, export_path):
      print("copying asset files to: %s" % export_path)
      for filepath in filepaths:
        print("copying asset file: %s" % filepath)

    # Run an export.
    tf.global_variables_initializer().run()
    export = exporter.Exporter(save)
    export.init(
        sess.graph.as_graph_def(),
        init_op=init_op,
        default_graph_signature=regression_signature,
        named_graph_signatures=named_graph_signature,
        assets_collection=tf.get_collection(tf.GraphKeys.ASSET_FILEPATHS),
        assets_callback=CopyAssets)
    export.export(export_dir, global_step_tensor, sess)


def main(_):
  Export(FLAGS.export_dir, FLAGS.use_checkpoint_v2)


if __name__ == "__main__":
  parser = argparse.ArgumentParser()
  parser.register("type", "bool", lambda v: v.lower() == "true")
  parser.add_argument(
      "--export_dir",
      type=str,
      default="/tmp/half_plus_two",
      help="Directory where to export inference model."
  )
  parser.add_argument(
      "--use_checkpoint_v2",
      type="bool",
      nargs="?",
      const=True,
      default=False,
      help="If true, write v2 checkpoint files.")
  FLAGS, unparsed = parser.parse_known_args()
  tf.app.run(main=main, argv=[sys.argv[0]] + unparsed)
