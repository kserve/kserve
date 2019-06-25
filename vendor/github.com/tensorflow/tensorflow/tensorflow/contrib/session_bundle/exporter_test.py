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
"""Tests for exporter.py."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import os.path

from tensorflow.contrib.session_bundle import constants
from tensorflow.contrib.session_bundle import exporter
from tensorflow.contrib.session_bundle import gc
from tensorflow.contrib.session_bundle import manifest_pb2
from tensorflow.core.framework import graph_pb2
from tensorflow.core.protobuf import config_pb2
from tensorflow.core.protobuf import saver_pb2
from tensorflow.python.client import session
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import state_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import flags
from tensorflow.python.platform import gfile
from tensorflow.python.platform import test
from tensorflow.python.training import saver

FLAGS = flags.FLAGS

GLOBAL_STEP = 222


def tearDownModule():
  gfile.DeleteRecursively(test.get_temp_dir())


class SaveRestoreShardedTest(test.TestCase):

  def doBasicsOneExportPath(self,
                            export_path,
                            clear_devices=False,
                            global_step=GLOBAL_STEP,
                            sharded=True,
                            export_count=1):
    # Build a graph with 2 parameter nodes on different devices.
    ops.reset_default_graph()
    with session.Session(
        target="",
        config=config_pb2.ConfigProto(device_count={"CPU": 2})) as sess:
      # v2 is an unsaved variable derived from v0 and v1.  It is used to
      # exercise the ability to run an init op when restoring a graph.
      with sess.graph.device("/cpu:0"):
        v0 = variables.VariableV1(10, name="v0")
      with sess.graph.device("/cpu:1"):
        v1 = variables.VariableV1(20, name="v1")
      v2 = variables.VariableV1(1, name="v2", trainable=False, collections=[])
      assign_v2 = state_ops.assign(v2, math_ops.add(v0, v1))
      init_op = control_flow_ops.group(assign_v2, name="init_op")

      ops.add_to_collection("v", v0)
      ops.add_to_collection("v", v1)
      ops.add_to_collection("v", v2)

      named_tensor_bindings = {"logical_input_A": v0, "logical_input_B": v1}
      signatures = {
          "foo":
              exporter.regression_signature(
                  input_tensor=v0, output_tensor=v1),
          "generic":
              exporter.generic_signature(named_tensor_bindings)
      }

      asset_filepath_orig = os.path.join(test.get_temp_dir(), "hello42.txt")
      asset_file = constant_op.constant(asset_filepath_orig, name="filename42")
      ops.add_to_collection(ops.GraphKeys.ASSET_FILEPATHS, asset_file)

      with gfile.FastGFile(asset_filepath_orig, "w") as f:
        f.write("your data here")
      assets_collection = ops.get_collection(ops.GraphKeys.ASSET_FILEPATHS)

      ignored_asset = os.path.join(test.get_temp_dir(), "ignored.txt")
      with gfile.FastGFile(ignored_asset, "w") as f:
        f.write("additional data here")

      variables.global_variables_initializer().run()

      # Run an export.
      save = saver.Saver(
          {
              "v0": v0,
              "v1": v1
          },
          restore_sequentially=True,
          sharded=sharded,
          write_version=saver_pb2.SaverDef.V1)
      export = exporter.Exporter(save)
      compare_def = ops.get_default_graph().as_graph_def()
      export.init(
          compare_def,
          init_op=init_op,
          clear_devices=clear_devices,
          default_graph_signature=exporter.classification_signature(
              input_tensor=v0),
          named_graph_signatures=signatures,
          assets_collection=assets_collection)

      for x in range(export_count):
        export.export(
            export_path,
            constant_op.constant(global_step + x),
            sess,
            exports_to_keep=gc.largest_export_versions(2))
      # Set global_step to the last exported version, as the rest of the test
      # uses it to construct model export path, loads model from it, and does
      # verifications. We want to make sure to always use the last exported
      # version, as old ones may have be garbage-collected.
      global_step += export_count - 1

    # Restore graph.
    ops.reset_default_graph()
    with session.Session(
        target="",
        config=config_pb2.ConfigProto(device_count={"CPU": 2})) as sess:
      save = saver.import_meta_graph(
          os.path.join(export_path, constants.VERSION_FORMAT_SPECIFIER %
                       global_step, constants.META_GRAPH_DEF_FILENAME))
      self.assertIsNotNone(save)
      meta_graph_def = save.export_meta_graph()
      collection_def = meta_graph_def.collection_def

      # Validate custom graph_def.
      graph_def_any = collection_def[constants.GRAPH_KEY].any_list.value
      self.assertEquals(len(graph_def_any), 1)
      graph_def = graph_pb2.GraphDef()
      graph_def_any[0].Unpack(graph_def)
      if clear_devices:
        for node in compare_def.node:
          node.device = ""
      self.assertProtoEquals(compare_def, graph_def)

      # Validate init_op.
      init_ops = collection_def[constants.INIT_OP_KEY].node_list.value
      self.assertEquals(len(init_ops), 1)
      self.assertEquals(init_ops[0], "init_op")

      # Validate signatures.
      signatures_any = collection_def[constants.SIGNATURES_KEY].any_list.value
      self.assertEquals(len(signatures_any), 1)
      signatures = manifest_pb2.Signatures()
      signatures_any[0].Unpack(signatures)
      default_signature = signatures.default_signature
      self.assertEqual(
          default_signature.classification_signature.input.tensor_name, "v0:0")
      bindings = signatures.named_signatures["generic"].generic_signature.map
      self.assertEquals(bindings["logical_input_A"].tensor_name, "v0:0")
      self.assertEquals(bindings["logical_input_B"].tensor_name, "v1:0")
      read_foo_signature = (
          signatures.named_signatures["foo"].regression_signature)
      self.assertEquals(read_foo_signature.input.tensor_name, "v0:0")
      self.assertEquals(read_foo_signature.output.tensor_name, "v1:0")

      # Validate the assets.
      assets_any = collection_def[constants.ASSETS_KEY].any_list.value
      self.assertEquals(len(assets_any), 1)
      asset = manifest_pb2.AssetFile()
      assets_any[0].Unpack(asset)
      assets_path = os.path.join(export_path,
                                 constants.VERSION_FORMAT_SPECIFIER %
                                 global_step, constants.ASSETS_DIRECTORY,
                                 "hello42.txt")
      asset_contents = gfile.GFile(assets_path).read()
      self.assertEqual(asset_contents, "your data here")
      self.assertEquals("hello42.txt", asset.filename)
      self.assertEquals("filename42:0", asset.tensor_binding.tensor_name)
      ignored_asset_path = os.path.join(export_path,
                                        constants.VERSION_FORMAT_SPECIFIER %
                                        global_step, constants.ASSETS_DIRECTORY,
                                        "ignored.txt")
      self.assertFalse(gfile.Exists(ignored_asset_path))

      # Validate graph restoration.
      if sharded:
        save.restore(sess,
                     os.path.join(export_path,
                                  constants.VERSION_FORMAT_SPECIFIER %
                                  global_step,
                                  constants.VARIABLES_FILENAME_PATTERN))
      else:
        save.restore(sess,
                     os.path.join(export_path,
                                  constants.VERSION_FORMAT_SPECIFIER %
                                  global_step, constants.VARIABLES_FILENAME))
      self.assertEqual(10, ops.get_collection("v")[0].eval())
      self.assertEqual(20, ops.get_collection("v")[1].eval())
      ops.get_collection(constants.INIT_OP_KEY)[0].run()
      self.assertEqual(30, ops.get_collection("v")[2].eval())

  def testDuplicateExportRaisesError(self):
    export_path = os.path.join(test.get_temp_dir(), "export_duplicates")
    self.doBasicsOneExportPath(export_path)
    self.assertRaises(RuntimeError, self.doBasicsOneExportPath, export_path)

  def testBasics(self):
    export_path = os.path.join(test.get_temp_dir(), "export")
    self.doBasicsOneExportPath(export_path)

  def testBasicsNoShard(self):
    export_path = os.path.join(test.get_temp_dir(), "export_no_shard")
    self.doBasicsOneExportPath(export_path, sharded=False)

  def testClearDevice(self):
    export_path = os.path.join(test.get_temp_dir(), "export_clear_device")
    self.doBasicsOneExportPath(export_path, clear_devices=True)

  def testGC(self):
    export_path = os.path.join(test.get_temp_dir(), "gc")
    self.doBasicsOneExportPath(export_path, global_step=100)
    self.assertEquals(gfile.ListDirectory(export_path), ["00000100"])
    self.doBasicsOneExportPath(export_path, global_step=101)
    self.assertEquals(
        sorted(gfile.ListDirectory(export_path)), ["00000100", "00000101"])
    self.doBasicsOneExportPath(export_path, global_step=102)
    self.assertEquals(
        sorted(gfile.ListDirectory(export_path)), ["00000101", "00000102"])

  def testExportMultipleTimes(self):
    export_path = os.path.join(test.get_temp_dir(), "export_multiple_times")
    self.doBasicsOneExportPath(export_path, export_count=10)


if __name__ == "__main__":
  test.main()
