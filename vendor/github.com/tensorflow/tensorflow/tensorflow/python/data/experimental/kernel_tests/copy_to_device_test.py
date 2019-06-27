# Copyright 2017 The TensorFlow Authors. All Rights Reserved.
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
"""Tests for `tf.data.experimental.copy_to_device()`."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.core.protobuf import config_pb2
from tensorflow.python.compat import compat
from tensorflow.python.data.experimental.ops import prefetching_ops
from tensorflow.python.data.kernel_tests import test_base
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.data.ops import iterator_ops
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import test_util
from tensorflow.python.ops import math_ops
from tensorflow.python.platform import test
from tensorflow.python.util import compat as util_compat


# TODO(b/117581999): add eager coverage when supported.
class CopyToDeviceTest(test_base.DatasetTestBase):

  @test_util.run_deprecated_v1
  def testCopyToDevice(self):
    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1"))

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)
    self.assertEqual([], next_element.shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyToDeviceInt32(self):
    host_dataset = dataset_ops.Dataset.from_tensors([0, 1, 2, 3])
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1"))

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int32, next_element.dtype)
    self.assertEqual((4,), next_element.shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      self.assertAllEqual([0, 1, 2, 3], self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyToSameDevice(self):
    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:0"))

    with ops.device("/cpu:0"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)
    self.assertEqual([], next_element.shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyToDeviceWithPrefetch(self):
    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1")).prefetch(1)

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)
    self.assertEqual([], next_element.shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyDictToDevice(self):
    host_dataset = dataset_ops.Dataset.range(10).map(lambda x: {"a": x})
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1"))

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element["a"].dtype)
    self.assertEqual([], next_element["a"].shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        self.assertEqual({"a": i}, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyDictToDeviceWithPrefetch(self):
    host_dataset = dataset_ops.Dataset.range(10).map(lambda x: {"a": x})
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1")).prefetch(1)

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element["a"].dtype)
    self.assertEqual([], next_element["a"].shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        self.assertEqual({"a": i}, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopySparseTensorsToDevice(self):

    def make_tensor(i):
      return sparse_tensor.SparseTensorValue(
          indices=[[0, 0]], values=(i * [1]), dense_shape=[2, 2])

    host_dataset = dataset_ops.Dataset.range(10).map(make_tensor)

    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1"))

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        actual = self.evaluate(next_element)
        self.assertAllEqual([i], actual.values)
        self.assertAllEqual([[0, 0]], actual.indices)
        self.assertAllEqual([2, 2], actual.dense_shape)
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopySparseTensorsToDeviceWithPrefetch(self):

    def make_tensor(i):
      return sparse_tensor.SparseTensorValue(
          indices=[[0, 0]], values=(i * [1]), dense_shape=[2, 2])

    host_dataset = dataset_ops.Dataset.range(10).map(make_tensor)

    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1")).prefetch(1)

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_one_shot_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      for i in range(10):
        actual = self.evaluate(next_element)
        self.assertAllEqual([i], actual.values)
        self.assertAllEqual([[0, 0]], actual.indices)
        self.assertAllEqual([2, 2], actual.dense_shape)
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpu(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuWithPrefetch(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0")).prefetch(1)

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuWithMap(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    def generator():
      for i in range(10):
        yield i, float(i), str(i)

    host_dataset = dataset_ops.Dataset.from_generator(
        generator, output_types=(dtypes.int32, dtypes.float32, dtypes.string))
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))

    def gpu_map_func(x, y, z):
      return math_ops.square(x), math_ops.square(y), z

    device_dataset = device_dataset.apply(
        prefetching_ops.map_on_gpu(gpu_map_func))
    options = dataset_ops.Options()
    options.experimental_autotune = False
    device_dataset = device_dataset.with_options(options)

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      for i in range(10):
        x, y, z = self.evaluate(next_element)
        self.assertEqual(i**2, x)
        self.assertEqual(float(i**2), y)
        self.assertEqual(util_compat.as_bytes(str(i)), z)
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuInt32(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.from_tensors([0, 1, 2, 3])
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      self.assertAllEqual([0, 1, 2, 3], self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuInt32AndPrefetch(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.from_tensors([0, 1, 2, 3])
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0")).prefetch(1)

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      self.assertAllEqual([0, 1, 2, 3], self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuStrings(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.from_tensors(["a", "b", "c"])
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      self.assertAllEqual([b"a", b"b", b"c"], self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuStringsAndPrefetch(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.from_tensors(["a", "b", "c"])
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      self.assertAllEqual([b"a", b"b", b"c"], self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDevicePingPongCPUGPU(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    with compat.forward_compatibility_horizon(2018, 8, 4):
      host_dataset = dataset_ops.Dataset.range(10)
      device_dataset = host_dataset.apply(
          prefetching_ops.copy_to_device("/gpu:0", source_device="/cpu:0"))
      back_to_cpu_dataset = device_dataset.apply(
          prefetching_ops.copy_to_device("/cpu:0", source_device="/gpu:0"))

      with ops.device("/cpu:0"):
        iterator = dataset_ops.make_initializable_iterator(back_to_cpu_dataset)
        next_element = iterator.get_next()

      with self.cached_session(
          config=config_pb2.ConfigProto(allow_soft_placement=False)):
        self.evaluate(iterator.initializer)
        for i in range(10):
          self.assertEqual(i, self.evaluate(next_element))
        with self.assertRaises(errors.OutOfRangeError):
          self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyToDeviceWithReInit(self):
    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1"))

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)
    self.assertEqual([], next_element.shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      self.evaluate(iterator.initializer)
      for i in range(5):
        self.assertEqual(i, self.evaluate(next_element))
      self.evaluate(iterator.initializer)
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  @test_util.run_deprecated_v1
  def testCopyToDeviceWithReInitAndPrefetch(self):
    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/cpu:1")).prefetch(1)

    with ops.device("/cpu:1"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    self.assertEqual(host_dataset.output_types, device_dataset.output_types)
    self.assertEqual(host_dataset.output_types, iterator.output_types)
    self.assertEqual(host_dataset.output_shapes, device_dataset.output_shapes)
    self.assertEqual(host_dataset.output_shapes, iterator.output_shapes)
    self.assertEqual(host_dataset.output_classes, device_dataset.output_classes)
    self.assertEqual(host_dataset.output_classes, iterator.output_classes)

    self.assertEqual(dtypes.int64, next_element.dtype)
    self.assertEqual([], next_element.shape)

    worker_config = config_pb2.ConfigProto(device_count={"CPU": 2})
    with self.test_session(config=worker_config):
      self.evaluate(iterator.initializer)
      for i in range(5):
        self.assertEqual(i, self.evaluate(next_element))
      self.evaluate(iterator.initializer)
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuWithReInit(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      for i in range(5):
        self.assertEqual(i, self.evaluate(next_element))
      self.evaluate(iterator.initializer)
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testCopyToDeviceGpuWithReInitAndPrefetch(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.range(10)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0")).prefetch(1)

    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_element = iterator.get_next()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      self.evaluate(iterator.initializer)
      for i in range(5):
        self.assertEqual(i, self.evaluate(next_element))
      self.evaluate(iterator.initializer)
      for i in range(10):
        self.assertEqual(i, self.evaluate(next_element))
      with self.assertRaises(errors.OutOfRangeError):
        self.evaluate(next_element)

  def testIteratorGetNextAsOptionalOnGPU(self):
    if not test_util.is_gpu_available():
      self.skipTest("No GPU available")

    host_dataset = dataset_ops.Dataset.range(3)
    device_dataset = host_dataset.apply(
        prefetching_ops.copy_to_device("/gpu:0"))
    with ops.device("/gpu:0"):
      iterator = dataset_ops.make_initializable_iterator(device_dataset)
      next_elem = iterator_ops.get_next_as_optional(iterator)
      elem_has_value_t = next_elem.has_value()
      elem_value_t = next_elem.get_value()

    with self.cached_session(
        config=config_pb2.ConfigProto(allow_soft_placement=False)):
      # Before initializing the iterator, evaluating the optional fails with
      # a FailedPreconditionError.
      with self.assertRaises(errors.FailedPreconditionError):
        self.evaluate(elem_has_value_t)
      with self.assertRaises(errors.FailedPreconditionError):
        self.evaluate(elem_value_t)

      # For each element of the dataset, assert that the optional evaluates to
      # the expected value.
      self.evaluate(iterator.initializer)
      for i in range(3):
        elem_has_value, elem_value = self.evaluate(
            [elem_has_value_t, elem_value_t])
        self.assertTrue(elem_has_value)
        self.assertEqual(i, elem_value)

      # After exhausting the iterator, `next_elem.has_value()` will evaluate to
      # false, and attempting to get the value will fail.
      for _ in range(2):
        self.assertFalse(self.evaluate(elem_has_value_t))
        with self.assertRaises(errors.InvalidArgumentError):
          self.evaluate(elem_value_t)


if __name__ == "__main__":
  test.main()
