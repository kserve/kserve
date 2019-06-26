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
"""Compiled parallel-for loop."""
# pylint: disable=missing-docstring

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import tensor_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import bitwise_ops
from tensorflow.python.ops import check_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import data_flow_ops
from tensorflow.python.ops import functional_ops
from tensorflow.python.ops import gen_parsing_ops
from tensorflow.python.ops import gen_sparse_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn_ops
from tensorflow.python.ops import parsing_ops
from tensorflow.python.ops import sparse_ops
from tensorflow.python.ops import tensor_array_ops
from tensorflow.python.platform import flags
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.util import nest

flags.DEFINE_bool(
    "op_conversion_fallback_to_while_loop", False,
    "If true, falls back to using a while loop for ops for "
    "which a converter is not defined.")


def _stack(t, length):
  """stacks `t` `length` times."""
  ones = array_ops.ones_like(array_ops.shape(t))
  multiples = array_ops.concat([length, ones], 0)
  t = array_ops.tile(array_ops.expand_dims(t, 0), multiples)
  return wrap(t, True)


# The following stateful ops can be safely called once, and with the same
# signature as the unconverted version, if their inputs are loop invariant.
# TODO(agarwal): implement a strategy for converting Variable reads/writes. The
# plan is to map each read/write in the loop_fn to a corresponding merged
# read/write in the converted graph. Writes need to be mergeable (e.g.
# AssignAdd) to be used in `pfor`. Given a certain read/write order in the
# loop_fn, doing a one-to-one conversion will simulate executing such
# instructions in lock-step across all iterations.
passthrough_stateful_ops = set([
    "VariableV2",
    "VarHandleOp",
    "ReadVariableOp",
    "StackV2",
    "TensorArrayWriteV3",
    "TensorArrayReadV3",
    "TensorArraySizeV3",
])


def _is_stateful_pfor_op(op):
  if isinstance(op, WhileOp):
    return op.is_stateful
  if op.type == "Const":
    # Const didn't have an op_def.
    return False
  if op.type in passthrough_stateful_ops:
    return False
  assert hasattr(op, "op_def") and op.op_def is not None, op
  return op.op_def.is_stateful


# pylint: disable=protected-access
class WhileOp(object):
  """Object for storing state for converting the outputs of a while_loop."""

  def __init__(self, exit_node, pfor_ops):
    """Initializer.

    Args:
      exit_node: A tensor output from the while_loop.
      pfor_ops: list of ops inside the current pfor loop.
    """
    self._pfor_ops = set(pfor_ops)
    self._pfor_op_ids = set([x._id for x in pfor_ops])
    assert isinstance(exit_node, ops.Tensor)
    self._while_context = exit_node.op._get_control_flow_context()
    assert isinstance(self._while_context, control_flow_ops.WhileContext)
    self._context_name = self._while_context.name
    self._condition = self._while_context.pivot.op.inputs[0]
    # Parts of an external while_loop could be created inside a pfor loop.
    # However for the purpose here, we declare such loops to be external. Also
    # note that we check if the condition was created inside or outside to
    # determine if the while_loop was first created inside or outside.
    # TODO(agarwal): check that the Enter and Exit of this loop are unstacked.
    self._is_inside_loop = self.op_is_inside_loop(self._condition.op)
    if self._is_inside_loop:
      for e in self._while_context.loop_exits:
        assert self.op_is_inside_loop(e.op)

    # Note the code below tries to reverse engineer an existing while_loop graph
    # by assuming the following pattern of nodes.
    #
    #          NextIteration <---- Body <--- Enter
    #              |                ^
    #              V             ___| Y
    #    Enter -> Merge -> Switch___
    #                       ^       | N
    #                       |       V
    #                  LoopCond    Exit

    # Node that elements in the list below correspond one-to-one with each
    # other. i.e. these lists are the same size, and the i_th entry corresponds
    # to different Operations/Tensors of a single cycle as illustrated above.
    # List of Switch ops (ops.Operation) that feed into an Exit Node.
    self._exit_switches = []
    # List of inputs (ops.Tensor) to NextIteration.
    self._body_outputs = []
    # List of list of control inputs of the NextIteration nodes.
    self._next_iter_control_inputs = []
    # List of Merge ops (ops.Operation).
    self._enter_merges = []
    # List of output (ops.Tensor) of Exit nodes.
    self._outputs = []

    # List of Enter Tensors.
    # There are two types of Enter nodes:
    # - The Enter nodes that are used in the `loop_vars` argument to
    # `while_loop` (see
    # https://www.tensorflow.org/api_docs/python/tf/while_loop). We collect
    # these Enter nodes immediately below by tracing backwards from the Exit
    # nodes via Exit <- Switch <- Merge <- Enter. You can see this chain in the
    # diagram above. This allows us to have a 1:1 correspondence between the
    # self._outputs and the first elements in self._enters.
    # - The Enter nodes that are used only by the body. They don't appear in the
    # `loop_vars` and are not returned from the `while_loop`. In Python code,
    # they are usually captured by the body lambda. We collect them below by
    # iterating over all the ops in the graph. They are appended to the end of
    # self._enters or self._direct_enters, and don't correspond to any outputs
    # in self._outputs. Note that we keep the resource/variant Enter nodes in
    # self._direct_enters and the constructed while_loop's body uses them
    # directly as opposed to passing them as loop variables. This is done
    # because the while_body cannot partition the resource/variant Tensors, so
    # it has to leave them unchanged.
    self._enters = []
    self._direct_enters = []

    for e in self._while_context.loop_exits:
      self._outputs.append(e.op.outputs[0])
      switch = e.op.inputs[0].op
      assert switch.type == "Switch", switch
      self._exit_switches.append(switch)
      merge = switch.inputs[0].op
      assert merge.type == "Merge", merge
      self._enter_merges.append(merge)
      enter = merge.inputs[0].op
      assert enter.type == "Enter", enter
      self._enters.append(enter.outputs[0])
      next_iter = merge.inputs[1].op
      assert next_iter.type == "NextIteration", next_iter
      self._body_outputs.append(next_iter.inputs[0])
      self._next_iter_control_inputs.append(next_iter.control_inputs)

    # Collect all the Enter nodes that are not part of `loop_vars`, the second
    # category described above.
    # Also track whether the loop body has any stateful ops.
    self._is_stateful = False
    for op in ops.get_default_graph().get_operations():
      # TODO(agarwal): make sure this works with nested case.
      control_flow_context = op._get_control_flow_context()
      if control_flow_context is None:
        continue
      if control_flow_context.name == self._context_name:
        self._is_stateful |= _is_stateful_pfor_op(op)
        if op.type == "Enter":
          output = op.outputs[0]
          if output not in self._enters:
            if output.dtype in (dtypes.resource, dtypes.variant):
              if output not in self._direct_enters:
                self._direct_enters.append(output)
            else:
              self._enters.append(output)

  def __str__(self):
    """String representation."""
    return "while_loop(%s)" % self.name

  @property
  def inputs(self):
    """Input to all the Enter nodes."""
    return [x.op.inputs[0] for x in self._enters + self._direct_enters]

  @property
  def control_inputs(self):
    """Control input to all the Enter nodes."""
    control_inputs = []
    for x in self._enters + self._direct_enters:
      control_inputs.extend(x.op.control_inputs)
    return control_inputs

  @property
  def outputs(self):
    """Outputs of all the Exit nodes."""
    return self._outputs

  @property
  def name(self):
    """Context name for the while loop."""
    return self._context_name

  @property
  def is_inside_loop(self):
    """Returns true if the while_loop was created inside the pfor."""
    return self._is_inside_loop

  def op_is_inside_loop(self, op):
    """True if op was created inside the pfor loop body."""
    assert isinstance(op, ops.Operation)
    # Note that we use self._pfor_op_ids for the check and not self._pfor_ops
    # since it appears there tensorflow API could return different python
    # objects representing the same Operation node.
    return op._id in self._pfor_op_ids

  @property
  def is_stateful(self):
    return self._is_stateful

  @property
  def pfor_converter(self):
    """Return a converter for the while loop."""
    return self

  def _init_pfor(self, parent_pfor, indices, cond_stacked, inputs,
                 inputs_stacked):
    """Create a PFor object for converting parts of the while_loop.

    Args:
      parent_pfor: PFor object being used for converting the while_loop.
      indices: int32 Tensor of ids for the iterations that are still active
        (i.e. did not exit the while_loop).
      cond_stacked: True if the while_loop condition is stacked.
      inputs: list of input Tensors corresponding 1-to-1 with self._enters. Note
        that these Tensors are a subset of the loop variables for the generated
        while_loop.
      inputs_stacked: List of booleans corresponding 1-to-1 with `inputs`,
        indicating if the value is stacked or not.

    Returns:
      A PFor instance. The instance is initialized by adding conversion mappings
        of nodes that will be external to the conversion that the returned
        instance will be used for. e.g. Enter nodes as well as Merge and Switch
        outputs are mapped to converted values.
    """
    num_outputs = len(self._outputs)
    assert len(inputs) == len(self._enters)
    assert len(inputs_stacked) == len(self._enters)
    loop_var = parent_pfor.loop_var
    loop_len = array_ops.size(indices)
    pfor = PFor(
        loop_var,
        loop_len,
        pfor_ops=self._pfor_ops,
        all_indices=indices,
        all_indices_partitioned=cond_stacked)
    # Map all inputs of Enter nodes in self._direct_enters to their converted
    # values.
    for enter in self._direct_enters:
      enter_input = enter.op.inputs[0]
      converted_enter, stacked, is_sparse_stacked = parent_pfor._convert_helper(
          enter_input)
      # Since these are resources / variants, they should be unstacked.
      assert not stacked and not is_sparse_stacked, (enter, converted_enter)
      pfor._add_conversion(enter, wrap(converted_enter, False))

    # Map all Enter nodes to the inputs.
    for enter, inp, stacked in zip(self._enters, inputs, inputs_stacked):
      pfor._add_conversion(enter, wrap(inp, stacked))
    # Map outputs of Switch and Merge.
    for i in range(num_outputs):
      wrapped_inp = wrap(inputs[i], inputs_stacked[i])
      merge = self._enter_merges[i]
      pfor._add_conversion(merge.outputs[0], wrapped_inp)
      # Note that second output of Merge is typically not used, except possibly
      # as a control dependency. To avoid trying to output the correct value, we
      # employ a hack here. We output a dummy invalid value with an incorrect
      # dtype. This will allow control dependency to work but if using it as an
      # input, it should typically lead to errors during graph construction due
      # to dtype mismatch.
      # TODO(agarwal): Check in the original graph to see if there are any
      # consumers of this Tensor that use it as an input.
      pfor._add_conversion(merge.outputs[1],
                           wrap(constant_op.constant(-1.0), False))
      switch = self._exit_switches[i]
      # Don't need to worry about switch.output[0] which will feed to Exit node.
      pfor._add_conversion(switch.outputs[1], wrapped_inp)
    return pfor

  def _convert_enter(self, parent_pfor, enter):
    """Converts an Enter node."""
    inp, stacked, _ = parent_pfor._convert_helper(enter.op.inputs[0])
    control_inputs = [
        parent_pfor._convert_helper(x).t for x in enter.op.control_inputs
    ]
    if control_inputs:
      with ops.control_dependencies(control_inputs):
        inp = array_ops.identity(inp)
    return inp, stacked

  def _maybe_stacked(self, cache, inp):
    """Heuristic to figue out if the coverting inp leads to a stacked value.


    Args:
      cache: map from Tensor to boolean indicating stacked/unstacked.
      inp: input Tensor.

    Returns:
      True if `inp` could get stacked. If the function returns False, the
      converted value should be guaranteed to be unstacked. If returning True,
      it may or may not be stacked.
    """
    if inp in cache:
      return cache[inp]
    if not self.op_is_inside_loop(inp.op):
      return False
    op = inp.op
    output = False
    if op.type in [
        "Shape",
        "Rank"
        "ShapeN",
        "ZerosLike",
        "TensorArrayV3",
        "TensorArraySizeV3",
    ]:
      output = False
    elif _is_stateful_pfor_op(op):
      # This may be fairly aggressive.
      output = True
    elif op.type == "Exit":
      # This may be fairly aggressive.
      output = True
    else:
      for t in op.inputs:
        if self._maybe_stacked(cache, t):
          output = True
          break
    cache[inp] = output
    return output

  def _create_init_values(self, pfor_input):
    """Create arguments passed to converted while_loop."""
    with ops.name_scope("while_init"):
      loop_len_vector = pfor_input.pfor.loop_len_vector
      loop_len = loop_len_vector[0]
      num_outputs = len(self._outputs)

      inputs = []
      maybe_stacked_cache = {}
      # Convert all the Enters. Need to do this before checking for stacking
      # below.
      for i, enter in enumerate(self._enters):
        inp, stacked = self._convert_enter(pfor_input.pfor, enter)
        inputs.append(inp)
        maybe_stacked_cache[enter] = stacked
        # Since this enter node is part of the `loop_vars`, it corresponds to an
        # output and its preceding switch. We mark this switch's output the same
        # stackness, to act at the base case for the logic below. Below, we will
        # be going through the body figuring out which inputs might need to be
        # stacked and which inputs can safely remain unstacked.
        if i < num_outputs:
          maybe_stacked_cache[self._exit_switches[i].outputs[1]] = stacked

      # Shape invariants for init_values corresponding to self._enters.
      input_shape_invariants = []
      # TensorArrays for outputs of converted while loop
      output_tas = []
      # Shape invariants for output TensorArrays.
      ta_shape_invariants = []
      # List of booleans indicating stackness of inputs, i.e. tensors
      # corresponding to self._enters.
      inputs_stacked = []
      for i, inp in enumerate(inputs):
        enter = self._enters[i]
        inp_stacked = self._maybe_stacked(maybe_stacked_cache, enter)
        # Note that even when an input is unstacked, the body could make it
        # stacked. we use a heuristic below to figure out if body may be making
        # it stacked.
        if i < num_outputs:
          body_output = self._body_outputs[i]
          if enter.op in self._pfor_ops:
            body_output_stacked = self._maybe_stacked(maybe_stacked_cache,
                                                      body_output)
          else:
            # If constructed outside of pfor loop, then the output would not be
            # stacked.
            body_output_stacked = False
          if body_output_stacked and not inp_stacked:
            inp = _stack(inp, loop_len_vector).t
            inputs[i] = inp
            inp_stacked = True
          # TODO(agarwal): other attributes for the TensorArray ?
          output_tas.append(tensor_array_ops.TensorArray(inp.dtype, loop_len))
          ta_shape_invariants.append(tensor_shape.TensorShape(None))

        inputs_stacked.append(inp_stacked)
        input_shape_invariants.append(tensor_shape.TensorShape(None))

      # See documentation for __call__ for the structure of init_values.
      init_values = [True, pfor_input.pfor.all_indices] + inputs + output_tas
      # TODO(agarwal): try stricter shape invariants
      shape_invariants = (
          [tensor_shape.TensorShape(None),
           tensor_shape.TensorShape(None)
          ] + input_shape_invariants + ta_shape_invariants)

      return init_values, inputs_stacked, shape_invariants

  def _process_cond_unstacked(self, conditions, indices, inputs, output_tas):
    """Handles case when condition is unstacked.

    Note that all iterations end together. So we don't need to partition the
    inputs. When all iterations are done, we write the inputs to the
    TensorArrays. Note that we only write to index 0 of output_tas. Since all
    iterations end together, they can all be output together.
    """
    not_all_done = array_ops.reshape(conditions, [])
    new_output_tas = []
    # pylint: disable=cell-var-from-loop
    for i, out_ta in enumerate(output_tas):
      inp = inputs[i]
      new_output_tas.append(
          control_flow_ops.cond(not_all_done,
                                lambda: out_ta,
                                lambda: out_ta.write(0, inp)))
    # pylint: enable=cell-var-from-loop
    return not_all_done, indices, inputs, new_output_tas

  def _process_cond_stacked(self, conditions, indices, inputs, inputs_stacked,
                            output_tas):
    num_outputs = len(self._outputs)
    # Compute if all iterations are done.
    not_all_done = math_ops.reduce_any(conditions)
    conditions_int = math_ops.cast(conditions, dtypes.int32)
    # Partition the indices.
    done_indices, new_indices = data_flow_ops.dynamic_partition(
        indices, conditions_int, 2)

    new_inputs = []
    new_output_tas = []
    for i, (inp, stacked) in enumerate(zip(inputs, inputs_stacked)):
      # Partition the inputs.
      if stacked:
        done_inp, new_inp = data_flow_ops.dynamic_partition(
            inp, conditions_int, 2)
      else:
        # TODO(agarwal): avoid this stacking. See TODO earlier in
        # _process_cond_unstacked.
        done_inp = _stack(inp, [array_ops.size(done_indices)]).t
        new_inp = inp
      new_inputs.append(new_inp)
      # For iterations that are done, write them to TensorArrays.
      if i < num_outputs:
        out_ta = output_tas[i]
        # Note that done_indices can be empty. done_inp should also be empty in
        # that case.
        new_output_tas.append(out_ta.scatter(done_indices, done_inp))
    return not_all_done, new_indices, new_inputs, new_output_tas

  def _process_body(self, pfor_input, inputs_stacked,
                    new_indices, cond_stacked, new_inputs,
                    not_all_done):
    """Convert the body function."""

    def true_fn(control_inputs, body_pfor, body_output, stacked):
      """Converts the body function for all but last iteration.

      This essentially converts body_output. Additionally, it needs to handle
      any control dependencies on the NextIteration node. So it creates another
      Identity node with the converted dependencies.
      """
      converted_control_inp = []
      for x in control_inputs:
        for t in x.outputs:
          converted_control_inp.append(body_pfor._convert_helper(t).t)
      if stacked:
        # Note convert always does the stacking.
        output = body_pfor.convert(body_output)
      else:
        output, convert_stacked, _ = body_pfor._convert_helper(body_output)
        assert convert_stacked == stacked, body_output
      with ops.control_dependencies(converted_control_inp):
        return array_ops.identity(output)

    body_pfor = self._init_pfor(pfor_input.pfor, new_indices,
                                cond_stacked, new_inputs,
                                inputs_stacked)
    new_outputs = []

    for i, (body_output, stacked) in enumerate(
        zip(self._body_outputs, inputs_stacked)):
      control_inp = self._next_iter_control_inputs[i]
      out_dtype = body_output.dtype
      # Note that we want to run the body only if not all pfor iterations are
      # done. If all are done, we return empty tensors since these values will
      # not be used. Notice that the value returned by the loop is based on
      # TensorArrays and not directly on these returned values.
      # pylint: disable=cell-var-from-loop
      new_output = control_flow_ops.cond(
          not_all_done,
          lambda: true_fn(control_inp, body_pfor, body_output, stacked),
          lambda: constant_op.constant([], dtype=out_dtype))
      # pylint: enable=cell-var-from-loop
      new_outputs.append(new_output)
    return new_outputs

  def __call__(self, pfor_input):
    """Converter for the while_loop.

    The conversion of a while_loop is another while_loop.

    The arguments to this converted while_loop are as follows:
    not_all_done: Boolean scalar Tensor indicating if all the pfor iterations
      are done.
    indices: int32 1-D Tensor storing the id of the iterations that are not
      done.
    args: Remaining arguments. These can be divided into 3 categories:
      - First set of arguments are the tensors that correspond to the initial
        elements of self._enters. The elements that appear in original while
        loop's `loop_vars`.
      - The second set of arguments are the tensors that correspond to the
        remaining elements of self._enters. These are the tensors that directly
        enter the original while loop body.
       - Finally, the last set of arguments are TensorArrays. These TensorArrays
         correspond to the outputs of the original while_loop, i.e. to the
         elements in self._outputs. Each TensorArray has `PFor.loop_len`
         elements, i.e. the number of pfor iterations. At the end, the i'th
         element of each TensorArray will contain the output computed by the
         i'th iteration of pfor. Note that elements can be written into these
         tensors arrays in any order, depending on when the corresponding pfor
         iteration is done.
      If the original while_loop had `k` tensors in its `loop_vars` and its body
      directly captured `m` tensors, the `args` will contain `2 * k + m` values.

    In each iteration, the while_loop body recomputes the condition for all
    active pfor iterations to see which of them are now done. It then partitions
    all the inputs and passes them along to the converted body. Values for all
    the iterations that are done are written to TensorArrays indexed by the pfor
    iteration number. When all iterations are done, the TensorArrays are stacked
    to get the final value.

    Args:
      pfor_input: A PForInput object corresponding to the output of any Exit
        node from this while loop.

    Returns:
      List of converted outputs.
    """
    # Create init_values that will be passed to the while_loop.
    init_values, inputs_stacked, shape_invariants = self._create_init_values(
        pfor_input)
    # Note that we use a list as a hack since we need the nested function body
    # to set the value of cond_is_stacked. python2.x doesn't support nonlocal
    # variables.
    cond_is_stacked = [None]

    def cond(not_all_done, *_):
      return not_all_done

    def body(not_all_done, indices, *args):
      # See documentatin for __call__ for the structure of *args.
      num_enters = len(self._enters)
      inputs = args[:num_enters]
      output_tas = args[num_enters:]
      # TODO(agarwal): see which outputs have consumers and only populate the
      # TensorArrays corresponding to those. Or do those paths get trimmed out
      # from inside the while_loop body?
      assert len(inputs) >= len(output_tas)
      assert len(inputs) == len(inputs_stacked)

      # Convert condition
      with ops.name_scope("while_cond"):
        # Note that we set cond_stacked to True here. At this point we don't
        # know if it could be loop invariant, hence the conservative value is
        # to assume stacked.
        cond_pfor = self._init_pfor(pfor_input.pfor, indices,
                                    cond_stacked=True,
                                    inputs=inputs,
                                    inputs_stacked=inputs_stacked)
        conditions, cond_stacked, _ = cond_pfor._convert_helper(self._condition)
        cond_is_stacked[0] = cond_stacked

      # Recompute the new condition, write outputs of done iterations, and
      # partition the inputs if needed.
      if not cond_stacked:
        (not_all_done, new_indices,
         new_inputs, new_output_tas) = self._process_cond_unstacked(
             conditions, indices, inputs, output_tas)
      else:
        (not_all_done, new_indices,
         new_inputs, new_output_tas) = self._process_cond_stacked(
             conditions, indices, inputs, inputs_stacked, output_tas)

      # Convert body
      with ops.name_scope("while_body"):
        #  Compute the outputs from the body.
        new_outputs = self._process_body(pfor_input, inputs_stacked,
                                         new_indices, cond_stacked, new_inputs,
                                         not_all_done)

      # Note that the first num_outputs new values of inputs are computed using
      # the body. Rest of them were direct Enters into the condition/body and
      # the partitioning done earlier is sufficient to give the new value.
      num_outputs = len(self._outputs)
      new_args = ([not_all_done, new_indices] + new_outputs + list(
          new_inputs[num_outputs:]) + new_output_tas)
      return tuple(new_args)

    while_outputs = control_flow_ops.while_loop(
        cond, body, init_values, shape_invariants=shape_invariants)
    output_tas = while_outputs[-len(self._outputs):]
    outputs = []
    assert cond_is_stacked[0] is not None
    for inp_stacked, ta in zip(inputs_stacked, output_tas):
      if cond_is_stacked[0]:
        outputs.append(wrap(ta.stack(), True))
      else:
        # Note that if while_loop condition is unstacked, all iterations exit at
        # the same time and we wrote those outputs in index 0 of the tensor
        # array.
        outputs.append(wrap(ta.read(0), inp_stacked))
    return outputs


