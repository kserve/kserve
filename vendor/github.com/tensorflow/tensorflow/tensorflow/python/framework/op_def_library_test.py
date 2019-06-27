# Copyright 2015 The TensorFlow Authors. All Rights Reserved.
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

"""Tests for tensorflow.python.ops.op_def_library."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from google.protobuf import text_format

from tensorflow.core.framework import op_def_pb2
from tensorflow.core.framework import tensor_shape_pb2
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import test_ops
from tensorflow.python.framework import test_util
from tensorflow.python.platform import googletest


def _unknown_shape(op):
  """Shape function for use with ops whose output shapes are unknown."""
  return [tensor_shape.unknown_shape() for _ in op.outputs]


class OpDefLibraryTest(test_util.TensorFlowTestCase):

  def setUp(self):
    self._lib = test_ops._op_def_lib

  def _add_op(self, ascii):  # pylint: disable=redefined-builtin
    op_def = op_def_pb2.OpDef()
    text_format.Merge(ascii, op_def)
    self._lib.add_op(op_def)

  def Tensor(self, t, name="in"):
    return self._lib.apply_op("OutT", T=t, name=name)

  def testNoRegisteredOpFails(self):
    with self.assertRaises(RuntimeError) as cm:
      self._lib.apply_op("unknown")
    self.assertEqual(str(cm.exception), "Unrecognized Op name unknown")

  def testAddOpValidation(self):
    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'MissingTypeAttr' "
                   "input_arg { name: 'a' type_attr: 'T' } ")
    self.assertEqual(str(cm.exception),
                     "Inconsistent OpDef for 'MissingTypeAttr', "
                     "missing attr 'T'")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'BadTypeAttr' "
                   "output_arg { name: 'a' type_attr: 'T' } "
                   "attr { name: 'T' type: 'int' }")
    self.assertEqual(
        str(cm.exception),
        "Attr 'T' of 'BadTypeAttr' used as a type_attr but has type int")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'MissingNumberAttr' "
                   "input_arg { name: 'a' type: DT_INT32 number_attr: 'N' } ")
    self.assertEqual(str(cm.exception),
                     "Inconsistent OpDef for 'MissingNumberAttr', "
                     "missing attr 'N'")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'BadNumberAttr' "
                   "output_arg { name: 'a' type: DT_INT32 number_attr: 'N' } "
                   "attr { name: 'N' type: 'type' }")
    self.assertEqual(
        str(cm.exception),
        "Attr 'N' of 'BadNumberAttr' used as a number_attr but has type type")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'TwoTypesA' "
                   "input_arg { name: 'a' type: DT_INT32 type_attr: 'T' } "
                   "attr { name: 'T' type: 'type' }")
    self.assertEqual(str(cm.exception),
                     "Arg 'a' of 'TwoTypesA' must have one type field not 2")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'TwoTypesB' "
                   "input_arg { name: 'a' type: DT_INT32 type_list_attr: 'T' } "
                   "attr { name: 'T' type: 'list(type)' }")
    self.assertEqual(str(cm.exception),
                     "Arg 'a' of 'TwoTypesB' must have one type field not 2")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'ThreeTypes' "
                   "input_arg { name: 'a' type: DT_INT32 type_attr: 'T' "
                   "type_list_attr: 'U' } "
                   "attr { name: 'T' type: 'type' } "
                   "attr { name: 'U' type: 'list(type)' }")
    self.assertEqual(str(cm.exception),
                     "Arg 'a' of 'ThreeTypes' must have one type field not 3")

    with self.assertRaises(TypeError) as cm:
      self._add_op("name: 'NoTypes' output_arg { name: 'a' } ")
    self.assertEqual(str(cm.exception),
                     "Arg 'a' of 'NoTypes' must have one type field not 0")

  def testSimple(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("Simple", a=3)
      self.assertEqual(dtypes.float32, out.dtype)
      self.assertProtoEquals("""
        name: 'Simple' op: 'Simple' input: 'Simple/a'
        """, out.op.node_def)

      out = self._lib.apply_op("Simple", a=4)
      self.assertProtoEquals("""
        name: 'Simple_1' op: 'Simple' input: 'Simple_1/a'
        """, out.op.node_def)

      out = self._lib.apply_op("Simple", a=5, name="named")
      self.assertProtoEquals("""
        name: 'named' op: 'Simple' input: 'named/a'
        """, out.op.node_def)

      out = self._lib.apply_op("Simple", a=[[1, 2, 3], [4, 5, 6]], name="two_d")
      self.assertProtoEquals("""
        name: 'two_d' op: 'Simple' input: 'two_d/a'
        """, out.op.node_def)

  def testSimpleFailures(self):
    with ops.Graph().as_default():
      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple", a="Bad string")
      self.assertEqual(str(cm.exception),
                       "Expected int32 passed to parameter 'a' of op 'Simple', "
                       "got 'Bad string' of type 'str' instead.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple", a=self.Tensor(dtypes.string))
      self.assertEqual(str(cm.exception),
                       "Input 'a' of 'Simple' Op has type string "
                       "that does not match expected type of int32.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple", a=6, extra="bogus")
      self.assertEqual(str(cm.exception),
                       "apply_op() got unexpected keyword arguments: extra")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple", a=6, extra1="bogus", extra2="also_bogus")
      self.assertEqual(str(cm.exception),
                       "apply_op() got unexpected keyword arguments: extra1, "
                       "extra2")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple")
      self.assertEqual(str(cm.exception), "No argument for input a")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple", wrong=7)
      self.assertEqual(str(cm.exception), "No argument for input a")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Simple", a={"label": 1})
      self.assertEqual(str(cm.exception),
                       "Expected int32 passed to parameter 'a' of op 'Simple', "
                       "got {'label': 1} of type 'dict' instead.")

  def testReservedInput(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("ReservedInput", input_=7, name="x")
      self.assertProtoEquals("""
        name: 'x' op: 'ReservedInput' input: 'x/input'
        """, op.node_def)

  def testPolymorphic(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("Polymorphic", a=7, name="p")
      self.assertEqual(dtypes.int32, out.dtype)
      self.assertProtoEquals("""
        name: 'p' op: 'Polymorphic' input: 'p/a'
        attr { key: 'T' value { type: DT_INT32 } }
        """, out.op.node_def)

      out = self._lib.apply_op("Polymorphic", a="s", name="q")
      self.assertEqual(dtypes.string, out.dtype)
      self.assertProtoEquals("""
        name: 'q' op: 'Polymorphic' input: 'q/a'
        attr { key: 'T' value { type: DT_STRING } }
        """, out.op.node_def)

      out = self._lib.apply_op("Polymorphic", a=["s", "t", "u"], name="r")
      self.assertEqual(dtypes.string, out.dtype)
      self.assertProtoEquals("""
        name: 'r' op: 'Polymorphic' input: 'r/a'
        attr { key: 'T' value { type: DT_STRING } }
        """, out.op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Polymorphic", a="s", T=dtypes.string)
      self.assertEqual(str(cm.exception),
                       "Should not specify value for inferred attr 'T'.")

  def testPolymorphicOut(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("PolymorphicOut", T=dtypes.int32, name="p")
      self.assertEqual(dtypes.int32, out.dtype)
      self.assertProtoEquals("""
        name: 'p' op: 'PolymorphicOut'
        attr { key: 'T' value { type: DT_INT32 } }
        """, out.op.node_def)

      out = self._lib.apply_op("PolymorphicOut", T=dtypes.bool, name="q")
      self.assertEqual(dtypes.bool, out.dtype)
      self.assertProtoEquals("""
        name: 'q' op: 'PolymorphicOut'
        attr { key: 'T' value { type: DT_BOOL } }
        """, out.op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("PolymorphicOut")
      self.assertEqual(str(cm.exception),
                       "No argument for attr T")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("PolymorphicOut", T=None)
      self.assertEqual(str(cm.exception),
                       "Expected DataType for argument 'T' not None.")

  def testPolymorphicDefaultOut(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("PolymorphicDefaultOut", T=None, name="p")
      self.assertEqual(dtypes.string, out.dtype)
      self.assertProtoEquals("""
        name: 'p' op: 'PolymorphicDefaultOut'
        attr { key: 'T' value { type: DT_STRING } }
        """, out.op.node_def)

      out = self._lib.apply_op("PolymorphicDefaultOut", T=dtypes.bool, name="q")
      self.assertEqual(dtypes.bool, out.dtype)
      self.assertProtoEquals("""
        name: 'q' op: 'PolymorphicDefaultOut'
        attr { key: 'T' value { type: DT_BOOL } }
        """, out.op.node_def)

  def testBinary(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("Binary", a=8, b=9, name="b")
      self.assertEqual(dtypes.int32, out.dtype)
      self.assertProtoEquals("""
        name: 'b' op: 'Binary' input: 'b/a' input: 'b/b'
        attr { key: 'T' value { type: DT_INT32 } }
        """, out.op.node_def)

      out = self._lib.apply_op("Binary", a="left", b="right", name="c")
      self.assertEqual(dtypes.string, out.dtype)
      self.assertProtoEquals("""
        name: 'c' op: 'Binary' input: 'c/a' input: 'c/b'
        attr { key: 'T' value { type: DT_STRING } }
        """, out.op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Binary", a="left", b=12)
      self.assertEqual(str(cm.exception),
                       "Expected string passed to parameter 'b' of op 'Binary',"
                       " got 12 of type 'int' instead.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Binary",
                           a=self.Tensor(dtypes.string),
                           b=self.Tensor(dtypes.int32))
      self.assertEqual(str(cm.exception),
                       "Input 'b' of 'Binary' Op has type int32 "
                       "that does not match type string of argument 'a'.")

  def testRestrict(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("Restrict", a="foo", name="g")
      self.assertEqual(dtypes.string, out.dtype)
      self.assertProtoEquals("""
        name: 'g' op: 'Restrict' input: 'g/a'
        attr { key: 'T' value { type: DT_STRING } }
        """, out.op.node_def)

      out = self._lib.apply_op("Restrict", a=True, name="h")
      self.assertEqual(dtypes.bool, out.dtype)
      self.assertProtoEquals("""
        name: 'h' op: 'Restrict' input: 'h/a'
        attr { key: 'T' value { type: DT_BOOL } }
        """, out.op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Restrict", a=17)
      self.assertEqual(str(cm.exception),
                       "Value passed to parameter 'a' has DataType int32 "
                       "not in list of allowed values: string, bool")

  def testTypeList(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("TypeList", a=["foo"], name="z")
      self.assertProtoEquals("""
        name: 'z' op: 'TypeList' input: 'z/a_0'
        attr { key: 'T' value { list { type: DT_STRING } } }
        """, op.node_def)

      op = self._lib.apply_op("TypeList", a=[True, 12], name="y")
      self.assertProtoEquals("""
        name: 'y' op: 'TypeList' input: 'y/a_0' input: 'y/a_1'
        attr { key: 'T' value { list { type: DT_BOOL type: DT_INT32 } } }
        """, op.node_def)

      op = self._lib.apply_op("TypeList", a=[], name="empty")
      self.assertProtoEquals("""
        name: 'empty' op: 'TypeList' attr { key: 'T' value { list { } } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("TypeList", a=17)
      self.assertStartsWith(str(cm.exception),
                            "Expected list for 'a' "
                            "argument to 'TypeList' Op, not ")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("TypeList", a=[self.Tensor(dtypes.int32), None])
      self.assertStartsWith(str(cm.exception),
                            "Tensors in list passed to 'a' of 'TypeList' Op "
                            "have types [int32, <NOT CONVERTIBLE TO TENSOR>]")

  def testTypeListTwice(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("TypeListTwice",
                              a=["foo", True],
                              b=["bar", False],
                              name="z")
      self.assertProtoEquals("""
        name: 'z' op: 'TypeListTwice'
        input: 'z/a_0' input: 'z/a_1' input: 'z/b_0' input: 'z/b_1'
        attr { key: 'T' value { list { type: DT_STRING type: DT_BOOL } } }
        """, op.node_def)

      op = self._lib.apply_op("TypeListTwice", a=[], b=[], name="empty")
      self.assertProtoEquals("""
        name: 'empty' op: 'TypeListTwice' attr { key: 'T' value { list { } } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("TypeListTwice", a=["foo", True], b=["bar", 6])
      self.assertEqual(str(cm.exception),
                       "Input 'b' of 'TypeListTwice' Op has type list of "
                       "string, int32 that does not match type list "
                       "string, bool of argument 'a'.")

  def testOutTypeList(self):
    with ops.Graph().as_default():
      out, = self._lib.apply_op("OutTypeList", T=[dtypes.float32], name="x")
      self.assertEqual(dtypes.float32, out.dtype)
      self.assertProtoEquals("""
        name: 'x' op: 'OutTypeList'
        attr { key: 'T' value { list { type: DT_FLOAT } } }
        """, out.op.node_def)

      out1, out2 = self._lib.apply_op("OutTypeList",
                                      T=[dtypes.int32, dtypes.bool],
                                      name="w")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.bool, out2.dtype)
      self.assertProtoEquals("""
        name: 'w' op: 'OutTypeList'
        attr { key: 'T' value { list { type: DT_INT32 type: DT_BOOL } } }
        """, out1.op.node_def)

      out = self._lib.apply_op("OutTypeList", T=[], name="empty")
      self.assertEqual([], out)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("OutTypeList", T=dtypes.int32)
      self.assertEqual(str(cm.exception), "Expected list for attr T")

  def testTypeListRestrict(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("TypeListRestrict", a=["foo", False], name="v")
      self.assertProtoEquals("""
        name: 'v' op: 'TypeListRestrict' input: 'v/a_0' input: 'v/a_1'
        attr { key: 'T' value { list { type: DT_STRING type: DT_BOOL } } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("TypeListRestrict", a=[True, 12])
      self.assertEqual(str(cm.exception),
                       "Value passed to parameter 'a' has DataType int32 "
                       "not in list of allowed values: string, bool")

  def testOutTypeListRestrict(self):
    with ops.Graph().as_default():
      out1, out2 = self._lib.apply_op("OutTypeListRestrict",
                                      t=[dtypes.bool, dtypes.string],
                                      name="u")
      self.assertEqual(dtypes.bool, out1.dtype)
      self.assertEqual(dtypes.string, out2.dtype)
      self.assertProtoEquals("""
        name: 'u' op: 'OutTypeListRestrict'
        attr { key: 't' value { list { type: DT_BOOL type: DT_STRING } } }
        """, out1.op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("OutTypeListRestrict",
                           t=[dtypes.string, dtypes.int32])
      self.assertEqual(str(cm.exception),
                       "Value passed to parameter 't' has DataType int32 "
                       "not in list of allowed values: string, bool")

  def testAttr(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("Attr", a=12, name="t")
      self.assertProtoEquals("""
        name: 't' op: 'Attr' attr { key: 'a' value { i: 12 } }
        """, op.node_def)

      op = self._lib.apply_op("Attr", a=tensor_shape.Dimension(13), name="u")
      self.assertProtoEquals("""
        name: 'u' op: 'Attr' attr { key: 'a' value { i: 13 } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Attr", a="bad")
      self.assertEqual(str(cm.exception),
                       "Expected int for argument 'a' not 'bad'.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Attr", a=[12])
      self.assertEqual(str(cm.exception),
                       "Expected int for argument 'a' not [12].")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Attr", a=None)
      self.assertEqual(str(cm.exception),
                       "Expected int for argument 'a' not None.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("Attr")
      self.assertEqual(str(cm.exception), "No argument for attr a")

  def testAttrFloat(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrFloat", a=1.2, name="t")
      self.assertProtoEquals("""
        name: 't' op: 'AttrFloat' attr { key: 'a' value { f: 1.2 } }
        """, op.node_def)

      op = self._lib.apply_op("AttrFloat", a=12, name="u")
      self.assertProtoEquals("""
        name: 'u' op: 'AttrFloat' attr { key: 'a' value { f: 12 } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("AttrFloat", a="bad")
      self.assertEqual(str(cm.exception),
                       "Expected float for argument 'a' not 'bad'.")

  def testAttrBool(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrBool", a=True, name="t")
      self.assertProtoEquals("""
        name: 't' op: 'AttrBool' attr { key: 'a' value { b: true } }
        """, op.node_def)

      op = self._lib.apply_op("AttrBool", a=False, name="u")
      self.assertProtoEquals("""
        name: 'u' op: 'AttrBool' attr { key: 'a' value { b: false } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("AttrBool", a=0)
      self.assertEqual(str(cm.exception),
                       "Expected bool for argument 'a' not 0.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("AttrBool", a=1)
      self.assertEqual(str(cm.exception),
                       "Expected bool for argument 'a' not 1.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("AttrBool", a=[])
      self.assertEqual(str(cm.exception),
                       "Expected bool for argument 'a' not [].")

  def testAttrBoolList(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrBoolList", a=[True, False, True], name="t")
      self.assertProtoEquals("""
        name: 't' op: 'AttrBoolList'
        attr { key: 'a' value { list { b: true b: false b:true } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrBoolList", a=[], name="u")
      self.assertProtoEquals("""
        name: 'u' op: 'AttrBoolList' attr { key: 'a' value { list { } } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("AttrBoolList", a=[0])
      self.assertEqual(str(cm.exception),
                       "Expected bool for argument 'a' not 0.")

  def testAttrMin(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrMin", a=12, name="s")
      self.assertProtoEquals("""
        name: 's' op: 'AttrMin' attr { key: 'a' value { i: 12 } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("AttrMin", a=2)
      self.assertEqual(str(cm.exception),
                       "Attr 'a' of 'AttrMin' Op passed 2 less than minimum 5.")

  def testAttrListMin(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrListMin", a=[1, 2], name="r")
      self.assertProtoEquals("""
        name: 'r' op: 'AttrListMin'
        attr { key: 'a' value { list { i: 1 i: 2 } } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("AttrListMin", a=[17])
      self.assertEqual(str(cm.exception),
                       "Attr 'a' of 'AttrListMin' Op "
                       "passed list of length 1 less than minimum 2.")

  def testAttrEnum(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrEnum", a="oranges", name="e")
      self.assertProtoEquals("""
        name: 'e' op: 'AttrEnum' attr { key: 'a' value { s: 'oranges' } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("AttrEnum", a="invalid")
      self.assertEqual(str(cm.exception),
                       'Attr \'a\' of \'AttrEnum\' Op '
                       'passed string \'invalid\' not in: '
                       '"apples", "oranges".')

  def testAttrEnumList(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrEnumList", a=["oranges", "apples"], name="f")
      self.assertProtoEquals("""
        name: 'f' op: 'AttrEnumList'
        attr { key: 'a' value { list { s: 'oranges' s: 'apples' } } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("AttrEnumList", a=["apples", "invalid", "oranges"])
      self.assertEqual(str(cm.exception),
                       'Attr \'a\' of \'AttrEnumList\' Op '
                       'passed string \'invalid\' not '
                       'in: "apples", "oranges".')

  def testAttrShape(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrShape", a=[5], name="s1")
      self.assertProtoEquals("""
        name: 's1' op: 'AttrShape'
        attr { key: 'a' value { shape { dim { size: 5 } } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrShape", a=(4, 3, 2), name="s2")
      self.assertProtoEquals("""
        name: 's2' op: 'AttrShape'
        attr { key: 'a' value {
          shape { dim { size: 4 } dim { size: 3 } dim { size: 2 } } } }
        """, op.node_def)

      op = self._lib.apply_op(
          "AttrShape", a=tensor_shape.TensorShape([3, 2]), name="s3")
      self.assertProtoEquals("""
        name: 's3' op: 'AttrShape'
        attr { key: 'a' value {
          shape { dim { size: 3 } dim { size: 2 } } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrShape", a=[], name="s4")
      self.assertProtoEquals("""
        name: 's4' op: 'AttrShape' attr { key: 'a' value { shape { } } }
        """, op.node_def)

      shape = tensor_shape_pb2.TensorShapeProto()
      shape.dim.add().size = 6
      shape.dim.add().size = 3
      op = self._lib.apply_op("AttrShape", a=shape, name="s5")
      self.assertProtoEquals("""
        name: 's5' op: 'AttrShape'
        attr { key: 'a' value { shape { dim { size: 6 } dim { size: 3 } } } }
        """, op.node_def)

      # TODO(josh11b): Re-enable this test once we stop promoting scalars to
      # shapes.
      # with self.assertRaises(TypeError) as cm:
      #   self._lib.apply_op("AttrShape", a=5)
      # self.assertEqual(str(cm.exception),
      #                  "Don't know how to convert 5 to a TensorShapeProto for"
      #                  " argument 'a'")

      with self.assertRaises(TypeError):
        self._lib.apply_op("AttrShape", a="ABC")

  def testAttrShapeList(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrShapeList", a=[[3, 2], [6, 5, 4]], name="sl")
      self.assertProtoEquals("""
        name: 'sl' op: 'AttrShapeList'
        attr { key: 'a' value { list {
          shape { dim { size: 3 } dim { size: 2 } }
          shape { dim { size: 6 } dim { size: 5 } dim { size: 4 } } } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrShapeList", a=[], name="esl")
      self.assertProtoEquals("""
        name: 'esl' op: 'AttrShapeList' attr { key: 'a' value { list { } } }
        """, op.node_def)

  def testAttrPartialShape(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrPartialShape", a=[5], name="s1")
      self.assertProtoEquals("""
        name: 's1' op: 'AttrPartialShape'
        attr { key: 'a' value { shape { dim { size: 5 } } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrPartialShape", a=(4, None, 2), name="s2")
      self.assertProtoEquals("""
        name: 's2' op: 'AttrPartialShape'
        attr { key: 'a' value {
          shape { dim { size: 4 } dim { size: -1 } dim { size: 2 } } } }
        """, op.node_def)

      op = self._lib.apply_op(
          "AttrPartialShape", a=tensor_shape.TensorShape([3, None]), name="s3")
      self.assertProtoEquals("""
        name: 's3' op: 'AttrPartialShape'
        attr { key: 'a' value {
          shape { dim { size: 3 } dim { size: -1 } } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrPartialShape", a=[], name="s4")
      self.assertProtoEquals("""
        name: 's4' op: 'AttrPartialShape'
        attr { key: 'a' value { shape { } } }
        """, op.node_def)

      shape = tensor_shape_pb2.TensorShapeProto()
      shape.dim.add().size = -1
      shape.dim.add().size = 3
      op = self._lib.apply_op("AttrPartialShape", a=shape, name="s5")
      self.assertProtoEquals("""
        name: 's5' op: 'AttrPartialShape'
        attr { key: 'a' value {
          shape { dim { size: -1 } dim { size: 3 } } } }
        """, op.node_def)

      # TODO(ebrevdo): Re-enable once we stop promoting scalars to shapes.
      # with self.assertRaises(TypeError) as cm:
      #   self._lib.apply_op("AttrPartialShape", a=5)
      # self.assertEqual(str(cm.exception),
      #                  "Don't know how to convert 5 to a TensorShapeProto for"
      #                  " argument 'a'")

      with self.assertRaises(TypeError):
        self._lib.apply_op("AttrPartialShape", a="ABC")

  def testAttrPartialShapeList(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op(
          "AttrPartialShapeList", a=[[3, 2], [6, None, 4]], name="sl")
      self.assertProtoEquals("""
        name: 'sl' op: 'AttrPartialShapeList'
        attr { key: 'a' value { list {
          shape { dim { size: 3 } dim { size: 2 } }
          shape { dim { size: 6 } dim { size: -1 } dim { size: 4 } } } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrPartialShapeList", a=[], name="esl")
      self.assertProtoEquals("""
        name: 'esl' op: 'AttrPartialShapeList' attr {
          key: 'a' value { list { } } }
        """, op.node_def)

  def testAttrDefault(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrDefault", a=None, name="d")
      self.assertProtoEquals("""
        name: 'd' op: 'AttrDefault' attr { key: 'a' value { s: 'banana' } }
        """, op.node_def)

      op = self._lib.apply_op("AttrDefault", a="kiwi", name="c")
      self.assertProtoEquals("""
        name: 'c' op: 'AttrDefault' attr { key: 'a' value { s: 'kiwi' } }
        """, op.node_def)

  def testAttrListDefault(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrListDefault", a=None, name="b")
      self.assertProtoEquals("""
        name: 'b' op: 'AttrListDefault'
        attr { key: 'a' value { list { i: 5 i: 15 } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrListDefault", a=[3], name="a")
      self.assertProtoEquals("""
        name: 'a' op: 'AttrListDefault'
        attr { key: 'a' value { list { i: 3 } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrListDefault", a=[], name="empty")
      self.assertProtoEquals("""
        name: 'empty' op: 'AttrListDefault'
        attr { key: 'a' value { list { } } }
        """, op.node_def)

  def testAttrEmptyListDefault(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("AttrEmptyListDefault", a=None, name="b")
      self.assertProtoEquals("""
        name: 'b' op: 'AttrEmptyListDefault'
        attr { key: 'a' value { list { } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrEmptyListDefault", a=[3], name="a")
      self.assertProtoEquals("""
        name: 'a' op: 'AttrEmptyListDefault'
        attr { key: 'a' value { list { f: 3 } } }
        """, op.node_def)

      op = self._lib.apply_op("AttrEmptyListDefault", a=[], name="empty")
      self.assertProtoEquals("""
        name: 'empty' op: 'AttrEmptyListDefault'
        attr { key: 'a' value { list { } } }
        """, op.node_def)

  def testReservedAttr(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("ReservedAttr", range_=7, name="x")
      self.assertProtoEquals("""
        name: 'x' op: 'ReservedAttr' attr { key: 'range' value { i: 7 } }
        """, op.node_def)

  def testDefaultAttrType(self):
    with ops.Graph().as_default():
      # Give an input whose type has no obvious output type.
      op = self._lib.apply_op("AttrTypeDefault", a=[], name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'AttrTypeDefault' input: 'n/a'
        attr { key: 'T' value { type: DT_INT32 } }
        """, op.node_def)

      # Give an input whose type can be inferred as different
      # than the default.
      op = self._lib.apply_op("AttrTypeDefault", a=[1.0], name="f")
      self.assertProtoEquals("""
        name: 'f' op: 'AttrTypeDefault' input: 'f/a'
        attr { key: 'T' value { type: DT_FLOAT } }
        """, op.node_def)

  def testDefaultListAttrType(self):
    with ops.Graph().as_default():
      # Give an input whose type can be inferred as different
      # than the default.
      op = self._lib.apply_op("AttrListTypeDefault", a=[1.0], b=[2.0], name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'AttrListTypeDefault' input: 'n/a_0' input: 'n/b_0'
        attr { key: 'T' value { type: DT_FLOAT } }
        attr { key: 'N' value { i: 1 } }
        """, op.node_def)

  def testNIntsIn(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("NIntsIn", a=[1, 2], name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'NIntsIn' input: 'n/a_0' input: 'n/a_1'
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NIntsIn", a=[5, 4, 3, 2, 1], name="o")
      self.assertProtoEquals("""
        name: 'o' op: 'NIntsIn'
        input: 'o/a_0' input: 'o/a_1' input: 'o/a_2' input: 'o/a_3' input: 'o/a_4'
        attr { key: 'N' value { i: 5 } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NIntsIn", a=["foo", "bar"])
      self.assertEqual(
          str(cm.exception),
          "Tensors in list passed to 'a' of 'NIntsIn' Op have types "
          "[string, string] that do not match expected type int32.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NIntsIn",
                           a=[self.Tensor(dtypes.string),
                              self.Tensor(dtypes.string)])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'a' of 'NIntsIn' Op have "
                       "types [string, string] that do not match expected type "
                       "int32.")

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NIntsIn", a=[99])
      self.assertEqual(str(cm.exception),
                       "List argument 'a' to 'NIntsIn' Op "
                       "with length 1 shorter than "
                       "minimum length 2.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NIntsIn", a=[38, "bar"])
      self.assertEqual(
          str(cm.exception),
          "Tensors in list passed to 'a' of 'NIntsIn' Op have types "
          "[int32, string] that do not match expected type int32.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NIntsIn",
                           a=[self.Tensor(dtypes.int32),
                              self.Tensor(dtypes.string)])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'a' of 'NIntsIn' Op "
                       "have types [int32, string] that do not match expected "
                       "type int32.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NIntsIn", a=17)
      self.assertStartsWith(str(cm.exception),
                            "Expected list for 'a' argument "
                            "to 'NIntsIn' Op, not ")

  def testNPolymorphicIn(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("NPolymorphicIn", a=[1, 2], name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'NPolymorphicIn' input: 'n/a_0' input: 'n/a_1'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NPolymorphicIn", a=[5, 4, 3, 2, 1], name="o")
      self.assertProtoEquals("""
        name: 'o' op: 'NPolymorphicIn'
        input: 'o/a_0' input: 'o/a_1' input: 'o/a_2' input: 'o/a_3' input: 'o/a_4'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 5 } }
        """, op.node_def)

      op = self._lib.apply_op("NPolymorphicIn", a=["foo", "bar"], name="p")
      self.assertProtoEquals("""
        name: 'p' op: 'NPolymorphicIn' input: 'p/a_0' input: 'p/a_1'
        attr { key: 'T' value { type: DT_STRING } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NPolymorphicIn",
                              a=[1, self.Tensor(dtypes.float32, name="x")],
                              name="q")
      self.assertProtoEquals("""
        name: 'q' op: 'NPolymorphicIn' input: 'q/a_0' input: 'x'
        attr { key: 'T' value { type: DT_FLOAT } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NPolymorphicIn",
                              a=[self.Tensor(dtypes.float32, name="y"),
                                 self.Tensor(dtypes.float32_ref, name="z")],
                              name="r")
      self.assertProtoEquals("""
        name: 'r' op: 'NPolymorphicIn' input: 'y' input: 'z'
        attr { key: 'T' value { type: DT_FLOAT } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NPolymorphicIn", a=[99])
      self.assertEqual(str(cm.exception),
                       "List argument 'a' to 'NPolymorphicIn' Op with length 1 "
                       "shorter than minimum length 2.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicIn", a=[38, "bar"])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'a' of 'NPolymorphicIn' Op "
                       "have types [int32, string] that don't all match.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicIn", a=[38, self.Tensor(dtypes.string)])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'a' of 'NPolymorphicIn' Op "
                       "have types [int32, string] that don't all match.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicIn", a=[38, None])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'a' of 'NPolymorphicIn' Op "
                       "have types [int32, <NOT CONVERTIBLE TO TENSOR>] that "
                       "don't all match.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicIn",
                           a=["abcd", self.Tensor(dtypes.int32)])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'a' of 'NPolymorphicIn' Op "
                       "have types [string, int32] that don't all match.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicIn", a=17)
      self.assertStartsWith(str(cm.exception),
                            "Expected list for 'a' argument "
                            "to 'NPolymorphicIn' Op, not ")

  def testNPolymorphicRestrictIn(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("NPolymorphicRestrictIn", a=["foo", "bar"],
                              name="p")
      self.assertProtoEquals("""
        name: 'p' op: 'NPolymorphicRestrictIn' input: 'p/a_0' input: 'p/a_1'
        attr { key: 'T' value { type: DT_STRING } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NPolymorphicRestrictIn",
                              a=[False, True, False],
                              name="b")
      self.assertProtoEquals("""
        name: 'b' op: 'NPolymorphicRestrictIn'
        input: 'b/a_0' input: 'b/a_1' input: 'b/a_2'
        attr { key: 'T' value { type: DT_BOOL } }
        attr { key: 'N' value { i: 3 } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicRestrictIn", a=[1, 2])
      self.assertEqual(
          str(cm.exception),
          "Value passed to parameter 'a' has DataType int32 not in "
          "list of allowed values: string, bool")

  def testNInTwice(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("NInTwice", a=[1, 2], b=["one", "two"], name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'NInTwice'
        input: 'n/a_0' input: 'n/a_1' input: 'n/b_0' input: 'n/b_1'
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NInTwice", a=[], b=[], name="o")
      self.assertProtoEquals("""
        name: 'o' op: 'NInTwice' attr { key: 'N' value { i: 0 } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NInTwice", a=[1, 2, 3], b=["too short"])
      self.assertEqual(str(cm.exception),
                       "List argument 'b' to 'NInTwice' Op "
                       "with length 1 must match "
                       "length 3 of argument 'a'.")

  def testNInPolymorphicTwice(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("NInPolymorphicTwice", a=[1, 2], b=[3, 4],
                              name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'NInPolymorphicTwice'
        input: 'n/a_0' input: 'n/a_1' input: 'n/b_0' input: 'n/b_1'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NInPolymorphicTwice", a=[1, 2, 3], b=[5])
      self.assertEqual(str(cm.exception),
                       "List argument 'b' to 'NInPolymorphicTwice' Op "
                       "with length 1 "
                       "must match length 3 of argument 'a'.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NInPolymorphicTwice", a=[1, 2], b=["one", "two"])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'b' of 'NInPolymorphicTwice' "
                       "Op have types [string, string] that do not match type "
                       "int32 inferred from earlier arguments.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NInPolymorphicTwice",
                           a=[self.Tensor(dtypes.int32)],
                           b=[self.Tensor(dtypes.string)])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'b' of "
                       "'NInPolymorphicTwice' Op have types [string] that do "
                       "not match type int32 inferred from earlier arguments.")

  def testNInTwoTypeVariables(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("NInTwoTypeVariables",
                              a=[1, 2],
                              b=[True, False],
                              name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'NInTwoTypeVariables'
        input: 'n/a_0' input: 'n/a_1' input: 'n/b_0' input: 'n/b_1'
        attr { key: 'S' value { type: DT_INT32 } }
        attr { key: 'T' value { type: DT_BOOL } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NInTwoTypeVariables", a=[1, 2], b=[3, 4],
                              name="o")
      self.assertProtoEquals("""
        name: 'o' op: 'NInTwoTypeVariables'
        input: 'o/a_0' input: 'o/a_1' input: 'o/b_0' input: 'o/b_1'
        attr { key: 'S' value { type: DT_INT32 } }
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 2 } }
        """, op.node_def)

      op = self._lib.apply_op("NInTwoTypeVariables",
                              a=[self.Tensor(dtypes.int32, name="q")],
                              b=[self.Tensor(dtypes.string, name="r")],
                              name="p")
      self.assertProtoEquals("""
        name: 'p' op: 'NInTwoTypeVariables' input: 'q' input: 'r'
        attr { key: 'S' value { type: DT_INT32 } }
        attr { key: 'T' value { type: DT_STRING } }
        attr { key: 'N' value { i: 1 } }
        """, op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NInTwoTypeVariables", a=[1, 2, 3], b=["5"])
      self.assertEqual(str(cm.exception),
                       "List argument 'b' to 'NInTwoTypeVariables' Op "
                       "with length 1 "
                       "must match length 3 of argument 'a'.")

  def testInPolymorphicTwice(self):
    with ops.Graph().as_default():
      op = self._lib.apply_op("InPolymorphicTwice", a=[8], b=[3, 4, 5],
                              name="n")
      self.assertProtoEquals("""
        name: 'n' op: 'InPolymorphicTwice'
        input: 'n/a_0' input: 'n/b_0' input: 'n/b_1' input: 'n/b_2'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 1 } }
        attr { key: 'M' value { i: 3 } }
        """, op.node_def)

      op = self._lib.apply_op("InPolymorphicTwice", a=[8], b=[], name="o")
      self.assertProtoEquals("""
        name: 'o' op: 'InPolymorphicTwice' input: 'o/a_0'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 1 } }
        attr { key: 'M' value { i: 0 } }
        """, op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("InPolymorphicTwice", a=[], b=[3, 4, 5])
      self.assertEqual(str(cm.exception),
                       "Don't know how to infer type variable from empty input "
                       "list passed to input 'a' of 'InPolymorphicTwice' Op.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("InPolymorphicTwice", a=[1, 2], b=["one", "two"])
      self.assertEqual(
          str(cm.exception),
          "Tensors in list passed to 'b' of 'InPolymorphicTwice' Op "
          "have types [string, string] that do not match type int32 "
          "inferred from earlier arguments.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("InPolymorphicTwice",
                           a=[self.Tensor(dtypes.int32)],
                           b=[self.Tensor(dtypes.string)])
      self.assertEqual(str(cm.exception),
                       "Tensors in list passed to 'b' of 'InPolymorphicTwice' "
                       "Op have types [string] that do not match type int32 "
                       "inferred from earlier arguments.")

  def testNIntsOut(self):
    with ops.Graph().as_default():
      out1, out2 = self._lib.apply_op("NIntsOut", N=2, name="n")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertProtoEquals("""
        name: 'n' op: 'NIntsOut' attr { key: 'N' value { i: 2 } }
        """, out1.op.node_def)

      out1, out2, out3, out4, out5 = self._lib.apply_op(
          "NIntsOut", N=5, name="o")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertEqual(dtypes.int32, out3.dtype)
      self.assertEqual(dtypes.int32, out4.dtype)
      self.assertEqual(dtypes.int32, out5.dtype)
      self.assertProtoEquals("""
        name: 'o' op: 'NIntsOut' attr { key: 'N' value { i: 5 } }
        """, out5.op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NIntsOut", N=1)
      self.assertEqual(
          str(cm.exception),
          "Attr 'N' of 'NIntsOut' Op passed 1 less than minimum 2.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NIntsOut", N=[3])
      self.assertEqual(str(cm.exception),
                       "Expected int for argument 'N' not [3].")

  def testNIntsOutDefault(self):
    with ops.Graph().as_default():
      out1, out2, out3 = self._lib.apply_op(
          "NIntsOutDefault", N=None, name="z")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertEqual(dtypes.int32, out3.dtype)
      self.assertProtoEquals("""
        name: 'z' op: 'NIntsOutDefault' attr { key: 'N' value { i: 3 } }
        """, out1.op.node_def)

      out1, out2 = self._lib.apply_op("NIntsOutDefault", N=2, name="y")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertProtoEquals("""
        name: 'y' op: 'NIntsOutDefault' attr { key: 'N' value { i: 2 } }
        """, out2.op.node_def)

  def testNPolymorphicOut(self):
    with ops.Graph().as_default():
      out1, out2 = self._lib.apply_op("NPolymorphicOut",
                                      N=2,
                                      T=dtypes.int32,
                                      name="n")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertProtoEquals("""
        name: 'n' op: 'NPolymorphicOut'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 2 } }
        """, out1.op.node_def)

      out1, out2, out3 = self._lib.apply_op(
          "NPolymorphicOut", T=dtypes.string, N=3, name="o")
      self.assertEqual(dtypes.string, out1.dtype)
      self.assertEqual(dtypes.string, out2.dtype)
      self.assertEqual(dtypes.string, out3.dtype)
      self.assertProtoEquals("""
        name: 'o' op: 'NPolymorphicOut'
        attr { key: 'T' value { type: DT_STRING } }
        attr { key: 'N' value { i: 3 } }
        """, out3.op.node_def)

      with self.assertRaises(ValueError) as cm:
        self._lib.apply_op("NPolymorphicOut", N=1, T=dtypes.string)
      self.assertEqual(str(cm.exception),
                       "Attr 'N' of 'NPolymorphicOut' Op "
                       "passed 1 less than minimum 2.")

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicOut", N=3, T=[dtypes.string])
      self.assertEqual(
          str(cm.exception),
          "Expected DataType for argument 'T' not [tf.string].")

  def testNPolymorphicOutDefault(self):
    with ops.Graph().as_default():
      out1, out2 = self._lib.apply_op(
          "NPolymorphicOutDefault", N=None, T=None, name="r")
      self.assertEqual(dtypes.bool, out1.dtype)
      self.assertEqual(dtypes.bool, out2.dtype)
      self.assertProtoEquals("""
        name: 'r' op: 'NPolymorphicOutDefault'
        attr { key: 'T' value { type: DT_BOOL } }
        attr { key: 'N' value { i: 2 } }
        """, out1.op.node_def)

      out1, out2, out3 = self._lib.apply_op(
          "NPolymorphicOutDefault", N=3, T=None, name="s")
      self.assertEqual(dtypes.bool, out1.dtype)
      self.assertEqual(dtypes.bool, out2.dtype)
      self.assertEqual(dtypes.bool, out3.dtype)
      self.assertProtoEquals("""
        name: 's' op: 'NPolymorphicOutDefault'
        attr { key: 'T' value { type: DT_BOOL } }
        attr { key: 'N' value { i: 3 } }
        """, out1.op.node_def)

      out1, out2 = self._lib.apply_op(
          "NPolymorphicOutDefault", N=None, T=dtypes.int32, name="t")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertProtoEquals("""
        name: 't' op: 'NPolymorphicOutDefault'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 2 } }
        """, out1.op.node_def)

      out1, out2, out3 = self._lib.apply_op(
          "NPolymorphicOutDefault", N=3, T=dtypes.int32, name="u")
      self.assertEqual(dtypes.int32, out1.dtype)
      self.assertEqual(dtypes.int32, out2.dtype)
      self.assertEqual(dtypes.int32, out3.dtype)
      self.assertProtoEquals("""
        name: 'u' op: 'NPolymorphicOutDefault'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: 'N' value { i: 3 } }
        """, out1.op.node_def)

  def testNPolymorphicRestrictOut(self):
    with ops.Graph().as_default():
      out1, out2, out3 = self._lib.apply_op(
          "NPolymorphicRestrictOut", N=3, T=dtypes.bool, name="u")
      self.assertEqual(dtypes.bool, out1.dtype)
      self.assertEqual(dtypes.bool, out2.dtype)
      self.assertEqual(dtypes.bool, out3.dtype)
      self.assertProtoEquals("""
        name: 'u' op: 'NPolymorphicRestrictOut'
        attr { key: 'T' value { type: DT_BOOL } }
        attr { key: 'N' value { i: 3 } }
        """, out1.op.node_def)

      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("NPolymorphicRestrictOut", N=2, T=dtypes.int32)
      self.assertEqual(str(cm.exception),
                       "Value passed to parameter 'T' has DataType int32 "
                       "not in list of allowed values: string, bool")

  def testRef(self):
    with ops.Graph().as_default():
      out = self._lib.apply_op("RefOut", T=dtypes.bool, name="o")
      self.assertEqual(dtypes.bool_ref, out.dtype)
      self.assertProtoEquals("""
        name: 'o' op: 'RefOut'
        attr { key: 'T' value { type: DT_BOOL } }
        """, out.op.node_def)

      op = self._lib.apply_op("RefIn", a=out, name="i")
      self.assertProtoEquals("""
        name: 'i' op: 'RefIn' input: 'o'
        attr { key: 'T' value { type: DT_BOOL } }
        attr { key: "_class" value { list { s: "loc:@o" } } }
        """, op.node_def)

      # Can pass ref to non-ref input.
      out = self._lib.apply_op("RefOut", T=dtypes.int32, name="r")
      out = self._lib.apply_op("Simple", a=out, name="s")
      self.assertProtoEquals("""
        name: 's' op: 'Simple' input: 'r'
        """, out.op.node_def)

      # Can't pass non-ref to ref input.
      with self.assertRaises(TypeError) as cm:
        self._lib.apply_op("RefIn", a=2)
      self.assertEqual(
          str(cm.exception),
          "'RefIn' Op requires that input 'a' be a mutable tensor " +
          "(e.g.: a tf.Variable)")

      input_a = self._lib.apply_op("RefOut", T=dtypes.int32, name="t")
      input_b = self._lib.apply_op("RefOut", T=dtypes.int32, name="u")
      op = self._lib.apply_op("TwoRefsIn", a=input_a, b=input_b, name="v")
      # NOTE(mrry): The order of colocation constraints is an implementation
      # detail.
      self.assertProtoEquals("""
        name: 'v' op: 'TwoRefsIn' input: 't' input: 'u'
        attr { key: 'T' value { type: DT_INT32 } }
        attr { key: "_class" value { list { s: "loc:@t" s: "loc:@u" } } }
        """, op.node_def)

  def testSpecifyDevice(self):
    graph = ops.Graph()
    with graph.as_default():
      with graph.device("/job:ADevice"):
        self._lib.apply_op("Simple", a=3)
      # We look at the whole graph here to make sure the Const op is also given
      # the specified device.
      graph_def = graph.as_graph_def()
      self.assertEqual(len(graph_def.node), 2)
      for node in graph_def.node:
        self.assertDeviceEqual(node.device, "/job:ADevice")

  def testStructuredOutputSingleList(self):
    with ops.Graph().as_default():
      for n_a in [0, 1, 3]:
        a = self._lib.apply_op("SimpleStruct", n_a=n_a)
        self.assertTrue(isinstance(a, list))
        self.assertEqual(n_a, len(a))

  def testStructuredOutputListAndSingle(self):
    with ops.Graph().as_default():
      for n_a in [0, 1, 3]:
        a, b = self._lib.apply_op("MixedStruct", n_a=n_a)
        self.assertTrue(isinstance(a, list))
        self.assertEqual(n_a, len(a))
        self.assertTrue(all(x.dtype == dtypes.int32 for x in a))
        self.assertTrue(isinstance(b, ops.Tensor))
        self.assertEqual(dtypes.float32, b.dtype)

  def testStructuredOutputMultipleLists(self):
    with ops.Graph().as_default():
      for n_a in [0, 1, 3]:
        for n_b in [0, 1, 3]:
          for t_c in [[],
                      [dtypes.int32],
                      [dtypes.int32, dtypes.float32]]:
            a, b, c = self._lib.apply_op("ComplexStruct",
                                         n_a=n_a,
                                         n_b=n_b,
                                         t_c=t_c)

            self.assertEqual(n_a, len(a))
            self.assertTrue(all(x.dtype == dtypes.int32 for x in a))
            self.assertEqual(n_b, len(b))
            self.assertTrue(all(x.dtype == dtypes.int64 for x in b))
            self.assertEqual(t_c, [x.dtype for x in c])


class OpDefLibraryGraphTest(test_util.TensorFlowTestCase):

  def setUp(self):
    self._lib = test_ops._op_def_lib

  def _add_op(self, ascii):  # pylint: disable=redefined-builtin
    op_def = op_def_pb2.OpDef()
    text_format.Merge(ascii, op_def)
    self._lib.add_op(op_def)

  def testNoGraph(self):
    out = self._lib.apply_op("Simple", a=3)
    self.assertEqual(out.graph, ops.get_default_graph())

  def testDefaultGraph(self):
    graph = ops.Graph()
    with graph.as_default():
      out = self._lib.apply_op("Simple", a=3)
      self.assertEqual(out.graph, graph)

  def testDifferentGraphFails(self):
    with ops.Graph().as_default():
      a = self._lib.apply_op("Simple", a=3)
    with ops.Graph().as_default():
      b = self._lib.apply_op("Simple", a=4)
    with self.assertRaises(ValueError) as cm:
      self._lib.apply_op("Binary", a=a, b=b)
    self.assertTrue("must be from the same graph" in str(cm.exception))


if __name__ == "__main__":
  googletest.main()
