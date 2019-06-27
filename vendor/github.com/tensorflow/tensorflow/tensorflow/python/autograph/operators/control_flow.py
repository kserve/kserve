# Copyright 2018 The TensorFlow Authors. All Rights Reserved.
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
"""Control flow statements: loops, conditionals, etc."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.autograph.operators import py_builtins
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.framework import tensor_util
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import gen_math_ops


def for_stmt(iter_, extra_test, body, init_state):
  """Functional form of a for statement.

  The loop operates on a state, which includes all symbols that are
  variant across loop iterations, excluding the iterate as well as the
  variables local to the loop.

  For example, given the loop below that calculates the geometric and
  arithmetic means or some numbers:

    geo_mean = 1
    arith_mean = 0
    for i in range(n):
      a = numbers[i]
      geo_mean *= a
      arith_mean += a

  The state is represented by the variables geo_mean and arith_mean. The
  argument for initial_state may contain the tuple (1, 0), the body will
  include the arguments geo_mean and arith_mean and will return a tuple
  representing the new values for geo_mean and respectively arith_mean.

  Args:
    iter_: The entity being iterated over.
    extra_test: Callable with the state as arguments, and boolean return type.
        An additional loop condition.
    body: Callable with the iterate and the state as arguments, and
        state as return type. The actual loop body.
    init_state: Tuple containing the initial state.

  Returns:
    Tuple containing the final state.
  """
  if tensor_util.is_tensor(iter_):
    return _known_len_for_stmt(iter_, extra_test, body, init_state)
  elif isinstance(iter_, dataset_ops.DatasetV2):
    return _dataset_for_stmt(iter_, extra_test, body, init_state)
  else:
    return _py_for_stmt(iter_, extra_test, body, init_state)


def _py_for_stmt(iter_, extra_test, body, init_state):
  """Overload of for_stmt that executes a Python for loop."""
  state = init_state
  for target in iter_:
    if not extra_test(*state):
      break
    state = body(target, *state)

  # TODO(mdan): Remove this special case.
  if len(state) == 1:
    return state[0]
  return state


def _known_len_for_stmt(iter_, extra_test, body, init_state):
  """Overload of for_stmt that iterates over objects that admit a length."""
  n = py_builtins.len_(iter_)

  def while_body(iterate_index, *state):
    iterate = iter_[iterate_index]
    new_state = body(iterate, *state)
    if new_state:
      return (iterate_index + 1,) + new_state
    else:
      return iterate_index + 1

  def while_cond(iterate_index, *state):
    return gen_math_ops.logical_and(iterate_index < n, extra_test(*state))

  results = while_stmt(
      while_cond,
      while_body,
      init_state=(0,) + init_state,
      extra_deps=(iter_,),
      opts=dict(maximum_iterations=n))

  # Dropping the iteration index because it's not syntactically visible.
  # TODO(mdan): Don't.
  if isinstance(results, (tuple, list)):
    assert len(results) >= 1  # Has at least the iterate.
    if len(results) > 1:
      results = results[1:]
    if len(results) == 1:
      # TODO(mdan): Remove this special case.
      results, = results
  else:
    results = ()

  return results


def _dataset_for_stmt(ds, extra_test, body, init_state):
  """Overload of for_stmt that iterates over TF Datasets."""
  if extra_test(*init_state) is not True:
    raise NotImplementedError(
        'break statements are not yet supported in for/Dataset loops')

  def reduce_body(state, iterate):
    new_state = body(iterate, *state)
    return new_state

  results = ds.reduce(init_state, reduce_body)

  # TODO(mdan): Remove this special case.
  if len(results) == 1:
    return results[0]
  return results


def while_stmt(test, body, init_state, extra_deps, opts=None):
  """Functional form of a while statement.

  The loop operates on a so-called state, which includes all symbols that are
  variant across loop iterations. In what follows we refer to state as either
  a tuple of entities that represent an actual state, or a list of arguments
  of the corresponding types.

  Args:
    test: Callable with the state as arguments, and boolean return type.
        The loop condition.
    body: Callable with the state as arguments, and state as return type.
        The actual loop body.
    init_state: Tuple containing the initial state.
    extra_deps: Tuple containing additional entities on which the loop may
        depend, such as loop invariants referenced by test. Used
        exclusively for dispatch control.
    opts: Optional dict of extra loop parameters.

  Returns:
    Tuple containing the final state.
  """
  # TODO(mdan): Consider adding a generic mechanism for dynamic dispatch.
  # That could be something as simple as a collection of dispatch rules, with
  # some prioritization.
  if any(tensor_util.is_tensor(v) for v in init_state + extra_deps):
    return _tf_while_stmt(test, body, init_state, opts)
  else:
    return _py_while_stmt(test, body, init_state, opts)


def _tf_while_stmt(test, body, init_state, opts):
  """Overload of while_stmt that stages a TF while_stmt."""
  if opts is None:
    opts = {}
  return control_flow_ops.while_loop(test, body, init_state, **opts)


def _py_while_stmt(test, body, init_state, opts):
  """Overload of while_stmt that executes a Python while loop."""
  del opts
  state = init_state
  while test(*state):
    state = body(*state)
  return state


def if_stmt(cond, body, orelse):
  """Functional form of an if statement.

  Args:
    cond: Boolean.
    body: Callable with no arguments, and outputs of the positive (if) branch
        as return type.
    orelse: Callable with no arguments, and outputs of the negative (else)
        branch as return type.

  Returns:
    Tuple containing the statement outputs.
  """
  if tensor_util.is_tensor(cond):
    return tf_if_stmt(cond, body, orelse)
  else:
    return _py_if_stmt(cond, body, orelse)


def tf_if_stmt(cond, body, orelse):
  """Overload of if_stmt that stages a TF cond."""
  return control_flow_ops.cond(cond, body, orelse)


def _py_if_stmt(cond, body, orelse):
  """Overload of if_stmt that executes a Python if statement."""
  return body() if cond else orelse()