class _PforInput(object):
  """Input object passed to registered pfor converters."""

  def __init__(self, pfor, op, inputs):
    """Creates a _PforInput object.

    Args:
      pfor: PFor converter object.
      op: the Operation object that is being converted.
      inputs: list of WrappedTensor objects representing converted values of the
        inputs of `op`.
    """
    self.pfor = pfor
    self._op = op
    self._inputs = inputs

  def stack_inputs(self, stack_indices=None):
    """Stacks unstacked inputs at `stack_indices`.

    Args:
      stack_indices: indices of inputs at which stacking is done. If None,
        stacking is done at all indices.
    """
    if stack_indices is None:
      stack_indices = range(len(self._inputs))
    length = self.pfor.loop_len_vector
    for i in stack_indices:
      inp = self._inputs[i]
      if not inp.is_stacked:
        self._inputs[i] = _stack(inp.t, length)

  def expanddim_inputs_for_broadcast(self):
    """Reshapes stacked inputs to prepare them for broadcast.

    Since stacked inputs have an extra leading dimension, automatic broadcasting
    rules could incorrectly try to expand dimensions before that leading
    dimension. To avoid that, we reshape these stacked inputs to the maximum
    rank they will need to be broadcasted to.
    """
    if not self._inputs:
      return

    # Find max rank
    def _get_rank(x):
      rank = array_ops.rank(x.t)
      if not x.is_stacked:
        rank += 1
      return rank

    ranks = [_get_rank(x) for x in self._inputs]
    max_rank = ranks[0]
    for rank in ranks[1:]:
      max_rank = math_ops.maximum(rank, max_rank)

    for i, inp in enumerate(self._inputs):
      if inp.is_stacked:
        shape = array_ops.shape(inp.t)
        rank_diff = array_ops.reshape(max_rank - ranks[i], [1])
        ones = array_ops.tile([1], rank_diff)
        new_shape = array_ops.concat([shape[:1], ones, shape[1:]], axis=0)
        self._inputs[i] = wrap(array_ops.reshape(inp.t, new_shape), True)

  @property
  def inputs(self):
    return self._inputs

  @property
  def num_inputs(self):
    return len(self._inputs)

  def input(self, index):
    assert len(self._inputs) > index, (index, self._inputs)
    return self._inputs[index]

  def stacked_input(self, index):
    t, is_stacked, _ = self.input(index)
    if not is_stacked:
      op_type = self.op_type
      op_def = getattr(self._op, "op_def", None)
      if op_def is None:
        input_name = "at index %d" % index
      else:
        input_name = "\"%s\"" % op_def.input_arg[index].name
      raise ValueError("Input %s of op \"%s\" expected to be not loop invariant"
                       ".\nError while converting op %s"
                       "with converted inputs\n%s" % (input_name, op_type,
                                                      self._op, self.inputs))
    return t

  def unstacked_input(self, index):
    t, is_stacked, _ = self.input(index)
    if is_stacked:
      op_type = self.op_type
      op_def = getattr(self._op, "op_def", None)
      if op_def is None:
        input_name = "at index %d" % index
      else:
        input_name = "\"%s\"" % op_def.input_arg[index].name
      raise ValueError("Input %s of op \"%s\" expected to be loop invariant"
                       ".\nError while converting op %s"
                       "with converted inputs\n%s" % (input_name, op_type,
                                                      self._op, self.inputs))
    return t

  @property
  def op(self):
    return self._op

  @property
  def op_type(self):
    return self._op.type

  def get_attr(self, attr):
    return self._op.get_attr(attr)

  @property
  def outputs(self):
    return self._op.outputs

  def output(self, index):
    assert index < len(self._op.outputs)
    return self._op.outputs[index]


