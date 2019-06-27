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
"""test the RunMetadata proto."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from collections import defaultdict

import six

from tensorflow.core.protobuf import config_pb2
from tensorflow.core.protobuf import rewriter_config_pb2
from tensorflow.python.client import session
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import test
from tensorflow.python.profiler import option_builder

# pylint: disable=g-bad-import-order
# XXX: this depends on pywrap_tensorflow and must come later
from tensorflow.python.profiler import model_analyzer
from tensorflow.python.profiler.internal import model_analyzer_testlib as lib

SIZE = 1300
builder = option_builder.ProfileOptionBuilder


def _extract_node(run_meta, node_name):
  ret = defaultdict(list)
  for dev_stat in run_meta.step_stats.dev_stats:
    dev = dev_stat.device.lower()
    if dev.find('cpu:') > 0:
      dev = dev[dev.find('cpu:'):]
    elif dev.find('gpu:') > 0:
      dev = dev[dev.find('gpu:'):]
    else:
      assert False, 'Unrecognized device name: %s' % dev

    for node_stat in dev_stat.node_stats:
      nname = node_stat.node_name
      if nname.find(':') > 0:
        nname = nname[:nname.find(':')]
      if nname == node_name:
        ret[dev].append(node_stat)
  return ret


def _run_model():
  x = random_ops.random_normal(shape=[1, SIZE])
  w = random_ops.random_normal(shape=[SIZE, 2 * SIZE])
  y = math_ops.matmul(x, w)

  config = config_pb2.ConfigProto()
  config.graph_options.rewrite_options.arithmetic_optimization = (
      rewriter_config_pb2.RewriterConfig.OFF)
  with session.Session(config=config) as sess:
    run_metadata = config_pb2.RunMetadata()
    opts = builder.time_and_memory()
    opts['min_micros'] = 0
    opts['min_bytes'] = 0
    opts['order_by'] = 'name'
    opts['output'] = 'none'
    _ = sess.run(y,
                 options=config_pb2.RunOptions(
                     trace_level=config_pb2.RunOptions.FULL_TRACE),
                 run_metadata=run_metadata)
    tfprof_node = model_analyzer.profile(
        sess.graph,
        run_meta=run_metadata,
        options=opts)

    return tfprof_node, run_metadata


def _run_loop_model():
  with session.Session() as sess:
    x = lib.BuildFullModel()

    sess.run(variables.global_variables_initializer())
    run_meta = config_pb2.RunMetadata()
    _ = sess.run(x,
                 options=config_pb2.RunOptions(
                     trace_level=config_pb2.RunOptions.FULL_TRACE),
                 run_metadata=run_meta)

    opts = builder.time_and_memory()
    opts['order_by'] = 'name'
    opts['output'] = 'none'

    tfprof_node = model_analyzer.profile(
        sess.graph, run_meta, options=opts)
    return tfprof_node, run_meta


class RunMetadataTest(test.TestCase):

  def testGPU(self):
    if not test.is_gpu_available(cuda_only=True):
      return

    gpu_dev = test.gpu_device_name()
    ops.reset_default_graph()
    with ops.device(gpu_dev):
      tfprof_node, run_meta = _run_model()
      self.assertEqual(tfprof_node.children[0].name, 'MatMul')
      self.assertGreater(tfprof_node.children[0].exec_micros, 10)

    ret = _extract_node(run_meta, 'MatMul')
    self.assertEqual(len(ret['gpu:0']), 1)
    self.assertEqual(len(ret['gpu:0/stream:all']), 1, '%s' % run_meta)

  def testAllocationHistory(self):
    if not test.is_gpu_available(cuda_only=True):
      return

    gpu_dev = test.gpu_device_name()
    ops.reset_default_graph()
    with ops.device(gpu_dev):
      _, run_meta = _run_model()

    mm = _extract_node(run_meta, 'MatMul')['gpu:0'][0]
    mm_allocs = mm.memory[0].allocation_records
    # has allocation and deallocation.
    self.assertEqual(len(mm_allocs), 2)
    # first allocated.
    self.assertGreater(mm_allocs[1].alloc_micros, mm_allocs[0].alloc_micros)
    self.assertGreater(mm_allocs[0].alloc_bytes, 0)
    # Then deallocated.
    self.assertLess(mm_allocs[1].alloc_bytes, 0)
    # All memory deallocated.
    self.assertEqual(mm_allocs[0].alloc_bytes + mm_allocs[1].alloc_bytes, 0)

    rand = _extract_node(
        run_meta, 'random_normal/RandomStandardNormal')['gpu:0'][0]
    random_allocs = rand.memory[0].allocation_records
    # random normal must allocated first since matmul depends on it.
    self.assertLess(random_allocs[0].alloc_micros, mm.all_start_micros)
    # deallocates the memory after matmul started.
    self.assertGreater(random_allocs[1].alloc_micros, mm.all_start_micros)

  @test_util.run_deprecated_v1
  def testCPU(self):
    ops.reset_default_graph()
    with ops.device('/cpu:0'):
      tfprof_node, run_meta = _run_model()
      self.assertEqual(tfprof_node.children[0].name, 'MatMul')
      self.assertGreater(tfprof_node.children[0].exec_micros, 0)

    ret = _extract_node(run_meta, 'MatMul')
    self.assertEqual(len(ret['cpu:0']), 1)

    ret = _extract_node(run_meta, 'MatMul:MatMul')
    self.assertEqual(len(ret), 0)

  @test_util.run_v1_only('b/120545219')
  def testLoopCPU(self):
    ops.reset_default_graph()
    with ops.device('/cpu:0'):
      tfprof_node, run_meta = _run_loop_model()
      # The while-loop caused a node to appear 4 times in scheduling.
      ret = _extract_node(run_meta,
                          'rnn/while/basic_rnn_cell/MatMul')
      self.assertEqual(len(ret['cpu:0']), 4)

      total_cpu_execs = 0
      for node in ret['cpu:0']:
        total_cpu_execs += node.op_end_rel_micros

      mm_node = lib.SearchTFProfNode(
          tfprof_node,
          'rnn/while/basic_rnn_cell/MatMul')

      self.assertEqual(mm_node.run_count, 4)
      self.assertEqual(mm_node.cpu_exec_micros, total_cpu_execs)
      self.assertEqual(mm_node.exec_micros, total_cpu_execs)

  def testGradientGraph(self):
    # Note: Please don't just adjust the test to make it pass.
    # The code view logic depends on it.
    ops.reset_default_graph()
    _, _ = _run_loop_model()
    graph = ops.get_default_graph()
    forward_op = set()
    backward_op = set()
    back_to_forward = dict()
    for op in graph.get_operations():
      if op.name.find('gradients/') > 0 and op.name.find('_grad/') > 0:
        backward_op.add(op.name)
        idx1 = op.name.find('gradients/') + 10
        idx2 = op.name.find('_grad/')
        back_to_forward[op.name] = op.name[idx1:idx2]
      else:
        forward_op.add(op.name)

    for _, f in six.iteritems(back_to_forward):
      self.assertTrue(f in forward_op)

  def testLoopGPU(self):
    if not test.is_gpu_available():
      return

    ops.reset_default_graph()
    with ops.device('/device:GPU:0'):
      _, run_meta = _run_loop_model()
      # The while-loop caused a node to appear 4 times in scheduling.
      ret = _extract_node(run_meta,
                          'rnn/while/basic_rnn_cell/MatMul')
      self.assertEqual(len(ret['gpu:0']), 4, '%s' % run_meta)

      total_cpu_execs = 0
      for node in ret['gpu:0']:
        total_cpu_execs += node.op_end_rel_micros

      self.assertGreaterEqual(len(ret['gpu:0/stream:all']), 4, '%s' % run_meta)


if __name__ == '__main__':
  test.main()
