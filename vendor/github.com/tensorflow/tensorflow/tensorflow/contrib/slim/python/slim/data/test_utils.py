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
"""Contains test utilities."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import os

import numpy as np

from tensorflow.core.example import example_pb2
from tensorflow.core.example import feature_pb2
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.lib.io import tf_record
from tensorflow.python.ops import image_ops


def _encoded_int64_feature(ndarray):
  return feature_pb2.Feature(int64_list=feature_pb2.Int64List(
      value=ndarray.flatten().tolist()))


def _encoded_bytes_feature(tf_encoded):
  encoded = tf_encoded.eval()

  def string_to_bytes(value):
    return feature_pb2.BytesList(value=[value])

  return feature_pb2.Feature(bytes_list=string_to_bytes(encoded))


def _string_feature(value):
  value = value.encode('utf-8')
  return feature_pb2.Feature(bytes_list=feature_pb2.BytesList(value=[value]))


def _encoder(image, image_format):
  assert image_format in ['jpeg', 'png']
  if image_format == 'jpeg':
    tf_image = constant_op.constant(image, dtype=dtypes.uint8)
    return image_ops.encode_jpeg(tf_image)
  if image_format == 'png':
    tf_image = constant_op.constant(image, dtype=dtypes.uint8)
    return image_ops.encode_png(tf_image)


def generate_image(image_shape, image_format='jpeg', label=0):
  """Generates an image and an example containing the encoded image.

  GenerateImage must be called within an active session.

  Args:
    image_shape: the shape of the image to generate.
    image_format: the encoding format of the image.
    label: the int64 labels for the image.

  Returns:
    image: the generated image.
    example: a TF-example with a feature key 'image/encoded' set to the
      serialized image and a feature key 'image/format' set to the image
      encoding format ['jpeg', 'png'].
  """
  image = np.random.random_integers(0, 255, size=image_shape)
  tf_encoded = _encoder(image, image_format)
  example = example_pb2.Example(features=feature_pb2.Features(feature={
      'image/encoded': _encoded_bytes_feature(tf_encoded),
      'image/format': _string_feature(image_format),
      'image/class/label': _encoded_int64_feature(np.array(label)),
  }))

  return image, example.SerializeToString()


def create_tfrecord_files(output_dir, num_files=3, num_records_per_file=10):
  """Creates TFRecords files.

  The method must be called within an active session.

  Args:
    output_dir: The directory where the files are stored.
    num_files: The number of files to create.
    num_records_per_file: The number of records per file.

  Returns:
    A list of the paths to the TFRecord files.
  """
  tfrecord_paths = []
  for i in range(num_files):
    path = os.path.join(output_dir,
                        'flowers.tfrecord-%d-of-%s' % (i, num_files))
    tfrecord_paths.append(path)

    writer = tf_record.TFRecordWriter(path)
    for _ in range(num_records_per_file):
      _, example = generate_image(image_shape=(10, 10, 3))
      writer.write(example)
    writer.close()

  return tfrecord_paths