_pfor_converter_registry = {}


class RegisterPFor(object):
  """Utility to register converters for pfor.

  Usage:
  @RegisterPFor(foo_op_type)
  def _foo_converter(pfor_input):
    ...

  The above will register conversion function `_foo_converter` for handling
  conversion of `foo_op_type`. During conversion, the registered functin will be
  called with a single argument of type `PForInput` which will contain state
  needed for the conversion.  This registered function should output a list of
  WrappedTensor object with the same length as the number of outputs of op being
  converted. If the op had zero outputs, then it should return a ops.Operation
  object.
  """

  def __init__(self, op_type):
    """Creates an object to register a converter for op with type `op_type`."""
    self.op_type = op_type

  def __call__(self, converter):
    name = self.op_type
    assert name not in _pfor_converter_registry, "Re-registering %s " % name
    _pfor_converter_registry[name] = converter
    return converter


class RegisterPForWithArgs(RegisterPFor):
  """Utility to register converters for pfor.

  Usage:
  @RegisteRPFor(foo_op_type, foo=value, ....)
  def _foo_converter(pfor_input, foo=None, ....):
    ...

  See RegisterPFor for details on the conversion function.
  `RegisterPForWithArgs` allows binding extra arguments to the
  conversion function at registration time.
  """

  def __init__(self, op_type, *args, **kw_args):
    super(RegisterPForWithArgs, self).__init__(op_type)
    self._args = args
    self._kw_args = kw_args

  def __call__(self, converter):

    def _f(pfor_input):
      return converter(pfor_input, self.op_type, *self._args, **self._kw_args)

    super(RegisterPForWithArgs, self).__call__(_f)
    return converter


def _create_op(op_type, inputs, op_dtypes, attrs=None):
  """Utility to create an op."""
  return ops.get_default_graph().create_op(
      op_type, inputs, op_dtypes, attrs=attrs, compute_device=True)


WrappedTensor = collections.namedtuple("WrappedTensor",
                                       ["t", "is_stacked", "is_sparse_stacked"])
"""Wrapper around the result of a Tensor conversion.

The additional fields are useful for keeping track of the conversion state as
data flows through the ops in the loop body. For every op whose output is a
Tensor, its converter should return either a WrappedTensor or a list of
WrappedTensors.

Args:
  t: The converted tensor
  is_stacked: True if the tensor is stacked, i.e. represents the results of all
    the iterations of the loop, where each row i of the tensor corresponds to
    that op's output on iteration i of the loop. False if the tensor is not
    stacked, i.e. represents the result of the op on of a single iteration of
    the loop, where the result does not vary between iterations.
  is_sparse_stacked: True if the tensor corresponds to a component tensor
    (indices, values, or dense_shape) of a sparse tensor, and has been logically
    stacked via a sparse conversion.
"""


def wrap(tensor, is_stacked=True, is_sparse_stacked=False):
  """Helper to create a WrappedTensor object."""
  assert isinstance(is_stacked, bool)
  assert isinstance(is_sparse_stacked, bool)
  assert isinstance(tensor, ops.Tensor)
  assert not is_sparse_stacked or is_stacked, ("If the wrapped tensor is "
                                               "stacked via a sparse "
                                               "conversion, it must also be "
                                               "stacked.")
  return WrappedTensor(tensor, is_stacked, is_sparse_stacked)


def _fallback_converter(pfor_input):
  logging.warn("Using a while_loop for converting %s", pfor_input.op_type)
  output_dtypes = [x.dtype for x in pfor_input.outputs]
  iters = pfor_input.pfor.loop_len_vector[0]

  def while_body(i, *ta_list):
    """Body of while loop."""
    inputs = [
        x[i, ...] if stacked else x for x, stacked, _ in pfor_input.inputs
    ]
    op_outputs = _create_op(
        pfor_input.op_type,
        inputs,
        output_dtypes,
        attrs=pfor_input.op.node_def.attr).outputs

    outputs = []
    for out, ta in zip(op_outputs, ta_list):
      assert isinstance(out, ops.Tensor)
      outputs.append(ta.write(i, array_ops.expand_dims(out, 0)))
    return tuple([i + 1] + outputs)

  ta_list = control_flow_ops.while_loop(
      lambda i, *ta: i < iters, while_body, [0] + [
          tensor_array_ops.TensorArray(dtype, iters) for dtype in output_dtypes
      ])[1:]
  return tuple([wrap(ta.concat(), True) for ta in ta_list])


class PFor(object):
  """Implementation of rewrite of parallel-for loops.

  This class takes a DAG or a set of DAGs representing the body of a
  parallel-for loop, and adds new operations to the graph that implements
  functionality equivalent to running that loop body for a specified number of
  iterations. This new set of nodes may or may not use a tensorflow loop
  construct.

  The process of conversion does not delete or change any existing operations.
  It only adds operations that efficiently implement the equivalent
  functionality. We refer to the added ops as "converted ops".

  The conversion process uses a simple greedy heuristic. It walks the loop body
  and tries to express the functionality of running each node in a loop with a
  new set of nodes. When converting an op several cases are possible:
  - The op is not inside the loop body. Hence it can be used as is.
  - The op does not depend on the iteration number and is stateless. In this
    case, it can be used as is.
  - The op is not stateful, and depends on iteration number only through control
    dependencies. In this case, we can create a single op with same inputs and
    attributes, but with "converted" control dependencies.
  - The op is not stateful, and all its inputs are loop invariant. In this
    case, similar to above, we can create a single op with same inputs and
    attributes, but with "converted" control dependencies.
  - The op is stateful or at least one of the inputs is not loop invariant. In
    this case, we run the registered converter for that op to create a set of
    converted ops. All nodes in the set will have converted control dependencies
    corresponding to control dependencies of the original op. If the op returned
    multiple outputs, "converted outputs" could be produced by different ops in
    this set.
  """

  def __init__(self,
               loop_var,
               loop_len,
               pfor_ops,
               all_indices=None,
               all_indices_partitioned=False):
    """Creates an object to rewrite a parallel-for loop.

    Args:
      loop_var: ops.Tensor output of a Placeholder operation. The value should
        be an int32 scalar representing the loop iteration number.
      loop_len: A scalar or scalar Tensor representing the number of iterations
        the loop is run for.
      pfor_ops: List of all ops inside the loop body.
      all_indices: If not None, an int32 vector with size `loop_len`
        representing the iteration ids that are still active. These values
        should be unique and sorted. However they may not be contiguous. This is
        typically the case when inside a control flow construct which has
        partitioned the indices of the iterations that are being converted.
      all_indices_partitioned: If True, this object is being constructed from a
       control flow construct where not all the pfor iterations are guaranteed
       to be active.
    """
    assert isinstance(loop_var, ops.Tensor)
    assert loop_var.op.type == "Placeholder"
    self._loop_var = loop_var
    loop_len_value = tensor_util.constant_value(loop_len)
    if loop_len_value is not None:
      loop_len = loop_len_value
    self._loop_len_vector = array_ops.reshape(loop_len, [1])
    self._all_indices_partitioned = all_indices_partitioned
    if all_indices_partitioned:
      assert all_indices is not None
    self.all_indices = (
        math_ops.range(loop_len) if all_indices is None else all_indices)

    self._conversion_map = {}
    self._conversion_map[loop_var] = wrap(self.all_indices, True)
    self._pfor_ops = set(pfor_ops)
    self._pfor_op_ids = set([x._id for x in pfor_ops])

  def op_is_inside_loop(self, op):
    """True if op was created inside the pfor loop body."""
    assert isinstance(op, ops.Operation)
    # Note that we use self._pfor_op_ids for the check and not self._pfor_ops
    # since it appears there tensorflow API could return different python
    # objects representing the same Operation node.
    return op._id in self._pfor_op_ids

  def _convert_sparse(self, y):
    """Returns the converted value corresponding to SparseTensor y.

    For SparseTensors, instead of stacking the component tensors separately,
    resulting in component tensors with shapes (N, m, rank), (N, m), and (N,
    rank) respectively for indices, values, and dense_shape (where N is the loop
    length and m is the number of sparse tensor values per loop iter), we want
    to logically stack the SparseTensors, to create a SparseTensor whose
    components are size (N * m, rank + 1), (N * m, ), and (rank + 1,)
    respectively.

    Here, we try to get the conversion of each component tensor.
    If the tensors are stacked via a sparse conversion, return the resulting
    SparseTensor composed of the converted components. Otherwise, the component
    tensors are either unstacked or stacked naively. In the latter case, we
    unstack the component tensors to reform loop_len SparseTensor elements,
    then correctly batch them.

    The unstacked tensors must have the same rank. Each dimension of each
    SparseTensor will expand to be the largest among all SparseTensor elements
    for that dimension. For example, if there are N SparseTensors of rank 3
    being stacked, with N dense shapes, where the i_th shape is (x_i, y_i, z_i),
    the new dense shape will be (N, max_i(x_i), max_i(y_i), max_i(z_i)).

    Args:
      y: A tf.SparseTensor.

    Returns:
      A tf.SparseTensor that is the converted value corresponding to y.
    """
    outputs = [
        self._convert_helper(t) for t in (y.indices, y.values, y.dense_shape)
    ]
    assert all(isinstance(o, WrappedTensor) for o in outputs)

    if all(w.is_sparse_stacked for w in outputs):
      return sparse_tensor.SparseTensor(*[w.t for w in outputs])

    assert not any(w.is_sparse_stacked for w in outputs), (
        "Error converting SparseTensor. All components should be logically "
        "stacked, or none.")

    # If component tensors were not sparsely stacked, they are either unstacked
    # or stacked without knowledge that they are components of sparse tensors.
    # In this case, we have to restack them.
    return self._restack_sparse_tensor_logically(
        *[self._unwrap_or_tile(w) for w in outputs])

  def _restack_sparse_tensor_logically(self, indices, values, shape):
    sparse_tensor_rank = indices.get_shape().dims[-1].value
    if sparse_tensor_rank is not None:
      sparse_tensor_rank += 1

    def map_fn(args):
      res = gen_sparse_ops.serialize_sparse(
          args[0], args[1], args[2], out_type=dtypes.variant)
      return res

    # Applies a map function to the component tensors to serialize each
    # sparse tensor element and batch them all, then deserializes the batch.
    # TODO(rachelim): Try to do this without map_fn -- add the right offsets
    # to shape and indices tensors instead.
    result = functional_ops.map_fn(
        map_fn, [indices, values, shape], dtype=dtypes.variant)
    return sparse_ops.deserialize_sparse(
        result, dtype=values.dtype, rank=sparse_tensor_rank)

  def _unwrap_or_tile(self, wrapped_tensor):
    """Given a wrapped tensor, unwrap if stacked. Otherwise, tiles it."""
    output, is_stacked = wrapped_tensor.t, wrapped_tensor.is_stacked
    if is_stacked:
      return output
    else:
      return _stack(output, self._loop_len_vector).t

  def convert(self, y):
    """Returns the converted value corresponding to y.

    Args:
      y: A ops.Tensor or a ops.Operation object. If latter, y should not have
        any outputs.

    Returns:
      If y does not need to be converted, it returns y as is. Else it returns
      the "converted value" corresponding to y.
    """
    if y is None:
      return None
    if isinstance(y, sparse_tensor.SparseTensor):
      return self._convert_sparse(y)
    output = self._convert_helper(y)
    if isinstance(output, WrappedTensor):
      assert isinstance(y, ops.Tensor)
      return self._unwrap_or_tile(output)
    else:
      assert isinstance(y, ops.Operation)
      assert not y.outputs
      assert isinstance(output, ops.Operation)
    return output

  def _was_converted(self, t):
    """True if t is not a conversion of itself."""
    converted_t = self._conversion_map[t]
    return converted_t.t is not t

  def _add_conversion(self, old_output, new_output):
    self._conversion_map[old_output] = new_output

  def _convert_helper(self, op_or_tensor):
    stack = [op_or_tensor]
    while stack:
      y = stack[0]
      if y in self._conversion_map:
        assert isinstance(self._conversion_map[y],
                          (WrappedTensor, ops.Operation))
        stack.pop(0)
        continue
      if isinstance(y, ops.Operation):
        assert not y.outputs, (
            "We only support converting Operation objects with no outputs. "
            "Got %s", y)
        y_op = y
      else:
        assert isinstance(y, ops.Tensor), y
        y_op = y.op

      is_while_loop = y_op.type == "Exit"
      if is_while_loop:
        while_op = WhileOp(y, pfor_ops=self._pfor_ops)
        is_inside_loop = while_op.is_inside_loop
        # If all nodes in the while_loop graph were created inside the pfor, we
        # treat the whole loop subgraph as a single op (y_op) and try to convert
        # it. For while_loops that are created completely or partially outside,
        # we treat them as external and should be able to simply return the Exit
        # node output as is without needing any conversion. Note that for
        # while_loops that are partially constructed inside, we assume they will
        # be loop invariant. If that is not the case, it will create runtime
        # errors since the converted graph would depend on the self._loop_var
        # placeholder.
        if is_inside_loop:
          y_op = while_op
      else:
        is_inside_loop = self.op_is_inside_loop(y_op)

      # If this op was not created inside the loop body, we will return as is.
      # 1. Convert inputs and control inputs.

      def _add_to_stack(x):
        if x not in self._conversion_map:
          stack.insert(0, x)
          return True
        else:
          return False

      if is_inside_loop:
        added_to_stack = False
        for inp in y_op.inputs:
          added_to_stack |= _add_to_stack(inp)
        for cinp in y_op.control_inputs:
          if cinp.outputs:
            for t in cinp.outputs:
              added_to_stack |= _add_to_stack(t)
          else:
            added_to_stack |= _add_to_stack(cinp)
        if added_to_stack:
          continue

        converted_inputs = [self._conversion_map[inp] for inp in y_op.inputs]
        some_input_converted = any(self._was_converted(x) for x in y_op.inputs)
        some_input_stacked = any(x.is_stacked for x in converted_inputs)

        converted_control_ops = set()
        some_control_input_converted = False
        for cinp in y_op.control_inputs:
          if cinp.outputs:
            for t in cinp.outputs:
              converted_t = self._conversion_map[t]
              if self._was_converted(t):
                some_control_input_converted = True
              converted_control_ops.add(converted_t.t.op)
          else:
            converted_cinp = self._conversion_map[cinp]
            assert isinstance(converted_cinp, ops.Operation)
            if converted_cinp != cinp:
              some_control_input_converted = True
            converted_control_ops.add(converted_cinp)
        converted_control_ops = list(converted_control_ops)
        is_stateful = _is_stateful_pfor_op(y_op)
      else:
        converted_inputs = []
        converted_control_ops = []
      logging.vlog(3, "converting op:%s\ninputs:%s\ncontrol_inputs:%s", y_op,
                   converted_inputs, converted_control_ops)

      # 2. Convert y_op
      # If converting a while_loop, we let the while_loop convertor deal with
      # putting the control dependencies appropriately.
      control_dependencies = [] if is_while_loop else converted_control_ops
      with ops.control_dependencies(control_dependencies), ops.name_scope(
          y_op.name + "/pfor/"):
        # None of the inputs and control inputs were converted.
        if (not is_inside_loop or
            (not is_stateful and not some_input_converted and
             not some_control_input_converted)):
          if y == y_op:
            assert not isinstance(y_op, WhileOp)
            new_outputs = y_op
          else:
            new_outputs = [wrap(x, False) for x in y_op.outputs]
        elif not (is_stateful or is_while_loop or some_input_stacked):
          # All inputs are unstacked or uncoverted but some control inputs are
          # converted.
          # TODO(rachelim): Handle the case where some inputs are sparsely
          # stacked (i.e. any(x.is_sparse_stacked for x in converted_inputs))
          new_op = _create_op(y_op.type, [x.t for x in converted_inputs],
                              [x.dtype for x in y_op.outputs],
                              y_op.node_def.attr)
          if y == y_op:
            new_outputs = new_op
          else:
            new_outputs = [wrap(x, False) for x in new_op.outputs]
        else:
          # Either some inputs are not loop invariant or op is stateful.
          if hasattr(y_op, "pfor_converter"):
            converter = y_op.pfor_converter
          else:
            converter = _pfor_converter_registry.get(y_op.type, None)
          if converter is None:
            if flags.FLAGS.op_conversion_fallback_to_while_loop:
              converter = _fallback_converter
            else:
              raise ValueError(
                  "No converter defined for %s\n%s\ninputs: %s. "
                  "\nEither add a converter or set "
                  "--op_conversion_fallback_to_while_loop=True, "
                  "which may run slower" % (y_op.type, y_op, converted_inputs))
          # TODO(rachelim): Handle the case where some inputs are sparsely
          # stacked. We should only call the converter if it supports handling
          # those inputs.
          new_outputs = converter(_PforInput(self, y_op, converted_inputs))
          if isinstance(new_outputs, WrappedTensor):
            new_outputs = [new_outputs]
          assert isinstance(new_outputs,
                            (list, tuple, ops.Operation)), new_outputs
        logging.vlog(2, "converted %s %s", y_op, new_outputs)

        # Insert into self._conversion_map
        if y == y_op:
          assert isinstance(new_outputs, ops.Operation)
          self._add_conversion(y_op, new_outputs)
        else:
          for old_output, new_output in zip(y_op.outputs, new_outputs):
            assert isinstance(new_output, WrappedTensor), (new_output, y, y_op)
            self._add_conversion(old_output, new_output)
        stack.pop(0)

    return self._conversion_map[op_or_tensor]

  @property
  def loop_len_vector(self):
    """Returns a single element vector whose value is number of iterations."""
    return self._loop_len_vector

  @property
  def loop_var(self):
    """Returns placeholder loop variable."""
    return self._loop_var

  @property
  def pfor_ops(self):
    return self._pfor_ops

  @property
  def all_indices_partitioned(self):
    """all_indices_partitioned property.

    Returns:
      True if we are inside a control flow construct and not all pfor iterations
      may be active.
    """
    return self._all_indices_partitioned

