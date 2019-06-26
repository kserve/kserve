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
# =============================================================================
"""xla is an experimental library that provides XLA support APIs."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections
import contextlib
from six.moves import xrange  # pylint: disable=redefined-builtin

from tensorflow.compiler.jit.ops import xla_ops
from tensorflow.compiler.jit.ops import xla_ops_grad  # pylint: disable=unused-import
from tensorflow.core.framework import attr_value_pb2
from tensorflow.python.estimator import model_fn as model_fn_lib
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import summary_op_util
from tensorflow.python.ops import variable_scope
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.util import compat
from tensorflow.python.util import function_utils
from tensorflow.python.util import tf_decorator
from tensorflow.python.util import tf_inspect

_XLA_COMPILE_ATTR = '_xla_compile_id'
_MAX_WARNING_LINES = 5

# Operations that indicate some error in the users graph. For example, XLA
# computation should not have any Placeholder op.
_BLACKLISTED_OPS = set([
    'Placeholder',
])

# XLA doesn't currently support reading of intermediate tensors, thus some ops
# are not supported.
_UNSUPPORTED_OPS = set([
    'AudioSummary',
    'AudioSummaryV2',
    'HistogramSummary',
    'ImageSummary',
    'MergeSummary',
    'Print',
    'ScalarSummary',
    'TensorSummary',
    'TensorSummaryV2',
])


def compile(computation, inputs=None):  # pylint: disable=redefined-builtin
  """Builds an operator that compiles and runs `computation` with XLA.

  Args:
    computation: A Python function that builds a computation to apply to the
      input. If the function takes n inputs, 'inputs' should be a list of n
      tensors.

      `computation` may return a list of operations and tensors.  Tensors must
      come before operations in the returned list.  The return value of
      `compile` is a list of tensors corresponding to the tensors from the
      output of `computation`.

      All `Operation`s returned from `computation` will be executed when
      evaluating any of the returned output tensors.
    inputs: A list of input tensors or `None` (equivalent to an empty list).

  Returns:
    A list of output tensors.
  """
  # pylint: disable=protected-access
  return _compile_internal(computation, inputs)


class XLACompileContext(control_flow_ops.XLAControlFlowContext):
  """A `ControlFlowContext` for nodes inside an XLA computation cluster.

  THIS IS ONLY FOR TENSORFLOW INTERNAL IMPLEMENTATION, DO NO USE DIRECTLY.

  The primary role of `XLACompileContext` is to mark operators inside a
  xla.compile() computation with attribute "_xla_compile_id=XYZ", where XYZ is
  a unique name.

  `ControlFlowContext` is used to perform the annotation since it integrates
  with Tensorflow constructs like ResourceVariables. For example, if a
  `ResourceVariable` is constructed inside a xla.compile() block, the
  `ResourceVariable` implementation can use
  `with ops.control_dependencies(None)` to build the variable's definition
  outside the compiled computation.
  """

  def __init__(self, name, pivot):
    """Builds a new XLACompileContext.

    Args:
      name: a unique name for the context, used to populate the
        `_xla_compile_id` attribute.
      pivot: a pivot node. Nodes in the XLACompileContext that do not have any
        inputs will have a control dependency on the pivot node. This ensures
        that nodes are correctly included in any enclosing control flow
        contexts.
    """
    super(XLACompileContext, self).__init__()
    self._name = name
    self._name_as_bytes = compat.as_bytes(name)
    self._unsupported_ops = []
    self._pivot = pivot

  def report_unsupported_operations(self):
    if self._unsupported_ops:
      op_str = '\n'.join([
          '  %s (%s)' % (op.type, op.name)
          for op in self._unsupported_ops[:_MAX_WARNING_LINES]
      ])
      logging.warning('%d unsupported operations found: \n%s',
                      len(self._unsupported_ops), op_str)
      if len(self._unsupported_ops) > _MAX_WARNING_LINES:
        logging.warning('... and %d more',
                        len(self._unsupported_ops) - _MAX_WARNING_LINES)

  def AddOp(self, op):
    """Create op in XLACompileContext and notifies outer context recursively."""
    # pylint: disable=protected-access
    if op.type in _BLACKLISTED_OPS:
      logging.error(
          'Operation of type %s (%s) is not supported in XLA. Execution will '
          'fail if this op is used in the graph. ', op.type, op.name)

    # TODO(ycao): Automatically disable summaries instead of reporting them.
    if op.type in _UNSUPPORTED_OPS:
      self._unsupported_ops.append(op)

    if any(x.dtype._is_ref_dtype for x in op.inputs):
      raise NotImplementedError(
          'Non-resource Variables are not supported inside XLA computations '
          '(operator name: %s)' % op.name)

    if _XLA_COMPILE_ATTR in op.node_def.attr:
      raise ValueError('XLA compiled computations cannot be nested, (operator '
                       'name: %s)' % op.name)

    op._set_attr(
        _XLA_COMPILE_ATTR, attr_value_pb2.AttrValue(s=self._name_as_bytes))

    op.graph.prevent_feeding(op)
    op.graph.prevent_fetching(op)

    # Remove any control edges from outer control flow contexts. These may cause
    # mismatched frame errors. An example is when one of op's inputs is
    # generated in a different While control flow context.
    (internal_control_inputs,
     external_control_inputs) = self._RemoveExternalControlEdges(op)

    if not op.inputs:
      # Add a control edge from the control pivot to this op.
      if not internal_control_inputs:
        # pylint: disable=protected-access
        op._add_control_input(self._pivot)
        # pylint: enable=protected-access
    else:
      for index in xrange(len(op.inputs)):
        x = op.inputs[index]
        real_x = self.AddValue(x)
        if real_x != x:
          op._update_input(index, real_x)  # pylint: disable=protected-access

    if external_control_inputs:
      # Use an identity to pull control inputs as data inputs. Note that we
      # ignore ops which don't have outputs. TODO(phawkins): fix that.
      external_control_inputs = [
          array_ops.identity(x.outputs[0]).op
          for x in external_control_inputs
          if x.outputs
      ]
      # pylint: disable=protected-access
      op._add_control_inputs(external_control_inputs)
      # pylint: enable=protected-access

    # Mark op's outputs as seen by this context and any outer contexts.
    output_names = [x.name for x in op.outputs]
    context = self
    while context is not None:
      # pylint: disable=protected-access
      context._values.update(output_names)
      context = context._outer_context
      # pylint: enable=protected-access

    if self._outer_context:
      self._outer_context.AddInnerOp(op)

  def AddValue(self, val):
    """Add `val` to the current context and its outer context recursively."""
    if val.name in self._values:
      # Use the real value if it comes from outer context.
      result = self._external_values.get(val.name)
      return val if result is None else result

    result = val
    self._values.add(val.name)
    if self._outer_context:
      result = self._outer_context.AddValue(val)
      self._values.add(result.name)

    self._external_values[val.name] = result

    return result

  def AddInnerOp(self, op):
    self.AddOp(op)
    if self._outer_context:
      self._outer_context.AddInnerOp(op)

  @property
  def grad_state(self):
    # Define the gradient loop state associated with the XLACompileContext to
    # be None as the XLACompileContext does not get nested nor does the
    # grad_state outside the XLACompileContext affect the graph inside so the
    # grad_state should be as if this is the top-level gradient state.
    return None

  @property
  def back_prop(self):
    """Forwards to the enclosing while context, if any."""
    if self.GetWhileContext():
      return self.GetWhileContext().back_prop
    return False


def _compile_internal(computation, inputs=None):
  """Builds graph operators that compiles and symbolically executes computation.

  Args:
    computation: A Python function that builds the computation to compile and
      execute.
    inputs: A list of input tensors or `None` (equivalent to `[]`). Its order
      should match ordering of computation arguments.
  Returns:
    A list of output tensors from computation.
  Raises:
    ValueError: If any element in computation outputs is neither an operations
      or a value that can be converted to tensor.
    TypeError: If `inputs` is not a list or tuple.
  """
  if inputs is None:
    inputs = []

  if not isinstance(inputs, collections.Sequence):
    raise TypeError('inputs must be a list')

  # Converts inputs to Tensors.
  inputs = [ops.convert_to_tensor(x) for x in inputs]
  input_arity = len(inputs)

  arg_error = check_function_argument_count(
      computation, input_arity, infeed_queue=None)
  if arg_error is not None:
    raise TypeError(
        'Supplied computation cannot be called with the specified inputs. You '
        'specified %d inputs: %s, but the computation needs %s' %
        (input_arity, str([i.name for i in inputs]), arg_error))

  cluster_name = ops.get_default_graph().unique_name('cluster')
  pivot = control_flow_ops.no_op(name=cluster_name + '/pivot')
  context = XLACompileContext(name=cluster_name, pivot=pivot)
  try:
    context.Enter()

    # Add identity ops so even unused inputs are 'consumed' by the
    # computation.
    computation_inputs = [
        array_ops.identity(x, name='input_{}'.format(i))
        for i, x in enumerate(inputs)
    ]

    # Only resource variables work inside an XLA computation, so turn on
    # resource variables for the computation.
    vscope = variable_scope.get_variable_scope()
    saved_use_resource = vscope.use_resource
    vscope.set_use_resource(True)

    with _disable_summary_context():
      outputs = computation(*computation_inputs)

    # Restore variable scope after computation.
    vscope.set_use_resource(saved_use_resource)

    # If the computation returns `None`, make it an empty tuple.
    if outputs is None:
      outputs = tuple()
    # If the computation only returned one value, make it a tuple.
    if not isinstance(outputs, collections.Sequence):
      outputs = (outputs,)

    # Append `no_op` here so that return value of this function always contains
    # at least one op that can trigger XlaLaunch node.
    outputs += (control_flow_ops.no_op(),)
    try:
      outputs = [
          o if isinstance(o, ops.Operation) else ops.convert_to_tensor(o)
          for o in outputs
      ]
    except Exception as e:
      raise ValueError(
          'XLA computation function return values must all either be Operations'
          ' or convertible to Tensors. Got error: "%s"' % str(e))

    # Separates the returned Operations and Tensors.
    output_operations = [o for o in outputs if isinstance(o, ops.Operation)]
    output_tensors = [o for o in outputs if not isinstance(o, ops.Operation)]

    if outputs != output_tensors + output_operations:
      raise ValueError(
          'XLA computation function must return zero or more Tensor values '
          'followed by zero or more Operations.')
    output_arity = len(output_tensors)

    new_output_tensors = []
    for t in output_tensors:
      with ops.device(t.device if t.device else ''):
        new_output_tensors.append(array_ops.identity(t))

    output_tensors = new_output_tensors
    context.ExitResult(output_tensors)
  finally:
    context.report_unsupported_operations()
    context.Exit()

  outputs = [
      xla_ops.xla_cluster_output(output_tensors[i], name='output{}'.format(i))
      for i in xrange(output_arity)
  ]

  with ops.control_dependencies(output_operations):
    if output_arity == 0:
      # When XLA computation returns only operations and no tensors, a NoOp
      # dependent on the operations in outputs is returned. Otherwise final
      # outputs would be empty and there is no way to trigger returned
      # operations.
      return control_flow_ops.no_op(name='output_0')
    else:
      # Wraps the outputs in identity operators that carries control
      # dependencies.
      return [
          array_ops.identity(outputs[i], name='output_%d' % i)
          for i in xrange(output_arity)
      ]


@contextlib.contextmanager
def _disable_summary_context():
  """Enters a context where all summary ops are skipped.

  Summaries are not yet supported in xla.compile(). So we provide this context
  manager that can skip creating summary ops. This is a temporary workaround due
  to XLA not supporting summary ops.

  Yields:
    None.
  """
  original_skip_summary_func = summary_op_util.skip_summary
  summary_op_util.skip_summary = lambda: True

  try:
    yield
  finally:
    summary_op_util.skip_summary = original_skip_summary_func


class _CapturedObject(object):
  """A placeholder to capture an object."""

  def __init__(self):
    self._object = None

  def capture(self, o):
    if self._object:
      raise RuntimeError(
          'InternalError: _CapturedObject can capture only once. Please file '
          'bug.')

    self._object = o

  def get(self):
    return self._object


def _get_scaffold(captured_scaffold_fn):
  """Retrieves the Scaffold from `captured_scaffold_fn`."""
  scaffold_fn = captured_scaffold_fn.get()

  if not scaffold_fn:
    return None

  scaffold = scaffold_fn()
  if scaffold is None:
    raise ValueError(
        'TPUEstimatorSpec.scaffold_fn returns None, which is not allowed')

  return scaffold


class _ModelFnWrapper(object):
  """_ModelFnWrapper supports executing model_fn with XLA."""

  def __init__(self, function):
    self._model_fn = function

  def __call__(self, features, labels, mode, params):

    # TPUEstimator compiles model_fn when use_tpu=True. To avoid double
    # compilation, we use this params['use_tpu'] as a hint. When it is set to
    # True, model_fn is called without compilation.
    # Note that this condition isn't accurate for the case of exporting a model.
    # In that case we should ideally not compile so that user can see detailed
    # graph. However, we don't have enough information to tell whether model_fn
    # is being called for export mode or not.
    # TODO(ycao): Make this condition more accurate when implementing PREDICT
    # mode.
    if params.get('use_tpu'):
      return self._call_model_fn(features, labels, mode, params)

    if mode == model_fn_lib.ModeKeys.TRAIN:
      train_step, captured_scaffold_fn = self._make_train_step(
          features, labels, params)
      (loss,) = compile(train_step)
      return model_fn_lib.EstimatorSpec(
          mode=mode,
          loss=loss,
          train_op=array_ops.identity(loss),
          scaffold=_get_scaffold(captured_scaffold_fn))
    elif mode == model_fn_lib.ModeKeys.EVAL:
      eval_step, captured_eval_metric_fn, captured_scaffold_fn = (
          self._make_eval_step(features, labels, params))
      outputs = compile(eval_step)
      loss = outputs[0]

      # Calculate eval_metric_ops if eval_metric_fn is set and captured.
      eval_metric_fn = captured_eval_metric_fn.get()
      if eval_metric_fn:
        eval_metric_fn_tensors = outputs[1:]
        eval_metric_ops = eval_metric_fn(*eval_metric_fn_tensors)
      else:
        eval_metric_ops = None

      return model_fn_lib.EstimatorSpec(
          mode=mode,
          loss=loss,
          eval_metric_ops=eval_metric_ops,
          scaffold=_get_scaffold(captured_scaffold_fn))
    else:
      raise NotImplementedError('%s is not implemented, only TRAIN and EVAL are'
                                ' supported' % mode)

  def _make_train_step(self, features, labels, params):
    """Creates a single step of training for xla.compile()."""
    captured_scaffold_fn = _CapturedObject()

    def train_step():
      """A single step of training."""
      estimator_spec = self._call_model_fn(features, labels,
                                           model_fn_lib.ModeKeys.TRAIN, params)

      try:
        captured_scaffold_fn.capture(estimator_spec.scaffold_fn)
      except AttributeError:
        captured_scaffold_fn.capture(None)

      # train_step will be run by xla.compile(). xla.compile() only supports
      # tensor output while train_op can be either an operation or a tensor.
      # Even though xla.compile() automatically adds operation-typed train_op as
      # control dependency of other tensor outputs, it doesn't do so for
      # tensor-typed train_op. Thus, we need to set it explicitly here.
      with ops.control_dependencies([estimator_spec.train_op]):
        return array_ops.identity(estimator_spec.loss)

    return train_step, captured_scaffold_fn

  def _make_eval_step(self, features, labels, params):
    """Creates a single step of evaluation for xla.compile()."""
    captured_eval_metric_fn = _CapturedObject()
    captured_scaffold_fn = _CapturedObject()

    def eval_step():
      """A single step of evaluation."""
      estimator_spec = self._call_model_fn(features, labels,
                                           model_fn_lib.ModeKeys.EVAL, params)

      try:
        captured_scaffold_fn.capture(estimator_spec.scaffold_fn)
      except AttributeError:
        captured_scaffold_fn.capture(None)

      eval_metric_fn = None
      eval_metric_fn_tensors = []
      try:
        if estimator_spec.eval_metrics:
          (eval_metric_fn, eval_metric_fn_tensors) = estimator_spec.eval_metrics
      except AttributeError:
        pass

      # If a dictionary is provided, we need to convert it into a list sorted
      # according to order of eval_metric_fn positional arguments.
      if isinstance(eval_metric_fn_tensors, dict):
        eval_metric_fn_args = function_utils.fn_args(eval_metric_fn)
        eval_metric_fn_tensors = [
            eval_metric_fn_tensors[i] for i in eval_metric_fn_args
        ]

      captured_eval_metric_fn.capture(eval_metric_fn)

      return tuple([estimator_spec.loss] + eval_metric_fn_tensors)

    return eval_step, captured_eval_metric_fn, captured_scaffold_fn

  def _call_model_fn(self, features, labels, mode, params):
    """Calls the model_fn with required parameters."""
    model_fn_args = function_utils.fn_args(self._model_fn)
    kwargs = {}

    if 'labels' in model_fn_args:
      kwargs['labels'] = labels
    elif labels is not None:
      raise ValueError(
          'model_fn does not take labels, but input_fn returns labels.')
    if 'mode' in model_fn_args:
      kwargs['mode'] = mode

    if 'params' in model_fn_args:
      kwargs['params'] = params

    return self._verify_estimator_spec(
        self._model_fn(features=features, **kwargs))

  def _verify_estimator_spec(self, estimator_spec):
    """Verifies estimator spec contains correct data."""
    # TODO(ycao): Implement estimator spec verification for other modes.

    try:
      if estimator_spec.scaffold:
        logging.warning('EstimatorSpec.scaffold is ignored with XLA compilation'
                        '. Please use TPUEstimatorSpec.scaffold_fn instead.')
    except AttributeError:
      pass

    try:
      if estimator_spec.eval_metric_ops:
        raise ValueError('EstimatorSpec.eval_metric_ops is not supported with '
                         'XLA compilation. Please use '
                         'TPUEstimatorSpec.eval_metrics instead.')
    except AttributeError:
      pass

    if estimator_spec.mode == model_fn_lib.ModeKeys.EVAL:
      # If estimator_spec is of type TPUEstimatorSpec and contains eval_metrics,
      # check that eval_metrics contains eval_metric_fn and
      # eval_metric_fn_tensors with matching arguments.
      try:
        eval_metrics = estimator_spec.eval_metrics
      except AttributeError:
        eval_metrics = None

      if eval_metrics:
        (eval_metric_fn, eval_metric_fn_tensors) = eval_metrics
        eval_metric_fn_args = function_utils.fn_args(eval_metric_fn)

        if isinstance(eval_metric_fn_tensors, dict):
          missing_tensors = [
              i for i in eval_metric_fn_args if i not in eval_metric_fn_tensors
          ]
          additional_tensors = [
              i for i in eval_metric_fn_tensors if i not in eval_metric_fn_args
          ]

          if missing_tensors:
            raise ValueError('Arguments %s are needed by metric_fn (first '
                             'element of TPUEstimatorSpec.eval_metrics) but '
                             'they are not provided by evaluation tensors '
                             '(second element of TPUEstimatorSpec.eval_metrics)'
                             '.' % missing_tensors)

          if additional_tensors:
            raise ValueError('Arguments %s are provided by evaluation tensors '
                             '(second element of TPUEstimatorSpec.eval_metrics)'
                             ' but they are not needed by metric_fn (first '
                             'element of TPUEstimatorSpec.eval_metrics).' %
                             additional_tensors)

    return estimator_spec


def estimator_model_fn(target_model_fn=None):
  """estimator_model_fn decorates a model_fn to be compiled for execution.

  Currently it only works with `TPUEstimator`. If you need to use it with base
  `Estimator`, please add `tf.enable_resource_variables()` at the beginning of
  your program.

  Example 1, decorating model_fn:
  ```
  @xla.estimator_model_fn()
  def model_fn(features, labels, mode, params):
    ...
    return EstimatorSpec(...)


  est = Estimator(model_fn=model_fn, ...)
  est.train(...)

  ```

  Example 2, decorator as function:
  ```
  def model_fn(features, labels, mode, params):
    ...
    return EstimatorSpec(...)

  est = Estimator(model_fn=xla.estimator_model_fn(model_fn), ...)
  est.train(...)
  ```

  Args:
    target_model_fn: model_fn to be decorated. This is only needed when
      decorator is used in function call form (example 2).

  Returns:
    Decorated target_model_fn.
  """

  def decorated(function):
    return tf_decorator.make_decorator(function, _ModelFnWrapper(function))

  return decorated(target_model_fn) if target_model_fn else decorated


def check_function_argument_count(func, input_arity, infeed_queue):
  """Validate the number of input arguments to an XLA function.

  Args:
    func: the Python function that will be called to generate the body of an XLA
      computation graph.
    input_arity: the number of explicit arguments supplied by the caller.
    infeed_queue: if not None, the infeed queue that will supply
      additional arguments to the function.

  Returns:
    None if function can be called with the supplied number of
      arguments, or an error string if it cannot.
  """
  def format_error(complaint, quantity):
    return '%s %d argument%s' % (complaint, quantity, ''
                                 if quantity == 1 else 's')

  num_args_supplied = input_arity
  if infeed_queue is not None:
    num_args_supplied += infeed_queue.number_of_tuple_elements
  arg_spec = tf_inspect.getargspec(func)
  num_func_args = len(arg_spec.args)
  if arg_spec.defaults is None:
    num_func_defaults = 0
  else:
    num_func_defaults = len(arg_spec.defaults)
  min_func_args = num_func_args - num_func_defaults
  if num_args_supplied < min_func_args:
    # The required number of arguments is not enough to call the function.
    if num_func_defaults == 0 and arg_spec.varargs is None:
      return format_error('exactly', num_func_args)
    else:
      return format_error('at least', min_func_args)
  if arg_spec.varargs is None and num_args_supplied > num_func_args:
    # The required number of arguments is too many to call the function.
    if num_func_defaults == 0:
      return format_error('exactly', num_func_args)
    else:
      return format_error('at most', num_func_args)
  # Reaching here means either
  # 1) There are varargs, func can accept any number of arguments greater than
  # the minimum.
  # 2) Number of supplied arguments falls in range of acceptable argument count
  # of func.
  return None
