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
"""Tests for AddSign."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.compiler.tests import xla_test
from tensorflow.contrib.opt.python.training import addsign
from tensorflow.contrib.opt.python.training import sign_decay
from tensorflow.python.framework import constant_op
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import test


def py_linear_decay_fn(decay_steps):
  def linear_decay(step):
    step = min(step, decay_steps)
    return float(decay_steps - step) / decay_steps
  return linear_decay


def addsign_update_numpy(params,
                         g_t,
                         m,
                         lr,
                         alpha=1.0,
                         beta=0.9,
                         py_sign_decay_fn=None,
                         t=None):
  m_t = beta * m + (1 - beta) * g_t
  if py_sign_decay_fn is None:
    sign_decayed = 1.0
  else:
    sign_decayed = py_sign_decay_fn(t-1)
  multiplier = alpha + sign_decayed * np.sign(g_t) * np.sign(m_t)
  params_t = params - lr * multiplier * g_t
  return params_t, m_t


class AddSignTest(xla_test.XLATestCase):

  def _testDense(self,
                 learning_rate=0.1,
                 sign_decay_fn=None,
                 py_sign_decay_fn=None,
                 alpha=1.0,
                 beta=0.9):
    for dtype in self.float_types:
      with self.cached_session(), self.test_scope():
        # Initialize variables for numpy implementation.
        m0, m1 = 0.0, 0.0
        var0_np = np.array([1.0, 2.0], dtype=dtype)
        grads0_np = np.array([0.1, 0.1], dtype=dtype)
        var1_np = np.array([3.0, 4.0], dtype=dtype)
        grads1_np = np.array([0.01, 0.01], dtype=dtype)

        var0 = resource_variable_ops.ResourceVariable(var0_np)
        var1 = resource_variable_ops.ResourceVariable(var1_np)
        global_step = resource_variable_ops.ResourceVariable(0, trainable=False)
        grads0 = constant_op.constant(grads0_np)
        grads1 = constant_op.constant(grads1_np)

        opt = addsign.AddSignOptimizer(
            learning_rate=learning_rate,
            alpha=alpha,
            beta=beta,
            sign_decay_fn=sign_decay_fn,
        )
        update = opt.apply_gradients(zip([grads0, grads1], [var0, var1]),
                                     global_step=global_step)
        neg_update = opt.apply_gradients(zip([-grads0, -grads1], [var0, var1]),
                                         global_step=global_step)
        variables.global_variables_initializer().run()

        # Fetch params to validate initial values
        self.assertAllClose([1.0, 2.0], self.evaluate(var0))
        self.assertAllClose([3.0, 4.0], self.evaluate(var1))

        # Run 7 steps of AddSign
        # first 4 steps with positive gradient
        # last 3 steps with negative gradient (sign(gm) should be -1)
        for t in range(1, 8):
          if t < 5:
            update.run()
          else:
            neg_update.run()

          var0_np, m0 = addsign_update_numpy(
              var0_np,
              grads0_np if t < 5 else -grads0_np,
              m0,
              learning_rate,
              alpha=alpha,
              beta=beta,
              py_sign_decay_fn=py_sign_decay_fn,
              t=t,
          )
          var1_np, m1 = addsign_update_numpy(
              var1_np,
              grads1_np if t < 5 else -grads1_np,
              m1,
              learning_rate,
              alpha=alpha,
              beta=beta,
              py_sign_decay_fn=py_sign_decay_fn,
              t=t,
          )

          # Validate updated params
          self.assertAllCloseAccordingToType(
              var0_np, self.evaluate(var0), half_rtol=1e-2)
          self.assertAllCloseAccordingToType(var1_np, self.evaluate(var1))

  def testDense(self):
    decay_steps = 10
    sign_decay_fn = sign_decay.get_linear_decay_fn(decay_steps)
    py_sign_decay_fn = py_linear_decay_fn(decay_steps)
    self._testDense()
    self._testDense(learning_rate=0.01, alpha=0.1, beta=0.8)
    self._testDense(
        sign_decay_fn=sign_decay_fn, py_sign_decay_fn=py_sign_decay_fn)


if __name__ == '__main__':
  test.main()