# nn_ops


def _flatten_first_two_dims(x):
  """Merges first two dimensions."""
  old_shape = array_ops.shape(x)
  new_shape = array_ops.concat([[-1], old_shape[2:]], axis=0)
  return array_ops.reshape(x, new_shape)


def _unflatten_first_dim(x, first_dim):
  """Splits first dimension into [first_dim, -1]."""
  old_shape = array_ops.shape(x)
  new_shape = array_ops.concat([first_dim, [-1], old_shape[1:]], axis=0)
  return array_ops.reshape(x, new_shape)


def _inputs_with_flattening(pfor_input, input_indices):
  """Stacks and flattens first dim of inputs at indices `input_indices`."""
  if input_indices is None:
    input_indices = []
  pfor_input.stack_inputs(stack_indices=input_indices)
  inputs = []
  for i in range(pfor_input.num_inputs):
    if i in input_indices:
      inp = pfor_input.stacked_input(i)
      inp = _flatten_first_two_dims(inp)
    else:
      inp = pfor_input.unstacked_input(i)
    inputs.append(inp)
  return inputs


@RegisterPForWithArgs("Conv2D", dims=[0])
@RegisterPForWithArgs("AvgPool", dims=[0])
@RegisterPForWithArgs("MaxPool", dims=[0])
@RegisterPForWithArgs("MaxPool3D", dims=[0])
@RegisterPForWithArgs("MaxPool3DGrad", dims=[0, 1, 2])
@RegisterPForWithArgs("MaxPoolGrad", dims=[0, 1, 2])
@RegisterPForWithArgs("MaxPool3DGradGrad", dims=[0, 1, 2])
@RegisterPForWithArgs("MaxPoolGradGrad", dims=[0, 1, 2])
@RegisterPForWithArgs("SoftmaxCrossEntropyWithLogits", dims=[0, 1])
def _convert_flatten_batch(pfor_input, op_type, dims):
  del op_type
  inputs = _inputs_with_flattening(pfor_input, dims)
  outputs = _create_op(
      pfor_input.op_type,
      inputs, [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  n = pfor_input.pfor.loop_len_vector
  outputs = [_unflatten_first_dim(x, n) for x in outputs]
  return [wrap(x, True) for x in outputs]


_channel_flatten_input_cache = {}


def _channel_flatten_input(x, data_format):
  """Merge the stack dimension with the channel dimension.

  If S is pfor's stacking dimension, then,
    - for SNCHW, we transpose to NSCHW. If N dimension has size 1, the transpose
      should be cheap.
    - for SNHWC, we transpose to NHWCS.
  We then merge the S and C dimension.

  Args:
    x: ops.Tensor to transform.
    data_format: "NCHW" or "NHWC".

  Returns:
    A 3-element tuple with the transformed value, along with the shape for
    reshape and order for transpose required to transform back.
  """

  graph = ops.get_default_graph()
  cache_key = (graph, x, data_format)
  if cache_key not in _channel_flatten_input_cache:
    x_shape = array_ops.shape(x)
    if data_format == b"NCHW":
      order = [1, 0, 2, 3, 4]
      shape = array_ops.concat([x_shape[1:2], [-1], x_shape[3:]], axis=0)
      reverse_order = order
    else:
      order = [1, 2, 3, 0, 4]
      shape = array_ops.concat([x_shape[1:4], [-1]], axis=0)
      reverse_order = [3, 0, 1, 2, 4]
    # Move S dimension next to C dimension.
    x = array_ops.transpose(x, order)
    reverse_shape = array_ops.shape(x)
    # Reshape to merge the S and C dimension.
    x = array_ops.reshape(x, shape)
    outputs = x, reverse_order, reverse_shape
    _channel_flatten_input_cache[cache_key] = outputs
  else:
    outputs = _channel_flatten_input_cache[cache_key]
  return outputs


# Note that with training=True, running FusedBatchNorm on individual examples
# is very different from running FusedBatchNorm on a batch of those examples.
# This is because, for the latter case, the operation can be considered as first
# computing the mean and variance over all the examples and then using these
# to scale all those examples. This creates a data dependency between these
# different "iterations" since the inputs to the scaling step depends on the
# statistics coming from all these inputs.
# As with other kernels, the conversion here effectively runs the kernel
# independently for each iteration, and returns outputs by stacking outputs from
# each of those iterations.
@RegisterPFor("FusedBatchNorm")
def _convert_fused_batch_norm(pfor_input):
  is_training = pfor_input.get_attr("is_training")
  # When BatchNorm is used with training=False, mean and variance are provided
  # externally and used as is by the op. Thus, we can merge the S and N
  # dimensions as we do for regular operations.
  # When BatchNorm is used with training=True, mean and variance are computed
  # for each channel across the batch dimension (first one). If we merge S and N
  # dimensions, mean and variances will be computed over a larger set. So, we
  # merge the S and C dimensions instead.
  if not is_training:
    # We return zeros for batch_mean and batch_variance output. Note that CPU
    # and GPU seem to have different behavior for those two outputs. CPU outputs
    # zero because these values are not used during inference. GPU outputs
    # something, probably real means and variances.
    inputs = _inputs_with_flattening(pfor_input, [0])
    outputs = _create_op(
        pfor_input.op_type,
        inputs, [x.dtype for x in pfor_input.outputs],
        attrs=pfor_input.op.node_def.attr).outputs
    y = outputs[0]
    n = pfor_input.pfor.loop_len_vector
    y = _unflatten_first_dim(y, n)
    mean = pfor_input.unstacked_input(3)
    zeros = array_ops.zeros_like(mean)
    return [wrap(y, True), wrap(zeros, False), wrap(zeros, False)]

  pfor_input.stack_inputs()
  data_format = pfor_input.get_attr("data_format")
  # We merge the first dimension with the "C" dimension, run FusedBatchNorm, and
  # then transpose back.
  x = pfor_input.stacked_input(0)
  x, reverse_order, reverse_shape = _channel_flatten_input(x, data_format)
  # Note that we stack all the other inputs as well so that they are the same
  # size as the new size of the channel dimension.
  inputs = [x] + [
      array_ops.reshape(pfor_input.stacked_input(i), [-1])
      for i in range(1, pfor_input.num_inputs)
  ]
  outputs = _create_op(
      pfor_input.op_type,
      inputs, [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  y = outputs[0]
  y = array_ops.reshape(y, reverse_shape)
  y = array_ops.transpose(y, reverse_order)
  n = pfor_input.pfor.loop_len_vector
  outputs = [_unflatten_first_dim(x, n) for x in outputs[1:]]
  outputs = [y] + outputs
  return [wrap(x, True) for x in outputs]


@RegisterPFor("FusedBatchNormGrad")
def _convert_fused_batch_norm_grad(pfor_input):
  pfor_input.stack_inputs()
  data_format = pfor_input.get_attr("data_format")
  y_backprop = pfor_input.stacked_input(0)
  y_backprop, _, _ = _channel_flatten_input(y_backprop, data_format)
  x = pfor_input.stacked_input(1)
  x, x_reverse_order, x_reverse_shape = _channel_flatten_input(x, data_format)
  inputs = [y_backprop, x] + [
      array_ops.reshape(pfor_input.stacked_input(i), [-1])
      for i in range(2, pfor_input.num_inputs)
  ]
  outputs = _create_op(
      pfor_input.op_type,
      inputs, [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  x_backprop = outputs[0]
  x_backprop = array_ops.reshape(x_backprop, x_reverse_shape)
  x_backprop = array_ops.transpose(x_backprop, x_reverse_order)
  n = pfor_input.pfor.loop_len_vector
  outputs = [_unflatten_first_dim(x, n) for x in outputs[1:]]
  outputs = [x_backprop] + outputs
  return [wrap(output, True) for output in outputs]


@RegisterPForWithArgs("Conv2DBackpropInput", flatten_dims=[2], shape_dim=0)
@RegisterPForWithArgs("AvgPoolGrad", flatten_dims=[1], shape_dim=0)
def _convert_flatten_batch_shape_input(pfor_input, op_type, flatten_dims,
                                       shape_dim):
  del op_type
  inputs = _inputs_with_flattening(pfor_input, flatten_dims)
  n = pfor_input.pfor.loop_len_vector
  # Adjust the `input_sizes` input.
  ones = array_ops.ones(
      [array_ops.shape(inputs[shape_dim])[0] - 1], dtype=n.dtype)
  inputs[shape_dim] *= array_ops.concat([n, ones], axis=0)
  outputs = _create_op(
      pfor_input.op_type,
      inputs, [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  outputs = [_unflatten_first_dim(x, n) for x in outputs]
  return [wrap(x, True) for x in outputs]


@RegisterPFor("Conv2DBackpropFilter")
def _convert_conv2d_backprop_filter(pfor_input):
  pfor_input.stack_inputs(stack_indices=[2])
  inputs, inputs_stacked, _ = pfor_input.input(0)
  filter_sizes = pfor_input.unstacked_input(1)
  grads = pfor_input.stacked_input(2)
  strides = pfor_input.get_attr("strides")
  padding = pfor_input.get_attr("padding")
  use_cudnn_on_gpu = pfor_input.get_attr("use_cudnn_on_gpu")
  data_format = pfor_input.get_attr("data_format")
  dilations = pfor_input.get_attr("dilations")
  if inputs_stacked:
    # TODO(agarwal): Implement this efficiently.
    logging.warn("Conv2DBackpropFilter uses a while_loop. Fix that!")

    def while_body(i, ta):
      inp_i = inputs[i, ...]
      grad_i = grads[i, ...]
      output = nn_ops.conv2d_backprop_filter(
          inp_i,
          filter_sizes,
          grad_i,
          strides=strides,
          padding=padding,
          use_cudnn_on_gpu=use_cudnn_on_gpu,
          data_format=data_format,
          dilations=dilations)
      return i + 1, ta.write(i, array_ops.expand_dims(output, 0))

    n = array_ops.reshape(pfor_input.pfor.loop_len_vector, [])
    _, ta = control_flow_ops.while_loop(
        lambda i, ta: i < n, while_body,
        (0, tensor_array_ops.TensorArray(inputs.dtype, n)))
    output = ta.concat()
    return wrap(output, True)
  else:
    # We merge the stack dimension with the channel dimension of the gradients
    # and pretend we had a larger filter (see change to filter_sizes below).
    # Once the filter backprop is computed, we reshape and transpose back
    # appropriately.
    grads, _, _ = _channel_flatten_input(grads, data_format)
    n = pfor_input.pfor.loop_len_vector
    old_filter_sizes = filter_sizes
    filter_sizes *= array_ops.concat([[1, 1, 1], n], axis=0)
    output = nn_ops.conv2d_backprop_filter(
        inputs,
        filter_sizes,
        grads,
        strides=strides,
        padding=padding,
        use_cudnn_on_gpu=use_cudnn_on_gpu,
        data_format=data_format,
        dilations=dilations)
    new_filter_shape = array_ops.concat([old_filter_sizes[:3], n, [-1]], axis=0)
    output = array_ops.reshape(output, new_filter_shape)
    output = array_ops.transpose(output, [3, 0, 1, 2, 4])
    return wrap(output, True)


# array_ops


@RegisterPForWithArgs("Identity", array_ops.identity)
@RegisterPForWithArgs("StopGradient", array_ops.stop_gradient)
@RegisterPForWithArgs("MatrixDiagPart", array_ops.matrix_diag_part)
def _convert_identity(pfor_input, op_type, op_func):
  del op_type
  return wrap(op_func(*[x.t for x in pfor_input.inputs]), True)


@RegisterPFor("IdentityN")
def _convert_identity_n(pfor_input):
  outputs = array_ops.identity_n([x.t for x in pfor_input.inputs])
  return [wrap(out, inp.is_stacked) for out, inp in
          zip(outputs, pfor_input.inputs)]


@RegisterPFor("Reshape")
def _convert_reshape(pfor_input):
  t = pfor_input.stacked_input(0)
  shape = pfor_input.unstacked_input(1)
  new_dim = array_ops.shape(t)[:1]
  new_shape = array_ops.concat([new_dim, shape], axis=0)
  return wrap(array_ops.reshape(t, new_shape), True)


@RegisterPFor("ExpandDims")
def _convert_expanddims(pfor_input):
  t = pfor_input.stacked_input(0)
  dim = pfor_input.unstacked_input(1)
  dim += math_ops.cast(dim >= 0, dtypes.int32)
  return wrap(array_ops.expand_dims(t, axis=dim), True)


@RegisterPFor("Slice")
def _convert_slice(pfor_input):
  t = pfor_input.stacked_input(0)
  begin = pfor_input.unstacked_input(1)
  size = pfor_input.unstacked_input(2)
  begin = array_ops.concat([[0], begin], axis=0)
  size = array_ops.concat([[-1], size], axis=0)
  return wrap(array_ops.slice(t, begin, size), True)


@RegisterPFor("Tile")
def _convert_tile(pfor_input):
  t = pfor_input.stacked_input(0)
  multiples = pfor_input.unstacked_input(1)
  multiples = array_ops.concat([[1], multiples], 0)
  return wrap(array_ops.tile(t, multiples), True)


@RegisterPFor("Pack")
def _convert_pack(pfor_input):
  pfor_input.stack_inputs()
  axis = pfor_input.get_attr("axis")
  if axis >= 0:
    axis += 1
  return wrap(
      array_ops.stack([x.t for x in pfor_input.inputs], axis=axis), True)


@RegisterPFor("Unpack")
def _convert_unpack(pfor_input):
  value = pfor_input.stacked_input(0)
  axis = pfor_input.get_attr("axis")
  if axis >= 0:
    axis += 1
  num = pfor_input.get_attr("num")
  return [wrap(x, True) for x in array_ops.unstack(value, axis=axis, num=num)]


@RegisterPFor("Pad")
def _convert_pad(pfor_input):
  t = pfor_input.stacked_input(0)
  paddings = pfor_input.unstacked_input(1)
  paddings = array_ops.concat([[[0, 0]], paddings], 0)
  return wrap(array_ops.pad(t, paddings, mode="CONSTANT"), True)


@RegisterPFor("Split")
def _convert_split(pfor_input):
  split_dim = pfor_input.unstacked_input(0)
  t = pfor_input.stacked_input(1)
  num_split = pfor_input.get_attr("num_split")
  split_dim += math_ops.cast(split_dim >= 0, dtypes.int32)
  return [wrap(x, True) for x in array_ops.split(t, num_split, axis=split_dim)]


@RegisterPFor("SplitV")
def _convert_split_v(pfor_input):
  t = pfor_input.stacked_input(0)
  splits = pfor_input.unstacked_input(1)
  split_dim = pfor_input.unstacked_input(2)
  split_dim += math_ops.cast(split_dim >= 0, dtypes.int32)
  return [wrap(x, True) for x in array_ops.split(t, splits, axis=split_dim)]


@RegisterPFor("Transpose")
def _convert_transpose(pfor_input):
  t = pfor_input.stacked_input(0)
  perm = pfor_input.unstacked_input(1)
  new_perm = array_ops.concat([[0], perm + 1], axis=0)
  return wrap(array_ops.transpose(t, new_perm), True)


@RegisterPFor("ZerosLike")
def _convert_zeroslike(pfor_input):
  t = pfor_input.stacked_input(0)
  shape = array_ops.shape(t)[1:]
  return wrap(array_ops.zeros(shape, dtype=t.dtype), False)


@RegisterPFor("Gather")
@RegisterPFor("GatherV2")
def _convert_gather(pfor_input):
  param, param_stacked, _ = pfor_input.input(0)
  indices, indices_stacked, _ = pfor_input.input(1)
  op_type = pfor_input.op_type
  if op_type == "Gather":
    validate_indices = pfor_input.get_attr("validate_indices")
    axis = 0
  else:
    validate_indices = None
    axis = pfor_input.unstacked_input(2)
    axis_value = tensor_util.constant_value(axis)
    if axis_value is not None:
      axis = axis_value
  if indices_stacked and not param_stacked:
    if indices == pfor_input.pfor.all_indices and axis == 0:
      param_shape0 = param.shape.dims[0].value
      indices_shape0 = indices.shape.dims[0].value
      if param_shape0 is not None and indices_shape0 == param_shape0:
        # Note that with loops and conditionals, indices may not be contiguous.
        # However they will be sorted and unique. So if the shape matches, then
        # it must be picking up all the rows of param.
        return wrap(param, True)
      # TODO(agarwal): use array_ops.slice here.
    output = array_ops.gather(
        param, indices, validate_indices=validate_indices, axis=axis)
    if axis != 0:
      axis = control_flow_ops.cond(
          axis < 0, lambda: axis + array_ops.rank(param), lambda: axis)
      order = array_ops.concat(
          [[axis],
           math_ops.range(axis),
           math_ops.range(axis + 1, array_ops.rank(output))],
          axis=0)
      output = control_flow_ops.cond(
          math_ops.equal(axis, 0), lambda: output,
          lambda: array_ops.transpose(output, order))
    return wrap(output, True)
  if param_stacked:
    loop_len_vector = pfor_input.pfor.loop_len_vector
    pfor_input.stack_inputs(stack_indices=[1])
    indices = pfor_input.stacked_input(1)
    param_flat = _flatten_first_two_dims(param)

    # Recompute indices to handle stacked param.
    indices_offset = math_ops.range(
        loop_len_vector[0]) * array_ops.shape(param)[1]
    # Reshape indices_offset to allow broadcast addition
    ones = array_ops.ones([array_ops.rank(indices) - 1], dtype=dtypes.int32)
    new_shape = array_ops.concat([loop_len_vector, ones], axis=0)
    indices_offset = array_ops.reshape(indices_offset, new_shape)
    indices += indices_offset

    # TODO(agarwal): handle axis != 0. May need to transpose param or
    # array_ops.gather_nd.
    if isinstance(axis, ops.Tensor):
      axis_value = tensor_util.constant_value(axis)
    else:
      try:
        axis_value = int(axis)
      except TypeError:
        axis_value = None
    msg = ("Gather, where indices and param are both loop dependent, currently "
           "requires axis=0")
    if axis_value is not None and axis_value != 0:
      raise ValueError("Error while converting %s. %s. Got axis=%d" %
                       (pfor_input.op, msg, axis))
    with ops.control_dependencies(
        [check_ops.assert_equal(axis, 0, message=msg)]):
      output = array_ops.gather(param_flat, indices)
    return wrap(output, True)


@RegisterPFor("ConcatV2")
def _convert_concatv2(pfor_input):
  n = pfor_input.num_inputs
  pfor_input.stack_inputs(stack_indices=range(n - 1))
  axis = pfor_input.unstacked_input(n - 1)
  axis += math_ops.cast(axis >= 0, axis.dtype)
  return wrap(
      array_ops.concat([x.t for x in pfor_input.inputs[:n - 1]], axis=axis),
      True)


@RegisterPFor("StridedSlice")
def _convert_strided_slice(pfor_input):
  inp = pfor_input.stacked_input(0)
  begin = pfor_input.unstacked_input(1)
  end = pfor_input.unstacked_input(2)
  strides = pfor_input.unstacked_input(3)
  begin_mask = pfor_input.get_attr("begin_mask")
  end_mask = pfor_input.get_attr("end_mask")
  ellipsis_mask = pfor_input.get_attr("ellipsis_mask")
  new_axis_mask = pfor_input.get_attr("new_axis_mask")
  shrink_axis_mask = pfor_input.get_attr("shrink_axis_mask")

  begin = array_ops.concat([[0], begin], axis=0)
  end = array_ops.concat([[0], end], axis=0)
  strides = array_ops.concat([[1], strides], axis=0)
  begin_mask = begin_mask << 1 | 1
  end_mask = end_mask << 1 | 1
  ellipsis_mask <<= 1
  new_axis_mask <<= 1
  shrink_axis_mask <<= 1
  return wrap(
      array_ops.strided_slice(
          inp,
          begin,
          end,
          strides,
          begin_mask=begin_mask,
          end_mask=end_mask,
          ellipsis_mask=ellipsis_mask,
          new_axis_mask=new_axis_mask,
          shrink_axis_mask=shrink_axis_mask), True)


@RegisterPFor("StridedSliceGrad")
def _convert_strided_slice_grad(pfor_input):
  shape = pfor_input.unstacked_input(0)
  begin = pfor_input.unstacked_input(1)
  end = pfor_input.unstacked_input(2)
  strides = pfor_input.unstacked_input(3)
  dy = pfor_input.stacked_input(4)
  begin_mask = pfor_input.get_attr("begin_mask")
  end_mask = pfor_input.get_attr("end_mask")
  ellipsis_mask = pfor_input.get_attr("ellipsis_mask")
  new_axis_mask = pfor_input.get_attr("new_axis_mask")
  shrink_axis_mask = pfor_input.get_attr("shrink_axis_mask")

  shape = array_ops.concat([pfor_input.pfor.loop_len_vector, shape], axis=0)
  begin = array_ops.concat([[0], begin], axis=0)
  end = array_ops.concat([[0], end], axis=0)
  strides = array_ops.concat([[1], strides], axis=0)
  begin_mask = begin_mask << 1 | 1
  end_mask = end_mask << 1 | 1
  ellipsis_mask <<= 1
  new_axis_mask <<= 1
  shrink_axis_mask <<= 1
  return wrap(
      array_ops.strided_slice_grad(
          shape,
          begin,
          end,
          strides,
          dy,
          begin_mask=begin_mask,
          end_mask=end_mask,
          ellipsis_mask=ellipsis_mask,
          new_axis_mask=new_axis_mask,
          shrink_axis_mask=shrink_axis_mask), True)


# math_ops


@RegisterPFor("MatMul")
def _convert_matmul(pfor_input):
  # TODO(agarwal): Check if tiling is faster than two transposes.
  a, a_stacked, _ = pfor_input.input(0)
  b, b_stacked, _ = pfor_input.input(1)
  tr_a = pfor_input.get_attr("transpose_a")
  tr_b = pfor_input.get_attr("transpose_b")
  if a_stacked and b_stacked:
    output = wrap(math_ops.matmul(a, b, adjoint_a=tr_a, adjoint_b=tr_b), True)
    return output
  elif a_stacked:
    if tr_a:
      a = array_ops.transpose(a, [0, 2, 1])
    if a.shape.is_fully_defined():
      x, y, z = a.shape
    else:
      x, y, z = [
          array_ops.reshape(i, [])
          for i in array_ops.split(array_ops.shape(a), 3)
      ]
    a = array_ops.reshape(a, [x * y, z])
    prod = math_ops.matmul(a, b, transpose_b=tr_b)
    return wrap(array_ops.reshape(prod, [x, y, -1]), True)
  else:
    assert b_stacked
    if tr_b:
      perm = [2, 0, 1]
      b = array_ops.transpose(b, perm)
    else:
      # As an optimization, if one of the first two dimensions is 1, then we can
      # reshape instead of transpose.
      # TODO(agarwal): This check can be done inside Transpose kernel.
      b_shape = array_ops.shape(b)
      min_dim = math_ops.minimum(b_shape[0], b_shape[1])
      perm = control_flow_ops.cond(
          math_ops.equal(min_dim, 1), lambda: [0, 1, 2], lambda: [1, 0, 2])
      new_shape = array_ops.stack([b_shape[1], b_shape[0], b_shape[2]])
      b = array_ops.transpose(b, perm)
      b = array_ops.reshape(b, new_shape)

    if b.shape.is_fully_defined():
      x, y, z = b.shape
    else:
      x, y, z = [
          array_ops.reshape(i, [])
          for i in array_ops.split(array_ops.shape(b), 3)
      ]
    b = array_ops.reshape(b, [x, y * z])
    prod = math_ops.matmul(a, b, transpose_a=tr_a)
    prod = array_ops.reshape(prod, [-1, y, z])
    prod = array_ops.transpose(prod, [1, 0, 2])
    return wrap(prod, True)


@RegisterPFor("BatchMatMul")
def _convert_batch_mat_mul(pfor_input):
  # TODO(agarwal): There may be a more efficient way to do this instead of
  # stacking the inputs.
  pfor_input.stack_inputs()
  x = pfor_input.stacked_input(0)
  y = pfor_input.stacked_input(1)
  adj_x = pfor_input.get_attr("adj_x")
  adj_y = pfor_input.get_attr("adj_y")

  x = _flatten_first_two_dims(x)
  y = _flatten_first_two_dims(y)
  output = math_ops.matmul(x, y, adjoint_a=adj_x, adjoint_b=adj_y)
  output = _unflatten_first_dim(output, pfor_input.pfor.loop_len_vector)
  return wrap(output, True)


@RegisterPForWithArgs("Sum", math_ops.reduce_sum)
@RegisterPForWithArgs("Prod", math_ops.reduce_prod)
@RegisterPForWithArgs("Max", math_ops.reduce_max)
@RegisterPForWithArgs("Min", math_ops.reduce_min)
def _convert_reduction(pfor_input, _, op_func):
  t = pfor_input.stacked_input(0)
  indices = pfor_input.unstacked_input(1)
  # Shift positive indices by one to account for the extra dimension.
  indices += math_ops.cast(indices >= 0, dtypes.int32)
  keep_dims = pfor_input.get_attr("keep_dims")
  return wrap(op_func(t, indices, keepdims=keep_dims), True)


@RegisterPForWithArgs("Cumsum", math_ops.cumsum)
@RegisterPForWithArgs("Cumprod", math_ops.cumprod)
def _convert_cumfoo(pfor_input, _, op_func):
  t = pfor_input.stacked_input(0)
  axis = pfor_input.unstacked_input(1)
  # Shift positive indices by one to account for the extra dimension.
  axis += math_ops.cast(axis >= 0, dtypes.int32)
  exclusive = pfor_input.get_attr("exclusive")
  reverse = pfor_input.get_attr("reverse")
  return wrap(op_func(t, axis, exclusive=exclusive, reverse=reverse), True)


@RegisterPFor("BiasAdd")
def _convert_biasadd(pfor_input):
  t = pfor_input.stacked_input(0)
  bias = pfor_input.unstacked_input(1)
  data_format = pfor_input.get_attr("data_format")
  if data_format != b"NCHW":
    return wrap(nn_ops.bias_add(t, bias, data_format=data_format), True)
  shape = array_ops.shape(t)
  flattened_shape = array_ops.concat([[-1], shape[2:]], axis=0)
  t = array_ops.reshape(t, flattened_shape)
  t = nn_ops.bias_add(t, bias, data_format=b"NCHW")
  t = array_ops.reshape(t, shape)
  return wrap(t, True)


@RegisterPFor("UnsortedSegmentSum")
def _convert_unsortedsegmentsum(pfor_input):
  data, data_stacked, _ = pfor_input.input(0)
  # TODO(agarwal): handle unstacked?
  segment_ids = pfor_input.stacked_input(1)
  # TODO(agarwal): handle stacked?
  num_segments = pfor_input.unstacked_input(2)
  if not data_stacked:
    data = _stack(data, pfor_input.pfor.loop_len_vector).t
  segment_shape = array_ops.shape(segment_ids)
  n = segment_shape[0]
  ones = array_ops.ones_like(segment_shape)[1:]
  segment_offset = num_segments * math_ops.range(n)
  segment_offset = array_ops.reshape(segment_offset,
                                     array_ops.concat([[n], ones], axis=0))
  segment_ids += segment_offset
  num_segments = math_ops.cast(num_segments, dtypes.int64) * math_ops.cast(
      n, dtypes.int64)
  output = math_ops.unsorted_segment_sum(data, segment_ids, num_segments)
  new_output_shape = array_ops.concat(
      [[n, -1], array_ops.shape(output)[1:]], axis=0)
  output = array_ops.reshape(output, new_output_shape)
  return wrap(output, True)


@RegisterPFor("Cast")
def _convert_cast(pfor_input):
  inp = pfor_input.stacked_input(0)
  dtype = pfor_input.get_attr("DstT")
  return wrap(math_ops.cast(inp, dtype), True)


@RegisterPForWithArgs("Abs", math_ops.abs)
@RegisterPForWithArgs("Acosh", math_ops.acosh)
@RegisterPForWithArgs("Acos", math_ops.acos)
@RegisterPForWithArgs("Add", math_ops.add)
@RegisterPForWithArgs("AddV2", math_ops.add_v2)
@RegisterPForWithArgs("Angle", math_ops.angle)
@RegisterPForWithArgs("Asinh", math_ops.asinh)
@RegisterPForWithArgs("Asin", math_ops.asin)
@RegisterPForWithArgs("Atan2", math_ops.atan2)
@RegisterPForWithArgs("Atanh", math_ops.atanh)
@RegisterPForWithArgs("Atan", math_ops.atan)
@RegisterPForWithArgs("BesselI0e", math_ops.bessel_i0e)
@RegisterPForWithArgs("BesselI1e", math_ops.bessel_i1e)
@RegisterPForWithArgs("BitwiseAnd", bitwise_ops.bitwise_and)
@RegisterPForWithArgs("BitwiseOr", bitwise_ops.bitwise_or)
@RegisterPForWithArgs("BitwiseXor", bitwise_ops.bitwise_xor)
@RegisterPForWithArgs("Ceil", math_ops.ceil)
@RegisterPForWithArgs("ComplexAbs", math_ops.complex_abs)
@RegisterPForWithArgs("Complex", math_ops.complex)
@RegisterPForWithArgs("Conj", math_ops.conj)
@RegisterPForWithArgs("Cosh", math_ops.cosh)
@RegisterPForWithArgs("Cos", math_ops.cos)
@RegisterPForWithArgs("Digamma", math_ops.digamma)
@RegisterPForWithArgs("Div", math_ops.div)
@RegisterPForWithArgs("DivNoNan", math_ops.div_no_nan)
@RegisterPForWithArgs("Elu", nn_ops.elu)
@RegisterPForWithArgs("Equal", math_ops.equal)
@RegisterPForWithArgs("Erfc", math_ops.erfc)
@RegisterPForWithArgs("Erf", math_ops.erf)
@RegisterPForWithArgs("Expm1", math_ops.expm1)
@RegisterPForWithArgs("Exp", math_ops.exp)
@RegisterPForWithArgs("FloorDiv", math_ops.floor_div)
@RegisterPForWithArgs("Floor", math_ops.floor)
@RegisterPForWithArgs("FloorMod", math_ops.floor_mod)
@RegisterPForWithArgs("GreaterEqual", math_ops.greater_equal)
@RegisterPForWithArgs("Greater", math_ops.greater)
@RegisterPForWithArgs("Igammac", math_ops.igammac)
@RegisterPForWithArgs("IgammaGradA", math_ops.igamma_grad_a)
@RegisterPForWithArgs("Igamma", math_ops.igamma)
@RegisterPForWithArgs("Imag", math_ops.imag)
@RegisterPForWithArgs("Invert", bitwise_ops.invert)
@RegisterPForWithArgs("Inv", math_ops.inv)
@RegisterPForWithArgs("IsFinite", math_ops.is_finite)
@RegisterPForWithArgs("IsInf", math_ops.is_inf)
@RegisterPForWithArgs("LeftShift", bitwise_ops.left_shift)
@RegisterPForWithArgs("LessEqual", math_ops.less_equal)
@RegisterPForWithArgs("Less", math_ops.less)
@RegisterPForWithArgs("Lgamma", math_ops.lgamma)
@RegisterPForWithArgs("Log1p", math_ops.log1p)
@RegisterPForWithArgs("LogicalAnd", math_ops.logical_and)
@RegisterPForWithArgs("LogicalNot", math_ops.logical_not)
@RegisterPForWithArgs("LogicalOr", math_ops.logical_or)
@RegisterPForWithArgs("LogicalXor", math_ops.logical_xor)
@RegisterPForWithArgs("Log", math_ops.log)
@RegisterPForWithArgs("Maximum", math_ops.maximum)
@RegisterPForWithArgs("Minimum", math_ops.minimum)
@RegisterPForWithArgs("Mod", math_ops.mod)
@RegisterPForWithArgs("Mul", math_ops.multiply)
@RegisterPForWithArgs("Neg", math_ops.negative)
@RegisterPForWithArgs("NotEqual", math_ops.not_equal)
@RegisterPForWithArgs("Polygamma", math_ops.polygamma)
@RegisterPForWithArgs("Pow", math_ops.pow)
@RegisterPForWithArgs("RealDiv", math_ops.divide)
@RegisterPForWithArgs("Real", math_ops.real)
@RegisterPForWithArgs("Reciprocal", math_ops.reciprocal)
@RegisterPForWithArgs("Relu6", nn_ops.relu6)
@RegisterPForWithArgs("Relu", nn_ops.relu)
@RegisterPForWithArgs("RightShift", bitwise_ops.right_shift)
@RegisterPForWithArgs("Rint", math_ops.rint)
@RegisterPForWithArgs("Round", math_ops.round)
@RegisterPForWithArgs("Rsqrt", math_ops.rsqrt)
@RegisterPForWithArgs("Selu", nn_ops.selu)
@RegisterPForWithArgs("Sigmoid", math_ops.sigmoid)
@RegisterPForWithArgs("Sign", math_ops.sign)
@RegisterPForWithArgs("Sinh", math_ops.sinh)
@RegisterPForWithArgs("Sin", math_ops.sin)
@RegisterPForWithArgs("Softplus", nn_ops.softplus)
@RegisterPForWithArgs("Softsign", nn_ops.softsign)
@RegisterPForWithArgs("Sqrt", math_ops.sqrt)
@RegisterPForWithArgs("SquaredDifference", math_ops.squared_difference)
@RegisterPForWithArgs("Square", math_ops.square)
@RegisterPForWithArgs("Sub", math_ops.subtract)
@RegisterPForWithArgs("Tanh", math_ops.tanh)
@RegisterPForWithArgs("Tan", math_ops.tan)
@RegisterPForWithArgs("TruncateDiv", math_ops.truncate_div)
@RegisterPForWithArgs("TruncateMod", math_ops.truncate_mod)
@RegisterPForWithArgs("Zeta", math_ops.zeta)
def _convert_cwise(pfor_input, op_type, op_func):
  # Note that ops handled here do not have attributes except "T" and "Tout", and
  # hence don't need extra arguments passed to the cwise_op call below.
  for attr in pfor_input.op.node_def.attr.keys():
    assert attr in [u"T", u"Tout"], (op_type, attr)
  pfor_input.expanddim_inputs_for_broadcast()
  return wrap(op_func(*[x.t for x in pfor_input.inputs]), True)


@RegisterPFor("ApproximateEqual")
def _convert_approximate_equal(pfor_input):
  pfor_input.expanddim_inputs_for_broadcast()
  x = pfor_input.input(0)[0]
  y = pfor_input.input(1)[0]
  tolerance = pfor_input.get_attr("tolerance")
  return wrap(math_ops.approximate_equal(x, y, tolerance=tolerance), True)


@RegisterPFor("Shape")
def _convert_shape(pfor_input):
  out_type = pfor_input.get_attr("out_type")
  return wrap(
      array_ops.shape(pfor_input.stacked_input(0), out_type=out_type)[1:],
      False)


@RegisterPFor("ShapeN")
def _convert_shape_n(pfor_input):
  out_type = pfor_input.get_attr("out_type")
  shapes = [
      array_ops.shape(x, out_type=out_type)[1:]
      if stacked else array_ops.shape(x) for x, stacked, _ in pfor_input.inputs
  ]
  return [wrap(x, False) for x in shapes]


@RegisterPFor("Size")
def _convert_size(pfor_input):
  out_type = pfor_input.get_attr("out_type")
  n = math_ops.cast(pfor_input.pfor.loop_len_vector[0], out_type)
  return wrap(
      array_ops.size(pfor_input.stacked_input(0), out_type=out_type) // n,
      False)


@RegisterPFor("Rank")
def _convert_rank(pfor_input):
  return wrap(array_ops.rank(pfor_input.stacked_input(0)) - 1, False)


@RegisterPFor("AddN")
def _convert_addn(pfor_input):
  # AddN does not support broadcasting.
  pfor_input.stack_inputs()
  return wrap(math_ops.add_n([x.t for x in pfor_input.inputs]), True)


@RegisterPFor("BiasAddGrad")
def _convert_biasaddgrad(pfor_input):
  grad = pfor_input.stacked_input(0)
  fmt = pfor_input.get_attr("data_format")
  if fmt == b"NCHW":
    output = math_ops.reduce_sum(grad, axis=[1, 3, 4], keepdims=False)
  else:
    grad_shape = array_ops.shape(grad)
    last_dim_shape = grad_shape[-1]
    first_dim_shape = grad_shape[0]
    output = array_ops.reshape(grad, [first_dim_shape, -1, last_dim_shape])
    output = math_ops.reduce_sum(output, axis=[1], keepdims=False)
  return wrap(output, True)


# Some required ops are not exposed under the tf namespace. Hence relying on
# _create_op to create them.
@RegisterPForWithArgs("EluGrad")
@RegisterPForWithArgs("Relu6Grad")
@RegisterPForWithArgs("ReluGrad")
@RegisterPForWithArgs("SeluGrad")
@RegisterPForWithArgs("SigmoidGrad")
@RegisterPForWithArgs("SoftplusGrad")
@RegisterPForWithArgs("SoftsignGrad")
@RegisterPForWithArgs("TanhGrad")
@RegisterPForWithArgs("SqrtGrad")
@RegisterPForWithArgs("RsqrtGrad")
@RegisterPForWithArgs("ReciprocalGrad")
def _convert_grads(pfor_input, op_type, *args, **kw_args):
  del args
  del kw_args
  # TODO(agarwal): Looks like these ops don't support broadcasting. Hence we
  # have to use tiling here.
  pfor_input.stack_inputs()
  outputs = _create_op(
      op_type, [x.t for x in pfor_input.inputs],
      [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  return [wrap(x, True) for x in outputs]


@RegisterPFor("Select")
def _convert_select(pfor_input):
  pfor_input.stack_inputs()
  cond = pfor_input.stacked_input(0)
  t = pfor_input.stacked_input(1)
  e = pfor_input.stacked_input(2)
  cond_rank = array_ops.rank(cond)
  cond, t, e = control_flow_ops.cond(
      cond_rank > 1, lambda: _inputs_with_flattening(pfor_input, [0, 1, 2]),
      lambda: [cond, t, e])
  outputs = _create_op(
      pfor_input.op_type, [cond, t, e], [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  n = pfor_input.pfor.loop_len_vector
  out = control_flow_ops.cond(cond_rank > 1,
                              lambda: _unflatten_first_dim(outputs[0], n),
                              lambda: outputs[0])
  return [wrap(out, True) for x in outputs]


# random_ops


@RegisterPForWithArgs("RandomUniform")
@RegisterPForWithArgs("RandomUniformInt")
@RegisterPForWithArgs("RandomStandardNormal")
@RegisterPForWithArgs("TruncatedNormal")
@RegisterPForWithArgs("RandomGamma")
@RegisterPForWithArgs("RandomPoissonV2")
def _convert_random(pfor_input, op_type, *args, **kw_args):
  del args
  del kw_args
  inputs = [pfor_input.unstacked_input(i) for i in range(pfor_input.num_inputs)]
  # inputs[0] is "shape"
  inputs[0] = array_ops.concat(
      [pfor_input.pfor.loop_len_vector, inputs[0]], axis=0)
  logging.warning(
      "Note that %s inside pfor op may not give same output as "
      "inside a sequential loop.", op_type)
  outputs = _create_op(
      op_type,
      inputs, [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  return [wrap(x, True) for x in outputs]


# logging_ops


@RegisterPFor("Assert")
def _convert_assert(pfor_input):
  cond, cond_stacked, _ = pfor_input.input(0)
  if cond_stacked:
    cond = math_ops.reduce_all(cond)

  data_list = [x.t for x in pfor_input.inputs][1:]
  return _create_op("Assert", [cond] + data_list, [],
                    attrs=pfor_input.op.node_def.attr)


@RegisterPFor("Print")
def _convert_print(pfor_input):
  # Note that we don't stack all the inputs. Hence unstacked values are printed
  # once here vs multiple times in a while_loop.
  pfor_input.stack_inputs([0])
  outputs = _create_op(
      "Print", [x.t for x in pfor_input.inputs],
      [x.dtype for x in pfor_input.outputs],
      attrs=pfor_input.op.node_def.attr).outputs
  return [wrap(x, True) for x in outputs]


# data_flow_ops

# TensorArray conversion is tricky since we don't support arrays of
# TensorArrays. For converting them, we consider two distinct cases:
#
# 1. The array is constructed outside the pfor call, and read/written inside the
# loop.
# This is an easier case since we don't need to make an array of TensorArrays.
# A correctness requirement is that these parallel iterations shouldn't attempt
# to write to the same location. Hence at conversion time we disallow indices to
# be loop-invariant as that would guarantee a collision. Even if the indices are
# not loop-invariant, they could conflict and that shall trigger runtime errors.
#
# 2. The array is constructed and used entirely inside each pfor iteration.
# For simplicity, here we require that the indices used for write/scatter are
# "unstacked". Otherwise it becomes hard to merge the TensorArrays created in
# different pfor iterations. We consider two sub_cases:
#
# 2a Elements written to the array are "stacked"
# To simulate multiple TensorArrays, we may increase the dimension of each
# element of the array. i.e. the i_th row of the j_th entry of the converted
# TensorArray corresponds to the j_th entry of the TensorArray in the i_th
# pfor iteration.
#
# 2b Elements written to the array are "unstacked"
# In this case we don't increase the dimensions to avoid redundant tiling. Each
# iteration is trying to write the same value. So we convert that to a single
# write.
#
# Here are some tricks used to implement the above:
# - TensorArrayV3 constructor encodes the element shape as an attr. Instead of
# trying to trace whether future writes are stacked or unstacked in order to set
# this attr, we set it to correspond to unknown shape.
# - We use the "flow" output of the different ops to track whether the array
# elements are stacked or unstacked. If a stacked write/scatter is done, we make
# the flow stacked as well.
# - We use some heuristic traversal of the graph to track whether the
# TensorArray handle was created inside or outside the pfor loop.


@RegisterPFor("TensorArrayV3")
def _convert_tensor_array_v3(pfor_input):
  size = pfor_input.unstacked_input(0)
  dtype = pfor_input.get_attr("dtype")
  dynamic_size = pfor_input.get_attr("dynamic_size")
  clear_after_read = pfor_input.get_attr("clear_after_read")
  identical_element_shapes = pfor_input.get_attr("identical_element_shapes")
  tensor_array_name = pfor_input.get_attr("tensor_array_name")
  handle, flow = data_flow_ops.tensor_array_v3(
      size,
      dtype=dtype,
      # We don't set element shape since we don't know if writes are stacked or
      # not yet.
      element_shape=None,
      dynamic_size=dynamic_size,
      clear_after_read=clear_after_read,
      identical_element_shapes=identical_element_shapes,
      tensor_array_name=tensor_array_name)
  # Note we keep flow unstacked for now since we don't know if writes will be
  # stacked or not.
  return wrap(handle, False), wrap(flow, False)


@RegisterPFor("TensorArraySizeV3")
def _convert_tensor_array_size_v3(pfor_input):
  handle = pfor_input.unstacked_input(0)
  flow, flow_stacked, _ = pfor_input.input(1)
  if flow_stacked:
    flow = _unstack_flow(flow)
  size = data_flow_ops.tensor_array_size_v3(handle, flow)
  return wrap(size, False)


def _handle_inside_pfor(pfor_input, handle):
  """Returns True if handle was created inside the pfor loop."""
  # We use some heuristic to find the original TensorArray creation op.
  # The logic should handle the common cases (except cond based subgraphs).
  # In theory the user could perform different operations on the handle (like
  # Reshape, stack multiple handles, etc) which could break this logic.
  # TODO(agarwal): handle Switch/Merge.
  while handle.op.type in ("Enter", "Identity"):
    handle = handle.op.inputs[0]
  if handle.op.type not in [
      "TensorArrayV3", "TensorArrayGradV3", "TensorArrayGradWithShape"]:
    raise ValueError("Unable to find source for handle %s" % handle)
  else:
    return pfor_input.pfor.op_is_inside_loop(handle.op)


def _unstack_flow(value):
  # TODO(agarwal): consider looking if this is a Tile op then get its input.
  # This may avoid running the Tile operations.
  return array_ops.gather(value, 0)


@RegisterPFor("TensorArrayReadV3")
def _convert_tensor_array_read_v3(pfor_input):
  handle = pfor_input.unstacked_input(0)
  index, index_stacked, _ = pfor_input.input(1)
  dtype = pfor_input.get_attr("dtype")
  flow, flow_stacked, _ = pfor_input.input(2)
  if flow_stacked:
    flow = _unstack_flow(flow)

  is_inside_pfor = _handle_inside_pfor(pfor_input, pfor_input.op.inputs[0])
  if is_inside_pfor:
    # Note that if we are inside a control flow construct inside the pfor, and
    # only some of the iterations are doing the read (i.e.
    # `all_indices_partitioned` is True), then the read operation should only
    # return values for the currently active pfor iterations (`all_indices`
    # below). Hence, whenever the returned value is stacked (i.e. `flow` is
    # stacked), we may need to do an extra gather after reading the values. Also
    # note that if `is_inside` is false, then values in the tensor array are
    # unstacked. So the check is only needed in this branch.
    all_indices = pfor_input.pfor.all_indices
    all_indices_partitioned = pfor_input.pfor.all_indices_partitioned
    # Note: flow_stacked indicates if values in the TensorArray are stacked or
    # not.
    if index_stacked:
      if flow_stacked:
        raise ValueError(
            "It looks like TensorArrayReadV3 was called on a TensorArray whose"
            " values are not loop-invariant, and the read indices were also"
            " not loop invariant. This is currently unsupported.")
      value = data_flow_ops.tensor_array_gather_v3(
          handle, index, flow, dtype=dtype)
      return wrap(value, True)
    value = data_flow_ops.tensor_array_read_v3(
        handle, index, flow, dtype=dtype)
    if flow_stacked and all_indices_partitioned:
      value = array_ops.gather(value, all_indices)
    return wrap(value, flow_stacked)
  # Values in the TensorArray should be unstacked (since different iterations
  # couldn't write to the same location). So whether output is stacked or not
  # depends on index_stacked.
  if index_stacked:
    value = data_flow_ops.tensor_array_gather_v3(
        handle, index, flow, dtype=dtype)
  else:
    value = data_flow_ops.tensor_array_read_v3(
        handle, index, flow, dtype=dtype)
  return wrap(value, index_stacked)


@RegisterPFor("TensorArrayWriteV3")
def _convert_tensor_array_write_v3(pfor_input):
  handle = pfor_input.unstacked_input(0)
  index, index_stacked, _ = pfor_input.input(1)
  value, value_stacked, _ = pfor_input.input(2)
  flow, flow_stacked, _ = pfor_input.input(3)
  if value_stacked and pfor_input.pfor.all_indices_partitioned:
    # Looks like we are in a control flow in a pfor where not all iterations are
    # active now. We don't allow that since that could lead to different indices
    # having different shapes which will be hard to merge later.
    raise ValueError("Writing non loop invariant values to TensorArray from "
                     "inside a while_loop/cond not supported.")
  if flow_stacked:
    flow = _unstack_flow(flow)
  is_inside = _handle_inside_pfor(pfor_input, pfor_input.op.inputs[0])
  if is_inside:
    if index_stacked:
      raise ValueError("Need indices for %s to be loop invariant" % handle)
    if not flow_stacked and not value_stacked:
      flow_out = data_flow_ops.tensor_array_write_v3(handle, index, value, flow)
      return wrap(flow_out, False)
    else:
      if not value_stacked:
        value = _stack(value, pfor_input.pfor.loop_len_vector).t
      # TODO(agarwal): Note that if flow is unstacked and value is stacked, then
      # this may or may not be a safe situation. flow is unstacked both for a
      # freshly created TensorArray, as well as after unstacked values are
      # written to it. If it is the latter, then we cannot write a stacked value
      # now since that may cause runtime errors due to different shapes in the
      # array. At the moment we are not able to handle this gracefully and
      # distinguish between the two cases. That would require some heuristic
      # traversal of the graph to figure out whether all the writes are
      # unstacked or not.
      flow_out = data_flow_ops.tensor_array_write_v3(handle, index, value, flow)
      return _stack(flow_out, pfor_input.pfor.loop_len_vector)
  else:
    if not index_stacked:
      raise ValueError("Need indices for %s to be not loop invariant" % handle)
    # Note that even when index_stacked is true, actual values in index may
    # still not be unique. However that will cause runtime error when executing
    # the scatter operation below.
    if not value_stacked:
      value = _stack(value, pfor_input.pfor.loop_len_vector).t
    flow_out = data_flow_ops.tensor_array_scatter_v3(handle, index, value, flow)
    return _stack(flow_out, pfor_input.pfor.loop_len_vector)


def _transpose_first_two_dims(value):
  # TODO(agarwal): optimize if one of the dims == 1.
  value_shape = array_ops.shape(value)
  v0 = value_shape[0]
  v1 = value_shape[1]
  value = array_ops.reshape(value, [v0, v1, -1])
  value = array_ops.transpose(value, [1, 0, 2])
  new_shape = array_ops.concat([[v1, v0], value_shape[2:]], axis=0)
  return array_ops.reshape(value, new_shape)


@RegisterPFor("TensorArrayGatherV3")
def _convert_tensor_array_gather_v3(pfor_input):
  handle = pfor_input.unstacked_input(0)
  indices, indices_stacked, _ = pfor_input.input(1)
  indices = array_ops.reshape(indices, [-1])
  flow, flow_stacked, _ = pfor_input.input(2)
  if flow_stacked:
    flow = _unstack_flow(flow)
  dtype = pfor_input.get_attr("dtype")
  # TODO(agarwal): support element_shape attr?

  n = pfor_input.pfor.loop_len_vector
  value = data_flow_ops.tensor_array_gather_v3(
      handle, indices, flow, dtype=dtype)
  is_inside = _handle_inside_pfor(pfor_input, pfor_input.op.inputs[0])
  if is_inside:
    # flow_stacked indicates if values in the TensorArray are stacked or not.
    if indices_stacked:
      if flow_stacked:
        raise ValueError(
            "It looks like TensorArrayGatherV3 was called on a TensorArray "
            "whose values are not loop-invariant, and the indices were also "
            "not loop invariant. This is currently unsupported.")
      else:
        value = _unflatten_first_dim(value, n)
        return wrap(value, True)
    else:
      if flow_stacked:
        # Since elements in this array are stacked and `value` was produced by
        # gather, its first two dims are "gathered elements" and "stack
        # dimension". Our semantics require these two to be flipped.
        value = _transpose_first_two_dims(value)
      return wrap(value, flow_stacked)
  else:
    # Values in the TensorArray should be unstacked (since different iterations
    # couldn't write to the same location). So whether output is stacked or not
    # depends on indices_stacked.
    if indices_stacked:
      value = _unflatten_first_dim(value, n)
    return wrap(value, indices_stacked)


@RegisterPFor("TensorArrayScatterV3")
def _convert_tensor_array_scatter_v3(pfor_input):
  handle = pfor_input.unstacked_input(0)
  indices, indices_stacked, _ = pfor_input.input(1)
  indices = array_ops.reshape(indices, [-1])
  value, value_stacked, _ = pfor_input.input(2)
  flow, flow_stacked, _ = pfor_input.input(3)

  if flow_stacked:
    flow = _unstack_flow(flow)

  is_inside = _handle_inside_pfor(pfor_input, pfor_input.op.inputs[0])
  if is_inside:
    if indices_stacked:
      raise ValueError("Need indices for %s to be loop invariant" % handle)
    # Note that flow_stacked indicates if existing values in the array are
    # stacked or not.
    if not flow_stacked and not value_stacked:
      flow_out = data_flow_ops.tensor_array_scatter_v3(handle, indices, value,
                                                       flow)
      return wrap(flow_out, False)
    if not value_stacked:
      # TODO(agarwal): tile in the second dimension directly instead of
      # transposing below.
      value = _stack(value, pfor_input.pfor.loop_len_vector).t

    value = _transpose_first_two_dims(value)
    # TODO(agarwal): Note that if a previous write was unstacked, flow will be
    # unstacked, and a stacked value may be written here which may cause
    # runtime error due to different elements having different shape. We do
    # not try to prevent that.
    flow_out = data_flow_ops.tensor_array_scatter_v3(handle, indices, value,
                                                     flow)
    return _stack(flow_out, pfor_input.pfor.loop_len_vector)
  if not indices_stacked:
    raise ValueError("Need indices for %s to be not loop invariant" % handle)
  if not value_stacked:
    value = _stack(value, pfor_input.pfor.loop_len_vector).t
  value = _flatten_first_two_dims(value)
  flow_out = data_flow_ops.tensor_array_scatter_v3(handle, indices, value,
                                                   flow)
  return _stack(flow_out, pfor_input.pfor.loop_len_vector)


@RegisterPFor("TensorArrayGradV3")
def _convert_tensor_array_grad_v3(pfor_input):
  handle = pfor_input.unstacked_input(0)
  flow, flow_stacked, _ = pfor_input.input(1)
  if flow_stacked:
    flow = _unstack_flow(flow)
  source = pfor_input.get_attr("source")
  # TODO(agarwal): For now, we assume that gradients are stacked if the
  # TensorArrayGradV3 call is being done inside the pfor. Getting that wrong
  # will give runtime error due to incorrect shape being written to the
  # accumulator. It is difficult to know in advance if gradients written will be
  # stacked or not. Note that flow being stacked is not indicative of the
  # gradient being stacked or not. Revisit this later.
  shape_to_prepend = pfor_input.pfor.loop_len_vector
  grad_handle, flow_out = data_flow_ops.tensor_array_grad_with_shape(
      handle=handle,
      flow_in=flow,
      shape_to_prepend=shape_to_prepend,
      source=source)
  flow_out = _stack(flow_out, pfor_input.pfor.loop_len_vector).t
  return [wrap(grad_handle, False), wrap(flow_out, True)]


# StackV2 conversion is tricky since we don't have arrays of StackV2. So similar
# to TensorArrays, we convert them by changing the dimension of the elements
# inside the stack.
#
# We consider two cases:
#
# 1. StackV2 is constructed and used entirely inside the pfor loop.
# We keep a single Stack and perform the push/pop operations of all the
# iterations in lock-step. We also assume that all the iterations perform these
# operations. In case of dynamic control flow, if only some of the iterations
# try to perform a push/pop, then the conversion may not work correctly and may
# cause undefined behavior.
# TODO(agarwal): test StackV2 with dynamic control flow.
#
# 2. StackV2 is constructed outside the pfor loop.
# Performing stack push/pop in a parallel fashion is ill-defined. However given
# that reading stacks created externally is a common operation when computing
# jacobians, we provide some special semantics here as follows.
#  - disallow push operations to the stack
#  - pop operations are performed in lock step by all iterations, similar to the
#  case when the stack is created inside. A single value is popped during the
#  lock-step operation and broadcast to all the iterations. Values in the stack
#  are assumed to be loop-invariant.
#
# Some other implementation details:
# We use an ugly logic to find whether values in Stack data structure are
# loop invariant or not. When converting push/pop operations, we keep track of
# whether the last conversion used a stacked value or not (see _stack_cache
# below). As a result if an unstacked value is written first, subsequent stacked
# writes are disallowed when they could have been allowed in theory.

# Map from cache key based on StackV2 handle to a bool indicating whether values
# are stacked or not.
# TODO(agarwal): move _stack_cache inside pfor?
_stack_cache = {}


def _stack_cache_key(pfor_input):
  """Create cache key corresponding to a stack handle."""
  op_type = pfor_input.op_type
  assert op_type in ["StackPushV2", "StackPopV2"], op_type
  orig_handle = pfor_input.op.inputs[0]
  while orig_handle.op.type in ["Identity", "Enter"]:
    orig_handle = orig_handle.op.inputs[0]
  assert orig_handle.op.type == "StackV2", orig_handle.op
  return ops.get_default_graph(), pfor_input.pfor, orig_handle


def _stack_handle_inside_pfor(handle, pfor_input):
  while handle.op.type in ["Identity", "Enter"]:
    handle = handle.op.inputs[0]
  assert handle.op.type == "StackV2", (
      "Unable to find StackV2 op. Got %s" % handle.op)
  return pfor_input.pfor.op_is_inside_loop(handle.op)


@RegisterPFor("StackPushV2")
def _convert_stack_push_v2(pfor_input):
  handle = pfor_input.unstacked_input(0)
  elem, elem_stacked, _ = pfor_input.input(1)
  swap_memory = pfor_input.get_attr("swap_memory")

  if not _stack_handle_inside_pfor(pfor_input.op.inputs[0], pfor_input):
    raise ValueError("StackPushV2 not allowed on stacks created outside pfor")
  stack_cache_key = _stack_cache_key(pfor_input)
  stacked = _stack_cache.get(stack_cache_key, None)
  if stacked is None:
    stacked = elem_stacked
    _stack_cache[stack_cache_key] = stacked
  else:
    # If we previously made it unstacked then we can't revert to being stacked.
    if not stacked and elem_stacked:
      raise ValueError(
          "It looks like the stack was previously determined to be loop"
          " invariant, but we are now trying to push a loop dependent value"
          " to it. This is currently unsupported.")
    if stacked and not elem_stacked:
      elem = _stack(elem, pfor_input.pfor.loop_len_vector).t
  out = data_flow_ops.stack_push_v2(handle, elem, swap_memory=swap_memory)
  return wrap(out, stacked)


# Note that inputs to this convertor will be unstacked. However it should get
# called since it is a stateful op.
@RegisterPFor("StackPopV2")
def _convert_stack_pop_v2(pfor_input):
  handle = pfor_input.unstacked_input(0)
  stack_cache_key = _stack_cache_key(pfor_input)
  stacked = _stack_cache.get(stack_cache_key, None)
  # If a StackPushV2 has not been converted yet, we default to unstacked since
  # the push could be outside of pfor, or the covertor may not be called if the
  # inputs are unconverted.
  if stacked is None:
    stacked = False
    _stack_cache[stack_cache_key] = False
  elem_type = pfor_input.get_attr("elem_type")
  out = data_flow_ops.stack_pop_v2(handle, elem_type)
  return wrap(out, stacked)


# parsing_ops


@RegisterPFor("DecodeCSV")
def _convert_decode_csv(pfor_input):
  lines = pfor_input.stacked_input(0)
  record_defaults = [
      pfor_input.unstacked_input(i) for i in range(1, pfor_input.num_inputs)
  ]
  field_delim = pfor_input.get_attr("field_delim")
  use_quote_delim = pfor_input.get_attr("use_quote_delim")
  select_cols = pfor_input.get_attr("select_cols")
  if not select_cols:
    select_cols = None
  return [
      wrap(t, True) for t in parsing_ops.decode_csv(
          lines,
          record_defaults,
          field_delim=field_delim,
          use_quote_delim=use_quote_delim,
          select_cols=select_cols)
  ]


@RegisterPFor("ParseSingleExample")
def _convert_parse_single_example(pfor_input):
  serialized = pfor_input.stacked_input(0)
  dense_defaults = [
      pfor_input.unstacked_input(i) for i in range(1, pfor_input.num_inputs)
  ]
  sparse_keys = pfor_input.get_attr("sparse_keys")
  dense_keys = pfor_input.get_attr("dense_keys")
  sparse_types = pfor_input.get_attr("sparse_types")
  dense_shapes = pfor_input.get_attr("dense_shapes")
  output = gen_parsing_ops.parse_example(
      serialized=serialized,
      names=[],
      dense_defaults=dense_defaults,
      sparse_keys=sparse_keys,
      dense_keys=dense_keys,
      sparse_types=sparse_types,
      dense_shapes=dense_shapes)
  return [wrap(t, True, True) for t in nest.flatten(output)]
