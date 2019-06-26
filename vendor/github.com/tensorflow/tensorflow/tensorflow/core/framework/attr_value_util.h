/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#ifndef TENSORFLOW_CORE_FRAMEWORK_ATTR_VALUE_UTIL_H_
#define TENSORFLOW_CORE_FRAMEWORK_ATTR_VALUE_UTIL_H_

#include <functional>
#include <string>
#include <vector>

#include "tensorflow/core/framework/partial_tensor_shape.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/core/stringpiece.h"
#include "tensorflow/core/lib/gtl/array_slice.h"

namespace tensorflow {

// Forward declare protos so their symbols can be removed from .so exports
class AttrValue;
class NameAttrList;

// A human-readable rendering of attr_value, that is more concise than a
// text-format proto.
string SummarizeAttrValue(const AttrValue& attr_value);

// Generates an error if attr_value doesn't have the indicated attr type.
Status AttrValueHasType(const AttrValue& attr_value, StringPiece type);

// Converts a text proto value from "text" into the field of *out
// indicated by "type" (e.g. from the type field of an AttrDef).
// Examples:
// * If type:"int" and text:"-14", then *out is set to "i: -14"
// * If type:"list(string)" and text:"['foo', 'bar']",
//   then *out is set to "list { s: ['foo', 'bar'] }"
// Returns true on success.
bool ParseAttrValue(StringPiece type, StringPiece text, AttrValue* out);

// Sets *out based on the type of value.
void SetAttrValue(const string& value, AttrValue* out);
void SetAttrValue(const char* value, AttrValue* out);
void SetAttrValue(StringPiece value, AttrValue* out);
void SetAttrValue(int64 value, AttrValue* out);
void SetAttrValue(int32 value, AttrValue* out);
void SetAttrValue(float value, AttrValue* out);
void SetAttrValue(double value, AttrValue* out);
void SetAttrValue(bool value, AttrValue* out);
void SetAttrValue(DataType value, AttrValue* out);
void SetAttrValue(const TensorShape& value, AttrValue* out);
void SetAttrValue(const TensorShapeProto& value, AttrValue* out);
void SetAttrValue(const PartialTensorShape& value, AttrValue* out);
void SetAttrValue(const Tensor& value, AttrValue* out);
void SetAttrValue(const TensorProto& value, AttrValue* out);
void SetAttrValue(const NameAttrList& value, AttrValue* out);

void SetAttrValue(gtl::ArraySlice<string> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<const char*> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<StringPiece> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<int64> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<int32> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<float> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<double> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<bool> value, AttrValue* out);
void SetAttrValue(const std::vector<bool>& value, AttrValue* out);
void SetAttrValue(std::initializer_list<bool> value, AttrValue* out);
void SetAttrValue(DataTypeSlice value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<TensorShape> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<TensorShapeProto> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<PartialTensorShape> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<Tensor> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<TensorProto> value, AttrValue* out);
void SetAttrValue(gtl::ArraySlice<NameAttrList> value, AttrValue* out);

void SetAttrValue(const AttrValue& value, AttrValue* out);

// Returns true if a and b have the same value.
bool AreAttrValuesEqual(const AttrValue& a, const AttrValue& b);

// Returns a hash of `a` that is consistent with AreAttrValuesEqual. In other
// words, if two AttrValues compare equal according to AreAttrValuesEqual,
// they will have the same hash value.
// Similarly to protobuf deterministic serialization, hash value is
// guaranteed to be stable only for a given binary. In particular, one should
// probably not persist the returned value.
uint64 AttrValueHash(const AttrValue& a);

// WARNING: Equality check might return false-negative for large (> 32mb)
// tensors defined with different TensorProto representations.
//
// A pair of consistent hash and equals functions that are guaranteed to be fast
// with AttrValues that potentially can have very large Tensors (larger than
// 32mb) defined by TensorProto. If large identical Tensors are defined using
// different representations (e.g. one with tensor content, and second with
// bool_val), they will have different hash code and equals will return false.
// Small (less than 32mb) tensors with different TensorProto representations
// hashed/compared by their tensor content.
uint64 FastAttrValueHash(const AttrValue& a);
bool FastAreAttrValuesEqual(const AttrValue& a, const AttrValue& b);

// Returns true if "val" has a placeholder.
bool HasPlaceHolder(const AttrValue& val);

// SubstitutePlaceholders recursively replaces placeholders in 'value'
// with an attr value by calling SubstituteFunc. Returns true iff all
// placeholders in "value" are replaced with a value.
//
// SubstituteFunc is given a placeholder string. If the placeholder is
// unknown, SubstituteFunc returns false. Otherwise, overwrites the
// attr value and returns true.
using SubstituteFunc = std::function<bool(const string&, AttrValue*)>;
bool SubstitutePlaceholders(const SubstituteFunc& substitute, AttrValue* value);

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_FRAMEWORK_ATTR_VALUE_UTIL_H_
