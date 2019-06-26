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

"""## Functions for working with arbitrarily nested sequences of elements.

This module can perform operations on nested structures. A nested structure is a
Python sequence, tuple (including `namedtuple`), or dict that can contain
further sequences, tuples, and dicts.

attr.s decorated classes (http://www.attrs.org) are also supported, in the
same way as `namedtuple`.

The utilities here assume (and do not check) that the nested structures form a
'tree', i.e., no references in the structure of the input of these functions
should be recursive.

Example structures: `((3, 4), 5, (6, 7, (9, 10), 8))`, `(np.array(0),
  (np.array([3, 4]), tf.constant([3, 4])))`
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections as _collections

import six as _six

from tensorflow.python import pywrap_tensorflow as _pywrap_tensorflow


def _get_attrs_values(obj):
  """Returns the list of values from an attrs instance."""
  attrs = getattr(obj.__class__, "__attrs_attrs__")
  return [getattr(obj, a.name) for a in attrs]


def _sorted(dict_):
  """Returns a sorted list of the dict keys, with error if keys not sortable."""
  try:
    return sorted(_six.iterkeys(dict_))
  except TypeError:
    raise TypeError("nest only supports dicts with sortable keys.")


def _is_namedtuple(instance, strict=False):
  """Returns True iff `instance` is a `namedtuple`.

  Args:
    instance: An instance of a Python object.
    strict: If True, `instance` is considered to be a `namedtuple` only if
        it is a "plain" namedtuple. For instance, a class inheriting
        from a `namedtuple` will be considered to be a `namedtuple`
        iff `strict=False`.

  Returns:
    True if `instance` is a `namedtuple`.
  """
  return _pywrap_tensorflow.IsNamedtuple(instance, strict)


# See the swig file (util.i) for documentation.
_is_mapping = _pywrap_tensorflow.IsMapping
_is_attrs = _pywrap_tensorflow.IsAttrs


def _sequence_like(instance, args):
  """Converts the sequence `args` to the same type as `instance`.

  Args:
    instance: an instance of `tuple`, `list`, `namedtuple`, `dict`, or
        `collections.OrderedDict`.
    args: elements to be converted to the `instance` type.

  Returns:
    `args` with the type of `instance`.
  """
  if _is_mapping(instance):
    # Pack dictionaries in a deterministic order by sorting the keys.
    # Notice this means that we ignore the original order of `OrderedDict`
    # instances. This is intentional, to avoid potential bugs caused by mixing
    # ordered and plain dicts (e.g., flattening a dict but using a
    # corresponding `OrderedDict` to pack it back).
    result = dict(zip(_sorted(instance), args))
    return type(instance)((key, result[key]) for key in _six.iterkeys(instance))
  elif _is_namedtuple(instance) or _is_attrs(instance):
    return type(instance)(*args)
  else:
    # Not a namedtuple
    return type(instance)(args)


def _yield_value(iterable):
  """Yields the next value from the given iterable."""
  if _is_mapping(iterable):
    # Iterate through dictionaries in a deterministic order by sorting the
    # keys. Notice this means that we ignore the original order of `OrderedDict`
    # instances. This is intentional, to avoid potential bugs caused by mixing
    # ordered and plain dicts (e.g., flattening a dict but using a
    # corresponding `OrderedDict` to pack it back).
    for key in _sorted(iterable):
      yield iterable[key]
  elif _is_attrs(iterable):
    for value in _get_attrs_values(iterable):
      yield value
  else:
    for value in iterable:
      yield value


# See the swig file (util.i) for documentation.
is_sequence = _pywrap_tensorflow.IsSequence


# See the swig file (util.i) for documentation.
flatten = _pywrap_tensorflow.Flatten


# See the swig file (util.i) for documentation.
_same_namedtuples = _pywrap_tensorflow.SameNamedtuples


class _DotString(object):

  def __str__(self):
    return "."

  def __repr__(self):
    return "."


_DOT = _DotString()


def assert_same_structure(nest1, nest2, check_types=True):
  """Asserts that two structures are nested in the same way.

  Note that namedtuples with identical name and fields are always considered
  to have the same shallow structure (even with `check_types=True`).
  For intance, this code will print `True`:

  ```python
  def nt(a, b):
    return collections.namedtuple('foo', 'a b')(a, b)
  print(assert_same_structure(nt(0, 1), nt(2, 3)))
  ```

  Args:
    nest1: an arbitrarily nested structure.
    nest2: an arbitrarily nested structure.
    check_types: if `True` (default) types of sequences are checked as well,
        including the keys of dictionaries. If set to `False`, for example a
        list and a tuple of objects will look the same if they have the same
        size. Note that namedtuples with identical name and fields are always
        considered to have the same shallow structure. Two types will also be
        considered the same if they are both list subtypes (which allows "list"
        and "_ListWrapper" from checkpointable dependency tracking to compare
        equal).

  Raises:
    ValueError: If the two structures do not have the same number of elements or
      if the two structures are not nested in the same way.
    TypeError: If the two structures differ in the type of sequence in any of
      their substructures. Only possible if `check_types` is `True`.
  """
  try:
    _pywrap_tensorflow.AssertSameStructure(nest1, nest2, check_types)
  except (ValueError, TypeError) as e:
    str1 = str(map_structure(lambda _: _DOT, nest1))
    str2 = str(map_structure(lambda _: _DOT, nest2))
    raise type(e)("%s\n"
                  "Entire first structure:\n%s\n"
                  "Entire second structure:\n%s"
                  % (str(e), str1, str2))


def flatten_dict_items(dictionary):
  """Returns a dictionary with flattened keys and values.

  This function flattens the keys and values of a dictionary, which can be
  arbitrarily nested structures, and returns the flattened version of such
  structures:

  ```python
  example_dictionary = {(4, 5, (6, 8)): ("a", "b", ("c", "d"))}
  result = {4: "a", 5: "b", 6: "c", 8: "d"}
  flatten_dict_items(example_dictionary) == result
  ```

  The input dictionary must satisfy two properties:

  1. Its keys and values should have the same exact nested structure.
  2. The set of all flattened keys of the dictionary must not contain repeated
     keys.

  Args:
    dictionary: the dictionary to zip

  Returns:
    The zipped dictionary.

  Raises:
    TypeError: If the input is not a dictionary.
    ValueError: If any key and value have not the same structure, or if keys are
      not unique.
  """
  if not isinstance(dictionary, (dict, _collections.Mapping)):
    raise TypeError("input must be a dictionary")
  flat_dictionary = {}
  for i, v in _six.iteritems(dictionary):
    if not is_sequence(i):
      if i in flat_dictionary:
        raise ValueError(
            "Could not flatten dictionary: key %s is not unique." % i)
      flat_dictionary[i] = v
    else:
      flat_i = flatten(i)
      flat_v = flatten(v)
      if len(flat_i) != len(flat_v):
        raise ValueError(
            "Could not flatten dictionary. Key had %d elements, but value had "
            "%d elements. Key: %s, value: %s."
            % (len(flat_i), len(flat_v), flat_i, flat_v))
      for new_i, new_v in zip(flat_i, flat_v):
        if new_i in flat_dictionary:
          raise ValueError(
              "Could not flatten dictionary: key %s is not unique."
              % (new_i))
        flat_dictionary[new_i] = new_v
  return flat_dictionary


def _packed_nest_with_indices(structure, flat, index):
  """Helper function for pack_sequence_as.

  Args:
    structure: Substructure (list / tuple / dict) to mimic.
    flat: Flattened values to output substructure for.
    index: Index at which to start reading from flat.

  Returns:
    The tuple (new_index, child), where:
      * new_index - the updated index into `flat` having processed `structure`.
      * packed - the subset of `flat` corresponding to `structure`,
                 having started at `index`, and packed into the same nested
                 format.

  Raises:
    ValueError: if `structure` contains more elements than `flat`
      (assuming indexing starts from `index`).
  """
  packed = []
  for s in _yield_value(structure):
    if is_sequence(s):
      new_index, child = _packed_nest_with_indices(s, flat, index)
      packed.append(_sequence_like(s, child))
      index = new_index
    else:
      packed.append(flat[index])
      index += 1
  return index, packed


def pack_sequence_as(structure, flat_sequence):
  """Returns a given flattened sequence packed into a given structure.

  If `structure` is a scalar, `flat_sequence` must be a single-element list;
  in this case the return value is `flat_sequence[0]`.

  If `structure` is or contains a dict instance, the keys will be sorted to
  pack the flat sequence in deterministic order. This is true also for
  `OrderedDict` instances: their sequence order is ignored, the sorting order of
  keys is used instead. The same convention is followed in `flatten`.
  This correctly repacks dicts and `OrderedDict`s after they have been
  flattened, and also allows flattening an `OrderedDict` and then repacking it
  back using a corresponding plain dict, or vice-versa.
  Dictionaries with non-sortable keys cannot be flattened.

  Args:
    structure: Nested structure, whose structure is given by nested lists,
        tuples, and dicts. Note: numpy arrays and strings are considered
        scalars.
    flat_sequence: flat sequence to pack.

  Returns:
    packed: `flat_sequence` converted to have the same recursive structure as
      `structure`.

  Raises:
    ValueError: If `flat_sequence` and `structure` have different
      element counts.
    TypeError: `structure` is or contains a dict with non-sortable keys.
  """
  if not is_sequence(flat_sequence):
    raise TypeError("flat_sequence must be a sequence")

  if not is_sequence(structure):
    if len(flat_sequence) != 1:
      raise ValueError("Structure is a scalar but len(flat_sequence) == %d > 1"
                       % len(flat_sequence))
    return flat_sequence[0]

  try:
    final_index, packed = _packed_nest_with_indices(structure, flat_sequence, 0)
    if final_index < len(flat_sequence):
      raise IndexError
  except IndexError:
    flat_structure = flatten(structure)
    if len(flat_structure) != len(flat_sequence):
      raise ValueError(
          "Could not pack sequence. Structure had %d elements, but "
          "flat_sequence had %d elements.  Structure: %s, flat_sequence: %s." %
          (len(flat_structure), len(flat_sequence), structure, flat_sequence))
  return _sequence_like(structure, packed)


def map_structure(func, *structure, **check_types_dict):
  """Applies `func` to each entry in `structure` and returns a new structure.

  Applies `func(x[0], x[1], ...)` where x[i] is an entry in
  `structure[i]`.  All structures in `structure` must have the same arity,
  and the return value will contain the results in the same structure.

  Args:
    func: A callable that accepts as many arguments as there are structures.
    *structure: scalar, or tuple or list of constructed scalars and/or other
      tuples/lists, or scalars.  Note: numpy arrays are considered as scalars.
    **check_types_dict: only valid keyword argument is `check_types`. If set to
      `True` (default) the types of iterables within the structures have to be
      same (e.g. `map_structure(func, [1], (1,))` raises a `TypeError`
      exception). To allow this set this argument to `False`.
      Note that namedtuples with identical name and fields are always
      considered to have the same shallow structure.

  Returns:
    A new structure with the same arity as `structure`, whose values correspond
    to `func(x[0], x[1], ...)` where `x[i]` is a value in the corresponding
    location in `structure[i]`. If there are different sequence types and
    `check_types` is `False` the sequence types of the first structure will be
    used.

  Raises:
    TypeError: If `func` is not callable or if the structures do not match
      each other by depth tree.
    ValueError: If no structure is provided or if the structures do not match
      each other by type.
    ValueError: If wrong keyword arguments are provided.
  """
  if not callable(func):
    raise TypeError("func must be callable, got: %s" % func)

  if not structure:
    raise ValueError("Must provide at least one structure")

  if check_types_dict:
    if "check_types" not in check_types_dict or len(check_types_dict) > 1:
      raise ValueError("Only valid keyword argument is check_types")
    check_types = check_types_dict["check_types"]
  else:
    check_types = True

  for other in structure[1:]:
    assert_same_structure(structure[0], other, check_types=check_types)

  flat_structure = [flatten(s) for s in structure]
  entries = zip(*flat_structure)

  return pack_sequence_as(
      structure[0], [func(*x) for x in entries])


def map_structure_with_paths(func, *structure, **kwargs):
  """Applies `func` to each entry in `structure` and returns a new structure.

  Applies `func(path, x[0], x[1], ..., **kwargs)` where x[i] is an entry in
  `structure[i]` and `path` is the common path to x[i] in the structures.  All
  structures in `structure` must have the same arity, and the return value will
  contain the results in the same structure. Special kwarg `check_types`
  determines whether the types of iterables within the structure must be the
  same-- see **kwargs definition below.

  Args:
    func: A callable with the signature func(path, *values, **kwargs) that is
      evaluated on the leaves of the structure.
    *structure: A variable number of compatible structures to process.
    **kwargs: Optional kwargs to be passed through to func. Special kwarg
      `check_types` is not passed to func, but instead determines whether the
      types of iterables within the structures have to be same (e.g.,
      `map_structure(func, [1], (1,))` raises a `TypeError` exception). By
      default, the types must match. To allow iteration over structures of
      different types (but common arity), set this kwarg to `False`.

  Returns:
    A structure of the same form as the input structures whose leaves are the
    result of evaluating func on corresponding leaves of the input structures.

  Raises:
    TypeError: If `func` is not callable or if the structures do not match
      each other by depth tree.
    TypeError: If `check_types` is not `False` and the two structures differ in
      the type of sequence in any of their substructures.
    ValueError: If no structures are provided.
  """
  if not callable(func):
    raise TypeError("func must be callable, got: %s" % func)
  if not structure:
    raise ValueError("Must provide at least one structure")

  check_types = kwargs.pop("check_types", True)
  for other in structure[1:]:
    assert_same_structure(structure[0], other, check_types=check_types)

  # First set paths_and_values to:
  # [[(p11, v11), ... (p1n, v1n)], ... [(pm1, vm1), ... (pmn, vmn)]]
  paths_and_values = [flatten_with_joined_string_paths(s) for s in structure]

  # Now zip(*paths_and_values) would be:
  # [((p11, v11), ... (pm1, vm1)), ... ((p1n, v1n), ... (pmn, vmn))]
  # so grouped_by_path is set to:
  # [[(p11, ... pm1), (v11, ... vm1)], ... [(p1n, ... pmn), (v1n, ... vmn)]]
  # Note that p1i, ... pmi must all be equal since the structures are the same.
  grouped_by_path = [zip(*p_v) for p_v in zip(*paths_and_values)]

  return pack_sequence_as(structure[0], [
      func(paths[0], *values, **kwargs) for paths, values in grouped_by_path])


def _yield_flat_up_to(shallow_tree, input_tree):
  """Yields elements `input_tree` partially flattened up to `shallow_tree`."""
  if is_sequence(shallow_tree):
    for shallow_branch, input_branch in zip(_yield_value(shallow_tree),
                                            _yield_value(input_tree)):
      for input_leaf in _yield_flat_up_to(shallow_branch, input_branch):
        yield input_leaf
  else:
    yield input_tree


def assert_shallow_structure(shallow_tree, input_tree, check_types=True):
  """Asserts that `shallow_tree` is a shallow structure of `input_tree`.

  That is, this function tests if the `input_tree` structure can be created from
  the `shallow_tree` structure by replacing its leaf nodes with deeper
  tree structures.

  Examples:

  The following code will raise an exception:
  ```python
    shallow_tree = ["a", "b"]
    input_tree = ["c", ["d", "e"], "f"]
    assert_shallow_structure(shallow_tree, input_tree)
  ```

  The following code will not raise an exception:
  ```python
    shallow_tree = ["a", "b"]
    input_tree = ["c", ["d", "e"]]
    assert_shallow_structure(shallow_tree, input_tree)
  ```

  Args:
    shallow_tree: an arbitrarily nested structure.
    input_tree: an arbitrarily nested structure.
    check_types: if `True` (default) the sequence types of `shallow_tree` and
      `input_tree` have to be the same. Note that even with check_types==True,
      this function will consider two different namedtuple classes with the same
      name and _fields attribute to be the same class.

  Raises:
    TypeError: If `shallow_tree` is a sequence but `input_tree` is not.
    TypeError: If the sequence types of `shallow_tree` are different from
      `input_tree`. Only raised if `check_types` is `True`.
    ValueError: If the sequence lengths of `shallow_tree` are different from
      `input_tree`.
  """
  if is_sequence(shallow_tree):
    if not is_sequence(input_tree):
      raise TypeError(
          "If shallow structure is a sequence, input must also be a sequence. "
          "Input has type: %s." % type(input_tree))

    if check_types and not isinstance(input_tree, type(shallow_tree)):
      # Duck-typing means that nest should be fine with two different
      # namedtuples with identical name and fields.
      shallow_is_namedtuple = _is_namedtuple(shallow_tree, False)
      input_is_namedtuple = _is_namedtuple(input_tree, False)
      if shallow_is_namedtuple and input_is_namedtuple:
        if not _same_namedtuples(shallow_tree, input_tree):
          raise TypeError(
              "The two namedtuples don't have the same sequence type. Input "
              "structure has type %s, while shallow structure has type %s."
              % (type(input_tree), type(shallow_tree)))
      elif not (isinstance(shallow_tree, _collections.Mapping)
                and isinstance(input_tree, _collections.Mapping)):
        raise TypeError(
            "The two structures don't have the same sequence type. Input "
            "structure has type %s, while shallow structure has type %s."
            % (type(input_tree), type(shallow_tree)))

    if len(input_tree) != len(shallow_tree):
      raise ValueError(
          "The two structures don't have the same sequence length. Input "
          "structure has length %s, while shallow structure has length %s."
          % (len(input_tree), len(shallow_tree)))

    if check_types and isinstance(shallow_tree, (dict, _collections.Mapping)):
      if set(input_tree) != set(shallow_tree):
        raise ValueError(
            "The two structures don't have the same keys. Input "
            "structure has keys %s, while shallow structure has keys %s." %
            (list(_six.iterkeys(input_tree)),
             list(_six.iterkeys(shallow_tree))))

      input_tree = list(sorted(_six.iteritems(input_tree)))
      shallow_tree = list(sorted(_six.iteritems(shallow_tree)))

    for shallow_branch, input_branch in zip(shallow_tree, input_tree):
      assert_shallow_structure(shallow_branch, input_branch,
                               check_types=check_types)


def flatten_up_to(shallow_tree, input_tree):
  """Flattens `input_tree` up to `shallow_tree`.

  Any further depth in structure in `input_tree` is retained as elements in the
  partially flatten output.

  If `shallow_tree` and `input_tree` are not sequences, this returns a
  single-element list: `[input_tree]`.

  Use Case:

  Sometimes we may wish to partially flatten a nested sequence, retaining some
  of the nested structure. We achieve this by specifying a shallow structure,
  `shallow_tree`, we wish to flatten up to.

  The input, `input_tree`, can be thought of as having the same structure as
  `shallow_tree`, but with leaf nodes that are themselves tree structures.

  Examples:

  ```python
  input_tree = [[[2, 2], [3, 3]], [[4, 9], [5, 5]]]
  shallow_tree = [[True, True], [False, True]]

  flattened_input_tree = flatten_up_to(shallow_tree, input_tree)
  flattened_shallow_tree = flatten_up_to(shallow_tree, shallow_tree)

  # Output is:
  # [[2, 2], [3, 3], [4, 9], [5, 5]]
  # [True, True, False, True]
  ```

  ```python
  input_tree = [[('a', 1), [('b', 2), [('c', 3), [('d', 4)]]]]]
  shallow_tree = [['level_1', ['level_2', ['level_3', ['level_4']]]]]

  input_tree_flattened_as_shallow_tree = flatten_up_to(shallow_tree, input_tree)
  input_tree_flattened = flatten(input_tree)

  # Output is:
  # [('a', 1), ('b', 2), ('c', 3), ('d', 4)]
  # ['a', 1, 'b', 2, 'c', 3, 'd', 4]
  ```

  Non-Sequence Edge Cases:

  ```python
  flatten_up_to(0, 0)  # Output: [0]
  flatten_up_to(0, [0, 1, 2])  # Output: [[0, 1, 2]]
  flatten_up_to([0, 1, 2], 0)  # Output: TypeError
  flatten_up_to([0, 1, 2], [0, 1, 2])  # Output: [0, 1, 2]
  ```

  Args:
    shallow_tree: a possibly pruned structure of input_tree.
    input_tree: an arbitrarily nested structure or a scalar object.
      Note, numpy arrays are considered scalars.

  Returns:
    A Python list, the partially flattened version of `input_tree` according to
    the structure of `shallow_tree`.

  Raises:
    TypeError: If `shallow_tree` is a sequence but `input_tree` is not.
    TypeError: If the sequence types of `shallow_tree` are different from
      `input_tree`.
    ValueError: If the sequence lengths of `shallow_tree` are different from
      `input_tree`.
  """
  assert_shallow_structure(shallow_tree, input_tree)
  return list(_yield_flat_up_to(shallow_tree, input_tree))


def map_structure_up_to(shallow_tree, func, *inputs):
  """Applies a function or op to a number of partially flattened inputs.

  The `inputs` are flattened up to `shallow_tree` before being mapped.

  Use Case:

  Sometimes we wish to apply a function to a partially flattened
  sequence (for example when the function itself takes sequence inputs). We
  achieve this by specifying a shallow structure, `shallow_tree` we wish to
  flatten up to.

  The `inputs`, can be thought of as having the same structure as
  `shallow_tree`, but with leaf nodes that are themselves tree structures.

  This function therefore will return something with the same base structure as
  `shallow_tree`.

  Examples:

  ```python
  ab_tuple = collections.namedtuple("ab_tuple", "a, b")
  op_tuple = collections.namedtuple("op_tuple", "add, mul")
  inp_val = ab_tuple(a=2, b=3)
  inp_ops = ab_tuple(a=op_tuple(add=1, mul=2), b=op_tuple(add=2, mul=3))
  out = map_structure_up_to(inp_val, lambda val, ops: (val + ops.add) * ops.mul,
                            inp_val, inp_ops)

  # Output is: ab_tuple(a=6, b=15)
  ```

  ```python
  data_list = [[2, 4, 6, 8], [[1, 3, 5, 7, 9], [3, 5, 7]]]
  name_list = ['evens', ['odds', 'primes']]
  out = map_structure_up_to(
      name_list,
      lambda name, sec: "first_{}_{}".format(len(sec), name),
      name_list, data_list)

  # Output is: ['first_4_evens', ['first_5_odds', 'first_3_primes']]
  ```

  Args:
    shallow_tree: a shallow tree, common to all the inputs.
    func: callable which will be applied to each input individually.
    *inputs: arbitrarily nested combination of objects that are compatible with
        shallow_tree. The function `func` is applied to corresponding
        partially flattened elements of each input, so the function must support
        arity of `len(inputs)`.

  Raises:
    TypeError: If `shallow_tree` is a sequence but `input_tree` is not.
    TypeError: If the sequence types of `shallow_tree` are different from
      `input_tree`.
    ValueError: If the sequence lengths of `shallow_tree` are different from
      `input_tree`.

  Returns:
    result of repeatedly applying `func`, with same structure as
    `shallow_tree`.
  """
  if not inputs:
    raise ValueError("Cannot map over no sequences")
  for input_tree in inputs:
    assert_shallow_structure(shallow_tree, input_tree)

  # Flatten each input separately, apply the function to corresponding elements,
  # then repack based on the structure of the first input.
  all_flattened_up_to = [flatten_up_to(shallow_tree, input_tree)
                         for input_tree in inputs]
  results = [func(*tensors) for tensors in zip(*all_flattened_up_to)]
  return pack_sequence_as(structure=shallow_tree, flat_sequence=results)


def get_traverse_shallow_structure(traverse_fn, structure):
  """Generates a shallow structure from a `traverse_fn` and `structure`.

  `traverse_fn` must accept any possible subtree of `structure` and return
  a depth=1 structure containing `True` or `False` values, describing which
  of the top-level subtrees may be traversed.  It may also
  return scalar `True` or `False` "traversal is OK / not OK for all subtrees."

  Examples are available in the unit tests (nest_test.py).

  Args:
    traverse_fn: Function taking a substructure and returning either a scalar
      `bool` (whether to traverse that substructure or not) or a depth=1
      shallow structure of the same type, describing which parts of the
      substructure to traverse.
    structure: The structure to traverse.

  Returns:
    A shallow structure containing python bools, which can be passed to
    `map_structure_up_to` and `flatten_up_to`.

  Raises:
    TypeError: if `traverse_fn` returns a sequence for a non-sequence input,
      or a structure with depth higher than 1 for a sequence input,
      or if any leaf values in the returned structure or scalar are not type
      `bool`.
  """
  to_traverse = traverse_fn(structure)
  if not is_sequence(structure):
    if not isinstance(to_traverse, bool):
      raise TypeError("traverse_fn returned structure: %s for non-structure: %s"
                      % (to_traverse, structure))
    return to_traverse
  level_traverse = []
  if isinstance(to_traverse, bool):
    if not to_traverse:
      # Do not traverse this substructure at all.  Exit early.
      return False
    else:
      # Traverse the entire substructure.
      for branch in _yield_value(structure):
        level_traverse.append(
            get_traverse_shallow_structure(traverse_fn, branch))
  elif not is_sequence(to_traverse):
    raise TypeError("traverse_fn returned a non-bool scalar: %s for input: %s"
                    % (to_traverse, structure))
  else:
    # Traverse some subset of this substructure.
    assert_shallow_structure(to_traverse, structure)
    for t, branch in zip(_yield_value(to_traverse), _yield_value(structure)):
      if not isinstance(t, bool):
        raise TypeError(
            "traverse_fn didn't return a depth=1 structure of bools.  saw: %s "
            " for structure: %s" % (to_traverse, structure))
      if t:
        level_traverse.append(
            get_traverse_shallow_structure(traverse_fn, branch))
      else:
        level_traverse.append(False)
  return _sequence_like(structure, level_traverse)


def yield_flat_paths(nest):
  """Yields paths for some nested structure.

  Paths are lists of objects which can be str-converted, which may include
  integers or other types which are used as indices in a dict.

  The flat list will be in the corresponding order as if you called
  `snt.nest.flatten` on the structure. This is handy for naming Tensors such
  the TF scope structure matches the tuple structure.

  E.g. if we have a tuple `value = Foo(a=3, b=Bar(c=23, d=42))`

  ```shell
  >>> nest.flatten(value)
  [3, 23, 42]
  >>> list(nest.yield_flat_paths(value))
  [('a',), ('b', 'c'), ('b', 'd')]
  ```

  ```shell
  >>> list(nest.yield_flat_paths({'a': [3]}))
  [('a', 0)]
  >>> list(nest.yield_flat_paths({'a': 3}))
  [('a',)]
  ```

  Args:
    nest: the value to produce a flattened paths list for.

  Yields:
    Tuples containing index or key values which form the path to a specific
      leaf value in the nested structure.
  """

  # The _maybe_add_final_path_element function is used below in order to avoid
  # adding trailing slashes when the sub-element recursed into is a leaf.
  if isinstance(nest, (dict, _collections.Mapping)):
    for key in _sorted(nest):
      value = nest[key]
      for sub_path in yield_flat_paths(value):
        yield (key,) + sub_path
  elif _is_namedtuple(nest):
    for key in nest._fields:
      value = getattr(nest, key)
      for sub_path in yield_flat_paths(value):
        yield (key,) + sub_path
  elif isinstance(nest, _six.string_types):
    yield ()
  elif isinstance(nest, _collections.Sequence):
    for idx, value in enumerate(nest):
      for sub_path in yield_flat_paths(value):
        yield (idx,) + sub_path
  else:
    yield ()


def flatten_with_joined_string_paths(structure, separator="/"):
  """Returns a list of (string path, data element) tuples.

  The order of tuples produced matches that of `nest.flatten`. This allows you
  to flatten a nested structure while keeping information about where in the
  structure each data element was located. See `nest.yield_flat_paths`
  for more information.

  Args:
    structure: the nested structure to flatten.
    separator: string to separate levels of hierarchy in the results, defaults
      to '/'.

  Returns:
    A list of (string, data element) tuples.
  """
  flat_paths = yield_flat_paths(structure)
  def stringify_and_join(path_elements):
    return separator.join(str(path_element) for path_element in path_elements)
  flat_string_paths = [stringify_and_join(path) for path in flat_paths]
  return list(zip(flat_string_paths, flatten(structure)))


_pywrap_tensorflow.RegisterType("Mapping", _collections.Mapping)
_pywrap_tensorflow.RegisterType("Sequence", _collections.Sequence)
