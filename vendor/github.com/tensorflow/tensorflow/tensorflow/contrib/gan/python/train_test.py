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
"""Tests for gan.python.train."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from absl.testing import parameterized
import numpy as np

from tensorflow.contrib import layers
from tensorflow.contrib.framework.python.ops import variables as variables_lib
from tensorflow.contrib.gan.python import namedtuples
from tensorflow.contrib.gan.python import train
from tensorflow.contrib.gan.python.features.python import random_tensor_pool
from tensorflow.contrib.slim.python.slim import learning as slim_learning
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import random_seed
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.ops import variables
from tensorflow.python.ops.distributions import categorical
from tensorflow.python.platform import test
from tensorflow.python.training import basic_session_run_hooks
from tensorflow.python.training import coordinator
from tensorflow.python.training import gradient_descent
from tensorflow.python.training import sync_replicas_optimizer
from tensorflow.python.training import training_util


def generator_model(inputs):
  return variable_scope.get_variable('dummy_g', initializer=2.0) * inputs


class Generator(object):

  def __call__(self, inputs):
    return generator_model(inputs)


def infogan_generator_model(inputs):
  return variable_scope.get_variable('dummy_g', initializer=2.0) * inputs[0]


class InfoGANGenerator(object):

  def __call__(self, inputs):
    return infogan_generator_model(inputs)


def discriminator_model(inputs, _):
  return variable_scope.get_variable('dummy_d', initializer=2.0) * inputs


class Discriminator(object):

  def __call__(self, inputs, _):
    return discriminator_model(inputs, _)


def infogan_discriminator_model(inputs, _):
  return (variable_scope.get_variable('dummy_d', initializer=2.0) * inputs,
          [categorical.Categorical([1.0])])


class InfoGANDiscriminator(object):

  def __call__(self, inputs, _):
    return infogan_discriminator_model(inputs, _)


def acgan_discriminator_model(inputs, _, num_classes=10):
  return (
      discriminator_model(inputs, _),
      array_ops.one_hot(
          # TODO(haeusser): infer batch size from input
          random_ops.random_uniform(
              [3], maxval=num_classes, dtype=dtypes.int32),
          num_classes))


class ACGANDiscriminator(object):

  def __call__(self, inputs, _, num_classes=10):
    return (
        discriminator_model(inputs, _),
        array_ops.one_hot(
            # TODO(haeusser): infer batch size from input
            random_ops.random_uniform(
                [3], maxval=num_classes, dtype=dtypes.int32),
            num_classes))


def stargan_generator_model(inputs, _):
  """Dummy generator for StarGAN."""

  return variable_scope.get_variable('dummy_g', initializer=0.5) * inputs


class StarGANGenerator(object):

  def __call__(self, inputs, _):
    return stargan_generator_model(inputs, _)


def stargan_discriminator_model(inputs, num_domains):
  """Differentiable dummy discriminator for StarGAN."""

  hidden = layers.flatten(inputs)

  output_src = math_ops.reduce_mean(hidden, axis=1)

  output_cls = layers.fully_connected(
      inputs=hidden,
      num_outputs=num_domains,
      activation_fn=None,
      normalizer_fn=None,
      biases_initializer=None)
  return output_src, output_cls


class StarGANDiscriminator(object):

  def __call__(self, inputs, num_domains):
    return stargan_discriminator_model(inputs, num_domains)


def get_gan_model():
  # TODO(joelshor): Find a better way of creating a variable scope.
  with variable_scope.variable_scope('generator') as gen_scope:
    pass
  with variable_scope.variable_scope('discriminator') as dis_scope:
    pass
  return namedtuples.GANModel(
      generator_inputs=None,
      generated_data=None,
      generator_variables=None,
      generator_scope=gen_scope,
      generator_fn=generator_model,
      real_data=array_ops.ones([1, 2, 3]),
      discriminator_real_outputs=array_ops.ones([1, 2, 3]),
      discriminator_gen_outputs=array_ops.ones([1, 2, 3]),
      discriminator_variables=None,
      discriminator_scope=dis_scope,
      discriminator_fn=discriminator_model)


def get_callable_gan_model():
  ganmodel = get_gan_model()
  return ganmodel._replace(
      generator_fn=Generator(), discriminator_fn=Discriminator())


def create_gan_model():
  return train.gan_model(
      generator_model,
      discriminator_model,
      real_data=array_ops.zeros([1, 2]),
      generator_inputs=random_ops.random_normal([1, 2]))


def create_callable_gan_model():
  return train.gan_model(
      Generator(),
      Discriminator(),
      real_data=array_ops.zeros([1, 2]),
      generator_inputs=random_ops.random_normal([1, 2]))


def get_infogan_model():
  return namedtuples.InfoGANModel(
      *get_gan_model(),
      structured_generator_inputs=[constant_op.constant(0)],
      predicted_distributions=[categorical.Categorical([1.0])],
      discriminator_and_aux_fn=infogan_discriminator_model)


def get_callable_infogan_model():
  return namedtuples.InfoGANModel(
      *get_callable_gan_model(),
      structured_generator_inputs=[constant_op.constant(0)],
      predicted_distributions=[categorical.Categorical([1.0])],
      discriminator_and_aux_fn=infogan_discriminator_model)


def create_infogan_model():
  return train.infogan_model(
      infogan_generator_model,
      infogan_discriminator_model,
      real_data=array_ops.zeros([1, 2]),
      unstructured_generator_inputs=[],
      structured_generator_inputs=[random_ops.random_normal([1, 2])])


def create_callable_infogan_model():
  return train.infogan_model(
      InfoGANGenerator(),
      InfoGANDiscriminator(),
      real_data=array_ops.zeros([1, 2]),
      unstructured_generator_inputs=[],
      structured_generator_inputs=[random_ops.random_normal([1, 2])])


def get_acgan_model():
  return namedtuples.ACGANModel(
      *get_gan_model(),
      one_hot_labels=array_ops.one_hot([0, 1, 2], 10),
      discriminator_real_classification_logits=array_ops.one_hot([0, 1, 3], 10),
      discriminator_gen_classification_logits=array_ops.one_hot([0, 1, 4], 10))


def get_callable_acgan_model():
  return namedtuples.ACGANModel(
      *get_callable_gan_model(),
      one_hot_labels=array_ops.one_hot([0, 1, 2], 10),
      discriminator_real_classification_logits=array_ops.one_hot([0, 1, 3], 10),
      discriminator_gen_classification_logits=array_ops.one_hot([0, 1, 4], 10))


def create_acgan_model():
  return train.acgan_model(
      generator_model,
      acgan_discriminator_model,
      real_data=array_ops.zeros([1, 2]),
      generator_inputs=random_ops.random_normal([1, 2]),
      one_hot_labels=array_ops.one_hot([0, 1, 2], 10))


def create_callable_acgan_model():
  return train.acgan_model(
      Generator(),
      ACGANDiscriminator(),
      real_data=array_ops.zeros([1, 2]),
      generator_inputs=random_ops.random_normal([1, 2]),
      one_hot_labels=array_ops.one_hot([0, 1, 2], 10))


def get_cyclegan_model():
  return namedtuples.CycleGANModel(
      model_x2y=get_gan_model(),
      model_y2x=get_gan_model(),
      reconstructed_x=array_ops.ones([1, 2, 3]),
      reconstructed_y=array_ops.zeros([1, 2, 3]))


def get_callable_cyclegan_model():
  return namedtuples.CycleGANModel(
      model_x2y=get_callable_gan_model(),
      model_y2x=get_callable_gan_model(),
      reconstructed_x=array_ops.ones([1, 2, 3]),
      reconstructed_y=array_ops.zeros([1, 2, 3]))


def create_cyclegan_model():
  return train.cyclegan_model(
      generator_model,
      discriminator_model,
      data_x=array_ops.zeros([1, 2]),
      data_y=array_ops.ones([1, 2]))


def create_callable_cyclegan_model():
  return train.cyclegan_model(
      Generator(),
      Discriminator(),
      data_x=array_ops.zeros([1, 2]),
      data_y=array_ops.ones([1, 2]))


def get_stargan_model():
  """Similar to get_gan_model()."""
  # TODO(joelshor): Find a better way of creating a variable scope.
  with variable_scope.variable_scope('generator') as gen_scope:
    pass
  with variable_scope.variable_scope('discriminator') as dis_scope:
    pass
  return namedtuples.StarGANModel(
      input_data=array_ops.ones([1, 2, 2, 3]),
      input_data_domain_label=array_ops.ones([1, 2]),
      generated_data=array_ops.ones([1, 2, 2, 3]),
      generated_data_domain_target=array_ops.ones([1, 2]),
      reconstructed_data=array_ops.ones([1, 2, 2, 3]),
      discriminator_input_data_source_predication=array_ops.ones([1]),
      discriminator_generated_data_source_predication=array_ops.ones([1]),
      discriminator_input_data_domain_predication=array_ops.ones([1, 2]),
      discriminator_generated_data_domain_predication=array_ops.ones([1, 2]),
      generator_variables=None,
      generator_scope=gen_scope,
      generator_fn=stargan_generator_model,
      discriminator_variables=None,
      discriminator_scope=dis_scope,
      discriminator_fn=stargan_discriminator_model)


def get_callable_stargan_model():
  model = get_stargan_model()
  return model._replace(
      generator_fn=StarGANGenerator(), discriminator_fn=StarGANDiscriminator())


def create_stargan_model():
  return train.stargan_model(
      stargan_generator_model, stargan_discriminator_model,
      array_ops.ones([1, 2, 2, 3]), array_ops.ones([1, 2]))


def create_callable_stargan_model():
  return train.stargan_model(StarGANGenerator(), StarGANDiscriminator(),
                             array_ops.ones([1, 2, 2, 3]),
                             array_ops.ones([1, 2]))


def get_sync_optimizer():
  return sync_replicas_optimizer.SyncReplicasOptimizer(
      gradient_descent.GradientDescentOptimizer(learning_rate=1.0),
      replicas_to_aggregate=1)


class GANModelTest(test.TestCase, parameterized.TestCase):
  """Tests for `gan_model`."""

  @parameterized.named_parameters(
      ('gan', get_gan_model, namedtuples.GANModel),
      ('callable_gan', get_callable_gan_model, namedtuples.GANModel),
      ('infogan', get_infogan_model, namedtuples.InfoGANModel),
      ('callable_infogan', get_callable_infogan_model,
       namedtuples.InfoGANModel),
      ('acgan', get_acgan_model, namedtuples.ACGANModel),
      ('callable_acgan', get_callable_acgan_model, namedtuples.ACGANModel),
      ('cyclegan', get_cyclegan_model, namedtuples.CycleGANModel),
      ('callable_cyclegan', get_callable_cyclegan_model,
       namedtuples.CycleGANModel),
      ('stargan', get_stargan_model, namedtuples.StarGANModel),
      ('callabel_stargan', get_callable_stargan_model, namedtuples.StarGANModel)
  )
  def test_output_type(self, create_fn, expected_tuple_type):
    """Test that output type is as expected."""
    self.assertIsInstance(create_fn(), expected_tuple_type)

  def test_no_shape_check(self):

    def dummy_generator_model(_):
      return (None, None)

    def dummy_discriminator_model(data, conditioning):  # pylint: disable=unused-argument
      return 1

    with self.assertRaisesRegexp(AttributeError, 'object has no attribute'):
      train.gan_model(
          dummy_generator_model,
          dummy_discriminator_model,
          real_data=array_ops.zeros([1, 2]),
          generator_inputs=array_ops.zeros([1]),
          check_shapes=True)
    train.gan_model(
        dummy_generator_model,
        dummy_discriminator_model,
        real_data=array_ops.zeros([1, 2]),
        generator_inputs=array_ops.zeros([1]),
        check_shapes=False)


class StarGANModelTest(test.TestCase):
  """Tests for `stargan_model`."""

  @staticmethod
  def create_input_and_label_tensor(batch_size, img_size, c_size, num_domains):
    input_tensor_list = []
    label_tensor_list = []
    for _ in range(num_domains):
      input_tensor_list.append(
          random_ops.random_uniform((batch_size, img_size, img_size, c_size)))
      domain_idx = random_ops.random_uniform(
          [batch_size], minval=0, maxval=num_domains, dtype=dtypes.int32)
      label_tensor_list.append(array_ops.one_hot(domain_idx, num_domains))
    return input_tensor_list, label_tensor_list

  def test_generate_stargan_random_domain_target(self):
    batch_size = 8
    domain_numbers = 3

    target_tensor = train._generate_stargan_random_domain_target(
        batch_size, domain_numbers)

    with self.cached_session() as sess:
      targets = sess.run(target_tensor)
      self.assertTupleEqual((batch_size, domain_numbers), targets.shape)
      for target in targets:
        self.assertEqual(1, np.sum(target))
        self.assertEqual(1, np.max(target))

  def test_stargan_model_output_type(self):
    batch_size = 2
    img_size = 16
    c_size = 3
    num_domains = 5

    input_tensor, label_tensor = StarGANModelTest.create_input_and_label_tensor(
        batch_size, img_size, c_size, num_domains)
    model = train.stargan_model(
        generator_fn=stargan_generator_model,
        discriminator_fn=stargan_discriminator_model,
        input_data=input_tensor,
        input_data_domain_label=label_tensor)

    self.assertIsInstance(model, namedtuples.StarGANModel)
    self.assertTrue(isinstance(model.discriminator_variables, list))
    self.assertTrue(isinstance(model.generator_variables, list))
    self.assertIsInstance(model.discriminator_scope,
                          variable_scope.VariableScope)
    self.assertTrue(model.generator_scope, variable_scope.VariableScope)
    self.assertTrue(callable(model.discriminator_fn))
    self.assertTrue(callable(model.generator_fn))

  def test_stargan_model_generator_output(self):
    batch_size = 2
    img_size = 16
    c_size = 3
    num_domains = 5

    input_tensor, label_tensor = StarGANModelTest.create_input_and_label_tensor(
        batch_size, img_size, c_size, num_domains)
    model = train.stargan_model(
        generator_fn=stargan_generator_model,
        discriminator_fn=stargan_discriminator_model,
        input_data=input_tensor,
        input_data_domain_label=label_tensor)

    with self.test_session(use_gpu=True) as sess:

      sess.run(variables.global_variables_initializer())

      input_data, generated_data, reconstructed_data = sess.run(
          [model.input_data, model.generated_data, model.reconstructed_data])
      self.assertTupleEqual(
          (batch_size * num_domains, img_size, img_size, c_size),
          input_data.shape)
      self.assertTupleEqual(
          (batch_size * num_domains, img_size, img_size, c_size),
          generated_data.shape)
      self.assertTupleEqual(
          (batch_size * num_domains, img_size, img_size, c_size),
          reconstructed_data.shape)

  def test_stargan_model_discriminator_output(self):
    batch_size = 2
    img_size = 16
    c_size = 3
    num_domains = 5

    input_tensor, label_tensor = StarGANModelTest.create_input_and_label_tensor(
        batch_size, img_size, c_size, num_domains)
    model = train.stargan_model(
        generator_fn=stargan_generator_model,
        discriminator_fn=stargan_discriminator_model,
        input_data=input_tensor,
        input_data_domain_label=label_tensor)

    with self.test_session(use_gpu=True) as sess:

      sess.run(variables.global_variables_initializer())

      disc_input_data_source_pred, disc_gen_data_source_pred = sess.run([
          model.discriminator_input_data_source_predication,
          model.discriminator_generated_data_source_predication
      ])
      self.assertEqual(1, len(disc_input_data_source_pred.shape))
      self.assertEqual(batch_size * num_domains,
                       disc_input_data_source_pred.shape[0])
      self.assertEqual(1, len(disc_gen_data_source_pred.shape))
      self.assertEqual(batch_size * num_domains,
                       disc_gen_data_source_pred.shape[0])

      input_label, disc_input_label, gen_label, disc_gen_label = sess.run([
          model.input_data_domain_label,
          model.discriminator_input_data_domain_predication,
          model.generated_data_domain_target,
          model.discriminator_generated_data_domain_predication
      ])
      self.assertTupleEqual((batch_size * num_domains, num_domains),
                            input_label.shape)
      self.assertTupleEqual((batch_size * num_domains, num_domains),
                            disc_input_label.shape)
      self.assertTupleEqual((batch_size * num_domains, num_domains),
                            gen_label.shape)
      self.assertTupleEqual((batch_size * num_domains, num_domains),
                            disc_gen_label.shape)


class GANLossTest(test.TestCase, parameterized.TestCase):
  """Tests for `gan_loss`."""

  @parameterized.named_parameters(
      ('gan', get_gan_model),
      ('callable_gan', get_callable_gan_model),
      ('infogan', get_infogan_model),
      ('callable_infogan', get_callable_infogan_model),
      ('acgan', get_acgan_model),
      ('callable_acgan', get_callable_acgan_model),
  )
  def test_output_type(self, get_gan_model_fn):
    """Test output type."""
    loss = train.gan_loss(get_gan_model_fn(), add_summaries=True)
    self.assertIsInstance(loss, namedtuples.GANLoss)
    self.assertNotEmpty(ops.get_collection(ops.GraphKeys.SUMMARIES))

  @parameterized.named_parameters(
      ('cyclegan', create_cyclegan_model),
      ('callable_cyclegan', create_callable_cyclegan_model),
  )
  def test_cyclegan_output_type(self, get_gan_model_fn):
    loss = train.cyclegan_loss(get_gan_model_fn(), add_summaries=True)
    self.assertIsInstance(loss, namedtuples.CycleGANLoss)
    self.assertNotEmpty(ops.get_collection(ops.GraphKeys.SUMMARIES))

  @parameterized.named_parameters(
      ('gan', create_gan_model, False),
      ('gan_one_sided', create_gan_model, True),
      ('callable_gan', create_callable_gan_model, False),
      ('callable_gan_one_sided', create_callable_gan_model, True),
      ('infogan', create_infogan_model, False),
      ('infogan_one_sided', create_infogan_model, True),
      ('callable_infogan', create_callable_infogan_model, False),
      ('callable_infogan_one_sided', create_callable_infogan_model, True),
      ('acgan', create_acgan_model, False),
      ('acgan_one_sided', create_acgan_model, True),
      ('callable_acgan', create_callable_acgan_model, False),
      ('callable_acgan_one_sided', create_callable_acgan_model, True),
  )
  def test_grad_penalty(self, create_gan_model_fn, one_sided):
    """Test gradient penalty option."""
    model = create_gan_model_fn()
    loss = train.gan_loss(model)
    loss_gp = train.gan_loss(
        model,
        gradient_penalty_weight=1.0,
        gradient_penalty_one_sided=one_sided)
    self.assertIsInstance(loss_gp, namedtuples.GANLoss)

    # Check values.
    with self.test_session(use_gpu=True) as sess:
      variables.global_variables_initializer().run()
      loss_gen_np, loss_gen_gp_np = sess.run(
          [loss.generator_loss, loss_gp.generator_loss])
      loss_dis_np, loss_dis_gp_np = sess.run(
          [loss.discriminator_loss, loss_gp.discriminator_loss])

    self.assertEqual(loss_gen_np, loss_gen_gp_np)
    self.assertLess(loss_dis_np, loss_dis_gp_np)

  @parameterized.named_parameters(
      ('infogan', get_infogan_model),
      ('callable_infogan', get_callable_infogan_model),
  )
  def test_mutual_info_penalty(self, create_gan_model_fn):
    """Test mutual information penalty option."""
    train.gan_loss(
        create_gan_model_fn(),
        mutual_information_penalty_weight=constant_op.constant(1.0))

  @parameterized.named_parameters(
      ('gan', get_gan_model),
      ('callable_gan', get_callable_gan_model),
      ('infogan', get_infogan_model),
      ('callable_infogan', get_callable_infogan_model),
      ('acgan', get_acgan_model),
      ('callable_acgan', get_callable_acgan_model),
  )
  def test_regularization_helper(self, get_gan_model_fn):
    """Test regularization loss."""
    # Evaluate losses without regularization.
    no_reg_loss = train.gan_loss(get_gan_model_fn())
    with self.test_session(use_gpu=True):
      no_reg_loss_gen_np = no_reg_loss.generator_loss.eval()
      no_reg_loss_dis_np = no_reg_loss.discriminator_loss.eval()

    with ops.name_scope(get_gan_model_fn().generator_scope.name):
      ops.add_to_collection(ops.GraphKeys.REGULARIZATION_LOSSES,
                            constant_op.constant(3.0))
    with ops.name_scope(get_gan_model_fn().discriminator_scope.name):
      ops.add_to_collection(ops.GraphKeys.REGULARIZATION_LOSSES,
                            constant_op.constant(2.0))

    # Check that losses now include the correct regularization values.
    reg_loss = train.gan_loss(get_gan_model_fn())
    with self.test_session(use_gpu=True):
      reg_loss_gen_np = reg_loss.generator_loss.eval()
      reg_loss_dis_np = reg_loss.discriminator_loss.eval()

    self.assertEqual(3.0, reg_loss_gen_np - no_reg_loss_gen_np)
    self.assertEqual(2.0, reg_loss_dis_np - no_reg_loss_dis_np)

  @parameterized.named_parameters(
      ('notcallable', create_acgan_model),
      ('callable', create_callable_acgan_model),
  )
  def test_acgan(self, create_gan_model_fn):
    """Test that ACGAN models work."""
    model = create_gan_model_fn()
    loss = train.gan_loss(model)
    loss_ac_gen = train.gan_loss(model, aux_cond_generator_weight=1.0)
    loss_ac_dis = train.gan_loss(model, aux_cond_discriminator_weight=1.0)
    self.assertIsInstance(loss, namedtuples.GANLoss)
    self.assertIsInstance(loss_ac_gen, namedtuples.GANLoss)
    self.assertIsInstance(loss_ac_dis, namedtuples.GANLoss)

    # Check values.
    with self.test_session(use_gpu=True) as sess:
      variables.global_variables_initializer().run()
      loss_gen_np, loss_ac_gen_gen_np, loss_ac_dis_gen_np = sess.run([
          loss.generator_loss, loss_ac_gen.generator_loss,
          loss_ac_dis.generator_loss
      ])
      loss_dis_np, loss_ac_gen_dis_np, loss_ac_dis_dis_np = sess.run([
          loss.discriminator_loss, loss_ac_gen.discriminator_loss,
          loss_ac_dis.discriminator_loss
      ])

    self.assertLess(loss_gen_np, loss_dis_np)
    self.assertTrue(np.isscalar(loss_ac_gen_gen_np))
    self.assertTrue(np.isscalar(loss_ac_dis_gen_np))
    self.assertTrue(np.isscalar(loss_ac_gen_dis_np))
    self.assertTrue(np.isscalar(loss_ac_dis_dis_np))

  @parameterized.named_parameters(
      ('notcallable', create_cyclegan_model),
      ('callable', create_callable_cyclegan_model),
  )
  def test_cyclegan(self, create_gan_model_fn):
    """Test that CycleGan models work."""
    model = create_gan_model_fn()
    loss = train.cyclegan_loss(model)
    self.assertIsInstance(loss, namedtuples.CycleGANLoss)

    # Check values.
    with self.test_session(use_gpu=True) as sess:
      variables.global_variables_initializer().run()
      (loss_x2y_gen_np, loss_x2y_dis_np, loss_y2x_gen_np,
       loss_y2x_dis_np) = sess.run([
           loss.loss_x2y.generator_loss, loss.loss_x2y.discriminator_loss,
           loss.loss_y2x.generator_loss, loss.loss_y2x.discriminator_loss
       ])

    self.assertGreater(loss_x2y_gen_np, loss_x2y_dis_np)
    self.assertGreater(loss_y2x_gen_np, loss_y2x_dis_np)
    self.assertTrue(np.isscalar(loss_x2y_gen_np))
    self.assertTrue(np.isscalar(loss_x2y_dis_np))
    self.assertTrue(np.isscalar(loss_y2x_gen_np))
    self.assertTrue(np.isscalar(loss_y2x_dis_np))

  @parameterized.named_parameters(
      ('notcallable', create_stargan_model),
      ('callable', create_callable_stargan_model),
  )
  def test_stargan(self, create_gan_model_fn):

    model = create_gan_model_fn()
    model_loss = train.stargan_loss(model)

    self.assertIsInstance(model_loss, namedtuples.GANLoss)

    with self.cached_session() as sess:

      sess.run(variables.global_variables_initializer())

      gen_loss, disc_loss = sess.run(
          [model_loss.generator_loss, model_loss.discriminator_loss])

      self.assertTrue(np.isscalar(gen_loss))
      self.assertTrue(np.isscalar(disc_loss))

  @parameterized.named_parameters(
      ('gan', create_gan_model),
      ('callable_gan', create_callable_gan_model),
      ('infogan', create_infogan_model),
      ('callable_infogan', create_callable_infogan_model),
      ('acgan', create_acgan_model),
      ('callable_acgan', create_callable_acgan_model),
  )
  def test_tensor_pool(self, create_gan_model_fn):
    """Test tensor pool option."""
    model = create_gan_model_fn()
    tensor_pool_fn = lambda x: random_tensor_pool.tensor_pool(x, pool_size=5)
    loss = train.gan_loss(model, tensor_pool_fn=tensor_pool_fn)
    self.assertIsInstance(loss, namedtuples.GANLoss)

    # Check values.
    with self.test_session(use_gpu=True) as sess:
      variables.global_variables_initializer().run()
      for _ in range(10):
        sess.run([loss.generator_loss, loss.discriminator_loss])

  def test_discriminator_only_sees_pool(self):
    """Checks that discriminator only sees pooled values."""
    def checker_gen_fn(_):
      return constant_op.constant(0.0)
    model = train.gan_model(
        checker_gen_fn,
        discriminator_model,
        real_data=array_ops.zeros([]),
        generator_inputs=random_ops.random_normal([]))
    def tensor_pool_fn(_):
      return (random_ops.random_uniform([]), random_ops.random_uniform([]))
    def checker_dis_fn(inputs, _):
      """Discriminator that checks that it only sees pooled Tensors."""
      self.assertFalse(constant_op.is_constant(inputs))
      return inputs
    model = model._replace(
        discriminator_fn=checker_dis_fn)
    train.gan_loss(model, tensor_pool_fn=tensor_pool_fn)

  def test_doesnt_crash_when_in_nested_scope(self):
    with variable_scope.variable_scope('outer_scope'):
      gan_model = train.gan_model(
          generator_model,
          discriminator_model,
          real_data=array_ops.zeros([1, 2]),
          generator_inputs=random_ops.random_normal([1, 2]))

      # This should work inside a scope.
      train.gan_loss(gan_model, gradient_penalty_weight=1.0)

    # This should also work outside a scope.
    train.gan_loss(gan_model, gradient_penalty_weight=1.0)


class TensorPoolAdjusteModelTest(test.TestCase):

  def _check_tensor_pool_adjusted_model_outputs(
      self, tensor1, tensor2, pool_size):
    history_values = []
    with self.test_session(use_gpu=True) as sess:
      variables.global_variables_initializer().run()
      for i in range(2 * pool_size):
        t1, t2 = sess.run([tensor1, tensor2])
        history_values.append(t1)
        if i < pool_size:
          # For [0, pool_size), the pool is not full, tensor1 should be equal
          # to tensor2 as the pool.
          self.assertAllEqual(t1, t2)
        else:
          # For [pool_size, ?), the pool is full, tensor2 must be equal to some
          # historical values of tensor1 (which is previously stored in the
          # pool).
          self.assertTrue(any((v == t2).all() for v in history_values))

  def _make_new_model_and_check(self, model, pool_size):
    pool_fn = lambda x: random_tensor_pool.tensor_pool(x, pool_size=pool_size)
    new_model = train._tensor_pool_adjusted_model(model, pool_fn)
    # 'Generator/dummy_g:0' and 'Discriminator/dummy_d:0'
    self.assertEqual(2, len(ops.get_collection(ops.GraphKeys.VARIABLES)))
    self.assertIsNot(new_model.discriminator_gen_outputs,
                     model.discriminator_gen_outputs)

    return new_model

  def test_tensor_pool_adjusted_model_gan(self):
    """Test `_tensor_pool_adjusted_model` for gan model."""
    pool_size = 5
    model = create_gan_model()
    new_model = self._make_new_model_and_check(model, pool_size)

    # Check values.
    self._check_tensor_pool_adjusted_model_outputs(
        model.discriminator_gen_outputs, new_model.discriminator_gen_outputs,
        pool_size)

  def test_tensor_pool_adjusted_model_infogan(self):
    """Test _tensor_pool_adjusted_model for infogan model."""
    pool_size = 5
    model = create_infogan_model()
    new_model = self._make_new_model_and_check(model, pool_size)

    # Check values.
    self.assertIsNot(new_model.predicted_distributions,
                     model.predicted_distributions)
    self._check_tensor_pool_adjusted_model_outputs(
        model.discriminator_gen_outputs, new_model.discriminator_gen_outputs,
        pool_size)

  def test_tensor_pool_adjusted_model_acgan(self):
    """Test _tensor_pool_adjusted_model for acgan model."""
    pool_size = 5
    model = create_acgan_model()
    new_model = self._make_new_model_and_check(model, pool_size)

    # Check values.
    self.assertIsNot(new_model.discriminator_gen_classification_logits,
                     model.discriminator_gen_classification_logits)
    self._check_tensor_pool_adjusted_model_outputs(
        model.discriminator_gen_outputs, new_model.discriminator_gen_outputs,
        pool_size)


class GANTrainOpsTest(test.TestCase, parameterized.TestCase):
  """Tests for `gan_train_ops`."""

  @parameterized.named_parameters(
      ('gan', create_gan_model),
      ('callable_gan', create_callable_gan_model),
      ('infogan', create_infogan_model),
      ('callable_infogan', create_callable_infogan_model),
      ('acgan', create_acgan_model),
      ('callable_acgan', create_callable_acgan_model),
  )
  def test_output_type(self, create_gan_model_fn):
    model = create_gan_model_fn()
    loss = train.gan_loss(model)

    g_opt = gradient_descent.GradientDescentOptimizer(1.0)
    d_opt = gradient_descent.GradientDescentOptimizer(1.0)
    train_ops = train.gan_train_ops(
        model,
        loss,
        g_opt,
        d_opt,
        summarize_gradients=True,
        colocate_gradients_with_ops=True)

    self.assertIsInstance(train_ops, namedtuples.GANTrainOps)

    # Make sure there are no training hooks populated accidentally.
    self.assertEmpty(train_ops.train_hooks)

  # TODO(joelshor): Add a test to check that custom update op is run.
  @parameterized.named_parameters(
      ('gan', create_gan_model, False),
      ('gan_provideupdates', create_gan_model, True),
      ('callable_gan', create_callable_gan_model, False),
      ('callable_gan_provideupdates', create_callable_gan_model, True),
      ('infogan', create_infogan_model, False),
      ('infogan_provideupdates', create_infogan_model, True),
      ('callable_infogan', create_callable_infogan_model, False),
      ('callable_infogan_provideupdates', create_callable_infogan_model, True),
      ('acgan', create_acgan_model, False),
      ('acgan_provideupdates', create_acgan_model, True),
      ('callable_acgan', create_callable_acgan_model, False),
      ('callable_acgan_provideupdates', create_callable_acgan_model, True),
  )
  def test_unused_update_ops(self, create_gan_model_fn, provide_update_ops):
    model = create_gan_model_fn()
    loss = train.gan_loss(model)

    # Add generator and discriminator update ops.
    with variable_scope.variable_scope(model.generator_scope):
      gen_update_count = variable_scope.get_variable('gen_count', initializer=0)
      gen_update_op = gen_update_count.assign_add(1)
      ops.add_to_collection(ops.GraphKeys.UPDATE_OPS, gen_update_op)
    with variable_scope.variable_scope(model.discriminator_scope):
      dis_update_count = variable_scope.get_variable('dis_count', initializer=0)
      dis_update_op = dis_update_count.assign_add(1)
      ops.add_to_collection(ops.GraphKeys.UPDATE_OPS, dis_update_op)

    # Add an update op outside the generator and discriminator scopes.
    if provide_update_ops:
      kwargs = {
          'update_ops': [
              constant_op.constant(1.0), gen_update_op, dis_update_op
          ]
      }
    else:
      ops.add_to_collection(ops.GraphKeys.UPDATE_OPS, constant_op.constant(1.0))
      kwargs = {}

    g_opt = gradient_descent.GradientDescentOptimizer(1.0)
    d_opt = gradient_descent.GradientDescentOptimizer(1.0)

    with self.assertRaisesRegexp(ValueError, 'There are unused update ops:'):
      train.gan_train_ops(
          model, loss, g_opt, d_opt, check_for_unused_update_ops=True, **kwargs)
    train_ops = train.gan_train_ops(
        model, loss, g_opt, d_opt, check_for_unused_update_ops=False, **kwargs)

    with self.test_session(use_gpu=True) as sess:
      sess.run(variables.global_variables_initializer())
      self.assertEqual(0, gen_update_count.eval())
      self.assertEqual(0, dis_update_count.eval())

      train_ops.generator_train_op.eval()
      self.assertEqual(1, gen_update_count.eval())
      self.assertEqual(0, dis_update_count.eval())

      train_ops.discriminator_train_op.eval()
      self.assertEqual(1, gen_update_count.eval())
      self.assertEqual(1, dis_update_count.eval())

  @parameterized.named_parameters(
      ('gan', create_gan_model, False),
      ('callable_gan', create_callable_gan_model, False),
      ('infogan', create_infogan_model, False),
      ('callable_infogan', create_callable_infogan_model, False),
      ('acgan', create_acgan_model, False),
      ('callable_acgan', create_callable_acgan_model, False),
      ('gan_canbeint32', create_gan_model, True),
  )
  def test_sync_replicas(self, create_gan_model_fn, create_global_step):
    model = create_gan_model_fn()
    loss = train.gan_loss(model)
    num_trainable_vars = len(variables_lib.get_trainable_variables())

    if create_global_step:
      gstep = variable_scope.get_variable(
          'custom_gstep', dtype=dtypes.int32, initializer=0, trainable=False)
      ops.add_to_collection(ops.GraphKeys.GLOBAL_STEP, gstep)

    g_opt = get_sync_optimizer()
    d_opt = get_sync_optimizer()
    train_ops = train.gan_train_ops(
        model, loss, generator_optimizer=g_opt, discriminator_optimizer=d_opt)
    self.assertIsInstance(train_ops, namedtuples.GANTrainOps)
    # No new trainable variables should have been added.
    self.assertLen(variables_lib.get_trainable_variables(), num_trainable_vars)

    # Sync hooks should be populated in the GANTrainOps.
    self.assertLen(train_ops.train_hooks, 2)
    for hook in train_ops.train_hooks:
      self.assertIsInstance(
          hook, sync_replicas_optimizer._SyncReplicasOptimizerHook)
    sync_opts = [hook._sync_optimizer for hook in train_ops.train_hooks]
    self.assertSetEqual(frozenset(sync_opts), frozenset((g_opt, d_opt)))

    g_sync_init_op = g_opt.get_init_tokens_op(num_tokens=1)
    d_sync_init_op = d_opt.get_init_tokens_op(num_tokens=1)

    # Check that update op is run properly.
    global_step = training_util.get_or_create_global_step()
    with self.test_session(use_gpu=True) as sess:
      variables.global_variables_initializer().run()
      variables.local_variables_initializer().run()

      g_opt.chief_init_op.run()
      d_opt.chief_init_op.run()

      gstep_before = global_step.eval()

      # Start required queue runner for SyncReplicasOptimizer.
      coord = coordinator.Coordinator()
      g_threads = g_opt.get_chief_queue_runner().create_threads(sess, coord)
      d_threads = d_opt.get_chief_queue_runner().create_threads(sess, coord)

      g_sync_init_op.run()
      d_sync_init_op.run()

      train_ops.generator_train_op.eval()
      # Check that global step wasn't incremented.
      self.assertEqual(gstep_before, global_step.eval())

      train_ops.discriminator_train_op.eval()
      # Check that global step wasn't incremented.
      self.assertEqual(gstep_before, global_step.eval())

      coord.request_stop()
      coord.join(g_threads + d_threads)

  @parameterized.named_parameters(
      ('is_chief', True),
      ('is_not_chief', False),
  )
  def test_is_chief_in_train_hooks(self, is_chief):
    """Make sure is_chief is propagated correctly to sync hooks."""
    model = create_gan_model()
    loss = train.gan_loss(model)
    g_opt = get_sync_optimizer()
    d_opt = get_sync_optimizer()
    train_ops = train.gan_train_ops(
        model,
        loss,
        g_opt,
        d_opt,
        is_chief=is_chief,
        summarize_gradients=True,
        colocate_gradients_with_ops=True)

    self.assertLen(train_ops.train_hooks, 2)
    for hook in train_ops.train_hooks:
      self.assertIsInstance(
          hook, sync_replicas_optimizer._SyncReplicasOptimizerHook)
    is_chief_list = [hook._is_chief for hook in train_ops.train_hooks]
    self.assertListEqual(is_chief_list, [is_chief, is_chief])


class GANTrainTest(test.TestCase, parameterized.TestCase):
  """Tests for `gan_train`."""

  def _gan_train_ops(self, generator_add, discriminator_add):
    step = training_util.create_global_step()
    # Increment the global count every time a train op is run so we can count
    # the number of times they're run.
    # NOTE: `use_locking=True` is required to avoid race conditions with
    # joint training.
    train_ops = namedtuples.GANTrainOps(
        generator_train_op=step.assign_add(generator_add, use_locking=True),
        discriminator_train_op=step.assign_add(
            discriminator_add, use_locking=True),
        global_step_inc_op=step.assign_add(1))
    return train_ops

  @parameterized.named_parameters(
      ('gan', create_gan_model),
      ('callable_gan', create_callable_gan_model),
      ('infogan', create_infogan_model),
      ('callable_infogan', create_callable_infogan_model),
      ('acgan', create_acgan_model),
      ('callable_acgan', create_callable_acgan_model),
  )
  def test_run_helper(self, create_gan_model_fn):
    random_seed.set_random_seed(1234)
    model = create_gan_model_fn()
    loss = train.gan_loss(model)

    g_opt = gradient_descent.GradientDescentOptimizer(1.0)
    d_opt = gradient_descent.GradientDescentOptimizer(1.0)
    train_ops = train.gan_train_ops(model, loss, g_opt, d_opt)

    final_step = train.gan_train(
        train_ops,
        logdir='',
        hooks=[basic_session_run_hooks.StopAtStepHook(num_steps=2)])
    self.assertTrue(np.isscalar(final_step))
    self.assertEqual(2, final_step)

  @parameterized.named_parameters(
      ('seq_train_steps', train.get_sequential_train_hooks),
      ('efficient_seq_train_steps', train.get_joint_train_hooks),
  )
  def test_multiple_steps(self, get_hooks_fn_fn):
    """Test multiple train steps."""
    train_ops = self._gan_train_ops(generator_add=10, discriminator_add=100)
    train_steps = namedtuples.GANTrainSteps(
        generator_train_steps=3, discriminator_train_steps=4)
    final_step = train.gan_train(
        train_ops,
        get_hooks_fn=get_hooks_fn_fn(train_steps),
        logdir='',
        hooks=[basic_session_run_hooks.StopAtStepHook(num_steps=1)])

    self.assertTrue(np.isscalar(final_step))
    self.assertEqual(1 + 3 * 10 + 4 * 100, final_step)

  def test_supervisor_run_gan_model_train_ops_multiple_steps(self):
    step = training_util.create_global_step()
    train_ops = namedtuples.GANTrainOps(
        generator_train_op=constant_op.constant(3.0),
        discriminator_train_op=constant_op.constant(2.0),
        global_step_inc_op=step.assign_add(1))
    train_steps = namedtuples.GANTrainSteps(
        generator_train_steps=3, discriminator_train_steps=4)

    final_loss = slim_learning.train(
        train_op=train_ops,
        logdir='',
        global_step=step,
        number_of_steps=1,
        train_step_fn=train.get_sequential_train_steps(train_steps))
    self.assertTrue(np.isscalar(final_loss))
    self.assertEqual(17.0, final_loss)

  @parameterized.named_parameters(
      ('gan', create_gan_model),
      ('callable_gan', create_callable_gan_model),
      ('infogan', create_infogan_model),
      ('callable_infogan', create_callable_infogan_model),
      ('acgan', create_acgan_model),
      ('callable_acgan', create_callable_acgan_model),
  )
  def test_train_hooks_exist_in_get_hooks_fn(self, create_gan_model_fn):
    model = create_gan_model_fn()
    loss = train.gan_loss(model)

    g_opt = get_sync_optimizer()
    d_opt = get_sync_optimizer()
    train_ops = train.gan_train_ops(
        model,
        loss,
        g_opt,
        d_opt,
        summarize_gradients=True,
        colocate_gradients_with_ops=True)

    sequential_train_hooks = train.get_sequential_train_hooks()(train_ops)
    self.assertLen(sequential_train_hooks, 4)
    sync_opts = [
        hook._sync_optimizer for hook in sequential_train_hooks if
        isinstance(hook, sync_replicas_optimizer._SyncReplicasOptimizerHook)]
    self.assertLen(sync_opts, 2)
    self.assertSetEqual(frozenset(sync_opts), frozenset((g_opt, d_opt)))

    joint_train_hooks = train.get_joint_train_hooks()(train_ops)
    self.assertLen(joint_train_hooks, 5)
    sync_opts = [
        hook._sync_optimizer for hook in joint_train_hooks if
        isinstance(hook, sync_replicas_optimizer._SyncReplicasOptimizerHook)]
    self.assertLen(sync_opts, 2)
    self.assertSetEqual(frozenset(sync_opts), frozenset((g_opt, d_opt)))


class PatchGANTest(test.TestCase, parameterized.TestCase):
  """Tests that functions work on PatchGAN style output."""

  @parameterized.named_parameters(
      ('gan', create_gan_model),
      ('callable_gan', create_callable_gan_model),
      ('infogan', create_infogan_model),
      ('callable_infogan', create_callable_infogan_model),
      ('acgan', create_acgan_model),
      ('callable_acgan', create_callable_acgan_model),
  )
  def test_patchgan(self, create_gan_model_fn):
    """Ensure that patch-based discriminators work end-to-end."""
    random_seed.set_random_seed(1234)
    model = create_gan_model_fn()
    loss = train.gan_loss(model)

    g_opt = gradient_descent.GradientDescentOptimizer(1.0)
    d_opt = gradient_descent.GradientDescentOptimizer(1.0)
    train_ops = train.gan_train_ops(model, loss, g_opt, d_opt)

    final_step = train.gan_train(
        train_ops,
        logdir='',
        hooks=[basic_session_run_hooks.StopAtStepHook(num_steps=2)])
    self.assertTrue(np.isscalar(final_step))
    self.assertEqual(2, final_step)


if __name__ == '__main__':
  test.main()
