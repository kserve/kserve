/* Copyright 2017 The TensorFlow Authors. All Rights Reserved.

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

// Generally useful utility functions that are common to (not specific to any
// given part of) the XLA code base.

#ifndef TENSORFLOW_COMPILER_XLA_UTIL_H_
#define TENSORFLOW_COMPILER_XLA_UTIL_H_

#include <algorithm>
#include <string>
#include <type_traits>
#include <vector>

#include "absl/algorithm/container.h"
#include "absl/container/inlined_vector.h"
#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/string_view.h"
#include "absl/types/span.h"
#include "tensorflow/compiler/xla/status.h"
#include "tensorflow/compiler/xla/status_macros.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/math/math_util.h"
#include "tensorflow/core/lib/strings/numbers.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/protobuf.h"
#include "tensorflow/core/platform/types.h"

namespace xla {

// Logs the provided status message with a backtrace.
//
// For use by Status-factories, logs a backtrace at the point where the status
// is created, such that we can use --vmodule=util=1 to see all status
// creation backtraces.
Status WithLogBacktrace(const Status& status);

// Ranks greater than 8 are very rare, so use InlinedVector<int64, 8> to store
// the bounds and indices. And for the rare cases of ranks greater than 8,
// the InlinedVector will just behave like an std::vector<> and allocate the
// memory to store its values.
static constexpr int kInlineRank = 8;
using DimensionVector = absl::InlinedVector<int64, kInlineRank>;

// RAII timer that logs with a given label the wall clock time duration in human
// readable form. This differs from base's ElapsedTimer primarily in that it
// spits out the human-readable duration form.
//
// By default, the timing traces are only printed at VLOG(1) and above:
//
//   XLA_SCOPED_LOGGING_TIMER("fooing bar");  // nop if !VLOG_IS_ON(1).
//
// but you can control this via:
//
//   XLA_SCOPED_LOGGING_TIMER_LEVEL("fooing bar", 2);  // nop if !VLOG_IS_ON(2)
//
#define XLA_SCOPED_LOGGING_TIMER(label) \
  XLA_SCOPED_LOGGING_TIMER_HELPER(label, 1, __COUNTER__)
#define XLA_SCOPED_LOGGING_TIMER_LEVEL(label, level) \
  XLA_SCOPED_LOGGING_TIMER_HELPER(label, level, __COUNTER__)

// Helper for implementing macros above.  Do not use directly.
//
// Forces the evaluation of "counter", which we expect is equal to __COUNTER__.
#define XLA_SCOPED_LOGGING_TIMER_HELPER(label, level, counter) \
  XLA_SCOPED_LOGGING_TIMER_HELPER2(label, level, counter)

// Helper for macros above.  Don't use directly.
#define XLA_SCOPED_LOGGING_TIMER_HELPER2(label, level, counter)      \
  ::xla::ScopedLoggingTimer XLA_ScopedLoggingTimerInstance##counter( \
      label, VLOG_IS_ON(level))

// RAII timer for XLA_SCOPED_LOGGING_TIMER and XLA_SCOPED_LOGGING_TIMER_LEVEL
// macros above.  Recommended usage is via the macros so you don't have to give
// the timer a name or worry about calling VLOG_IS_ON yourself.
struct ScopedLoggingTimer {
  // The timer does nothing if enabled is false.  This lets you pass in your
  // file's VLOG_IS_ON value.
  ScopedLoggingTimer(const string& label, bool enabled);
  ~ScopedLoggingTimer();

  bool enabled;
  string label;
  uint64 start_micros;
};

// Given a vector<T>, returns a Span<char> that points at its
// internals.
//
// Warning: if the vector is updated its storage pointer may change, so use this
// with caution (ideally in limited scopes with temporary lifetimes).
template <typename T>
absl::Span<uint8> MutableByteSlice(std::vector<T>* v) {
  return absl::Span<uint8>(reinterpret_cast<uint8*>(v->data()),
                           v->size() * sizeof(T));
}

// Turns an immutable slice of type T into an immutable slice of bytes with the
// same byte size.
template <typename T>
absl::Span<const uint8> CastToByteSlice(absl::Span<const T> slice) {
  return absl::Span<const uint8>(reinterpret_cast<const uint8*>(slice.data()),
                                 slice.size() * sizeof(T));
}

// Casts a byte slice to a non-byte type T, checking that the original slice
// length is a multiple of sizeof(T).
template <typename T>
absl::Span<const T> CastByteSlice(absl::Span<const uint8> slice) {
  CHECK_EQ(0, slice.size() % sizeof(T));
  return absl::Span<const T>(reinterpret_cast<const T*>(slice.data()),
                             slice.size() / sizeof(T));
}

// Convenience function to force a vector to convert to an immutable slice.
template <typename T>
absl::Span<const T> AsSlice(const std::vector<T>& v) {
  return absl::Span<const T>(v);
}

// Converts a mutable vector pointer into a Span of the same
// type.
template <typename T>
absl::Span<T> AsMutableSlice(std::vector<T>* v) {
  return absl::Span<T>(v->data(), v->size());
}

// xla::int64 is not the same type as tensorflow::protobuf_int64 in open-source.
// Wrapper function that gives an int64 array slice view of a repeated int64
// protobuf field.
static inline absl::Span<const int64> AsInt64Slice(
    const tensorflow::protobuf::RepeatedField<tensorflow::protobuf_int64>& v) {
  absl::Span<const tensorflow::protobuf_int64> slice(v);
  return absl::Span<const int64>(reinterpret_cast<const int64*>(slice.data()),
                                 slice.size());
}

// TODO(b/29771030): This nop overload was added to simplify the migration of
// Shape from a proto to a C++ class. Remove after class has been migrated.
static inline absl::Span<const int64> AsInt64Slice(
    absl::Span<const int64> slice) {
  return slice;
}

// As above, but for uint64 types.
static inline absl::Span<const uint64> AsUInt64Slice(
    const tensorflow::protobuf::RepeatedField<tensorflow::protobuf_uint64>& v) {
  absl::Span<const tensorflow::protobuf_uint64> slice(v);
  return absl::Span<const uint64>(reinterpret_cast<const uint64*>(slice.data()),
                                  slice.size());
}

// Compares two containers for equality. Returns true iff the two containers
// have the same size and all their elements compare equal using their
// operator==. Like std::equal, but forces size equality.
template <typename Container1T, typename Container2T>
bool ContainersEqual(const Container1T& c1, const Container2T& c2) {
  return ((c1.size() == c2.size()) &&
          std::equal(std::begin(c1), std::end(c1), std::begin(c2)));
}

template <typename Container1T,
          typename ElementType = typename Container1T::value_type>
bool ContainersEqual(const Container1T& c1,
                     std::initializer_list<ElementType> il) {
  absl::Span<const ElementType> c2{il};
  return ContainersEqual(c1, c2);
}

// Compares two containers for equality. Returns true iff the two containers
// have the same size and all their elements compare equal using the predicate
// p. Like std::equal, but forces size equality.
template <typename Container1T, typename Container2T, class PredicateT>
bool ContainersEqual(const Container1T& c1, const Container2T& c2,
                     PredicateT p) {
  return ((c1.size() == c2.size()) &&
          std::equal(std::begin(c1), std::end(c1), std::begin(c2), p));
}

// Performs a copy of count values from src to dest, using different strides for
// source and destination. The source starting index is src_base, while the
// destination one is dest_base.
template <typename D, typename S>
void StridedCopy(absl::Span<D> dest, int64 dest_base, int64 dest_stride,
                 absl::Span<const S> src, int64 src_base, int64 src_stride,
                 int64 count) {
  for (; count > 0; --count, dest_base += dest_stride, src_base += src_stride) {
    dest[dest_base] = static_cast<D>(src[src_base]);
  }
}

// Adds some context information to the error message in a
// Status.  This is useful as Statuses are
// propagated upwards.
Status AddStatus(Status prior, absl::string_view context);
Status AppendStatus(Status prior, absl::string_view context);

// Status error shorthands -- StrFormat's the arguments to be used as an error
// message and returns a status in the canonical error space.
template <typename... Args>
Status InvalidArgument(const absl::FormatSpec<Args...>& format,
                       const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::InvalidArgument(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status Unimplemented(const absl::FormatSpec<Args...>& format,
                     const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::Unimplemented(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status InternalError(const absl::FormatSpec<Args...>& format,
                     const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::Internal(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status FailedPrecondition(const absl::FormatSpec<Args...>& format,
                          const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::FailedPrecondition(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status Cancelled(const absl::FormatSpec<Args...>& format, const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::Cancelled(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status ResourceExhausted(const absl::FormatSpec<Args...>& format,
                         const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::ResourceExhausted(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status NotFound(const absl::FormatSpec<Args...>& format, const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::NotFound(absl::StrFormat(format, args...)));
}
template <typename... Args>
Status Unavailable(const absl::FormatSpec<Args...>& format,
                   const Args&... args) {
  return WithLogBacktrace(
      tensorflow::errors::Unavailable(absl::StrFormat(format, args...)));
}

template <typename... Args>
Status InvalidArgumentStrCat(Args&&... concat) {
  return InvalidArgument("%s", absl::StrCat(std::forward<Args>(concat)...));
}

template <typename... Args>
Status UnimplementedStrCat(Args&&... concat) {
  return Unimplemented("%s", absl::StrCat(std::forward<Args>(concat)...));
}

template <typename... Args>
Status InternalErrorStrCat(Args&&... concat) {
  return InternalError("%s", absl::StrCat(std::forward<Args>(concat)...));
}

template <typename... Args>
Status ResourceExhaustedStrCat(Args&&... concat) {
  return ResourceExhausted("%s", absl::StrCat(std::forward<Args>(concat)...));
}

// Splits the lines of the original, replaces leading whitespace with the prefix
// given by "indentation", and returns the string joined by newlines again. As a
// side effect, any additional trailing whitespace is removed.
//
// Note: even different amounts of leading whitespace on different lines will be
// uniformly replaced with "indentation".
string Reindent(absl::string_view original, absl::string_view indentation);

// Checks whether permutation is a permutation of the [0, rank) integer range.
bool IsPermutation(absl::Span<const int64> permutation, int64 rank);

// Applies `permutation` on `input` and returns the permuted array.
// For each i, output[permutation[i]] = input[i].
//
// Precondition:
// 1. `permutation` is a permutation of 0..permutation.size()-1.
// 2. permutation.size() == input.size().
template <typename Container>
std::vector<typename Container::value_type> Permute(
    absl::Span<const int64> permutation, const Container& input) {
  using T = typename Container::value_type;
  absl::Span<const T> data(input);
  CHECK(IsPermutation(permutation, data.size()));
  std::vector<T> output(data.size());
  for (size_t i = 0; i < permutation.size(); ++i) {
    output[permutation[i]] = data[i];
  }
  return output;
}

// Inverts a permutation, i.e., output_permutation[input_permutation[i]] = i.
std::vector<int64> InversePermutation(
    absl::Span<const int64> input_permutation);

// Composes two permutations: output[i] = p1[p2[i]].
std::vector<int64> ComposePermutations(absl::Span<const int64> p1,
                                       absl::Span<const int64> p2);

// Returns true iff permutation == {0, 1, 2, ...}.
bool IsIdentityPermutation(absl::Span<const int64> permutation);

template <typename Container>
int64 PositionInContainer(const Container& container, int64 value) {
  return std::distance(container.begin(),
                       std::find(container.begin(), container.end(), value));
}

// Formats the container as a comma-separated string. StrAppend must support
// appending the elements of the container. Prefix is prepended and suffix is
// appended to the returned string.
template <typename Container>
string CommaSeparatedString(const Container& c, const char* prefix = "",
                            const char* suffix = "") {
  // Not using Join() since the implementation here is simple anyway and this
  // avoids copying the string to append prefix.
  string comma_separated = prefix;
  const char* separator = "";
  for (const auto& entry : c) {
    absl::StrAppend(&comma_separated, separator, entry);
    separator = ", ";
  }
  comma_separated += suffix;
  return comma_separated;
}

// Overload needed to allow the container to be an initializer list. The default
// type for T makes an empty initializer list work as well.
template <typename T = int>
string CommaSeparatedString(const std::initializer_list<T>& c,
                            const char* prefix = "", const char* suffix = "") {
  return CommaSeparatedString<std::initializer_list<T>>(c, prefix, suffix);
}

// Formats the container in the mathematical notation for a vector, e.g. (1, 3,
// 7). StrAppend must support appending the elements of c.
template <typename Container>
string VectorString(const Container& c) {
  return CommaSeparatedString(c, "(", ")");
}

// Overload needed to allow the container to be an initializer list. The default
// type for T makes an empty initializer list work as well.
template <typename T = int>
string VectorString(const std::initializer_list<T>& c) {
  return VectorString<std::initializer_list<T>>(c);
}

// Returns a PaddingConfig object that represents no padding for the given rank.
PaddingConfig MakeNoPaddingConfig(int64 rank);

// Returns a PaddingConfig object where 'padding' contains
// (low edge padding, high edge padding) pairs for each dimension.
PaddingConfig MakeEdgePaddingConfig(
    absl::Span<const std::pair<int64, int64>> padding);

// Returns true if the padding configuration has at least one dimension with
// non-zero interior padding.
bool HasInteriorPadding(const PaddingConfig& config);

// Imports the templated FloorOfRatio math function from the TensorFlow
// namespace, as it is very commonly used.
template <typename T>
T FloorOfRatio(T dividend, T divisor) {
  return tensorflow::MathUtil::FloorOfRatio<T>(dividend, divisor);
}

// Imports the templated CeilOfRatio math function from the TensorFlow
// namespace, as it is very commonly used.
template <typename T>
T CeilOfRatio(T dividend, T divisor) {
  return tensorflow::MathUtil::CeilOfRatio<T>(dividend, divisor);
}

template <typename T>
std::vector<T> ElementWiseCeilOfRatio(absl::Span<const T> dividends,
                                      absl::Span<const T> divisors) {
  std::vector<T> ceil_of_ratios;
  CHECK_EQ(dividends.size(), divisors.size());
  ceil_of_ratios.reserve(dividends.size());
  absl::c_transform(dividends, divisors, std::back_inserter(ceil_of_ratios),
                    [](const T dividend, const T divisor) {
                      return CeilOfRatio<T>(dividend, divisor);
                    });
  return ceil_of_ratios;
}

// Rounds the value up to a multiple of the divisor by first calling CeilOfRatio
// then multiplying by the divisor. For example: RoundUpToNearest(13, 8) => 16
template <typename T>
T RoundUpToNearest(T value, T divisor) {
  return CeilOfRatio(value, divisor) * divisor;
}

// Rounds the value down to a multiple of the divisor by first calling
// FloorOfRatio then multiplying by the divisor. For example:
// RoundDownToNearest(13, 8) => 8
template <typename T>
T RoundDownToNearest(T value, T divisor) {
  return FloorOfRatio(value, divisor) * divisor;
}

// Given a number of flops executed in an amount of time, produces a string that
// represents the throughput;
// e.g. HumanReadableNumFlops(1e9, 1e9) => 1.00GFLOP/s.
string HumanReadableNumFlops(double flops, double nanoseconds);

// Given a number of transcendental ops executed in an amount of time, produces
// a string that represents the throughput;
// e.g. HumanReadableNumTranscendentalOps(1e9, 1e9) => 1.00GTROP/s.
string HumanReadableNumTranscendentalOps(double trops, double nanoseconds);

// Split the text into multiple lines and log each line with the given
// severity, filename, and line number.
void LogLines(int sev, absl::string_view text, const char* fname, int lineno);

template <typename T>
inline bool IsPowerOfTwo(T x) {
  static_assert(!std::numeric_limits<T>::is_signed, "unsigned types only");
  return x != 0 && (x & (x - 1)) == 0;
}

// Returns a mask with "bits" number of least significant bits set.
inline uint32 LsbMaskU32(int bits) {
  CHECK_GE(bits, 0);
  return (1U << bits) - 1;
}

// Utility for performing a static_cast<> on a std::unique_ptr<>.
template <typename Derived, typename Base>
std::unique_ptr<Derived> unique_ptr_static_cast(std::unique_ptr<Base> ptr) {
  return std::unique_ptr<Derived>(static_cast<Derived*>(ptr.release()));
}

int64 Product(absl::Span<const int64> xs);

// Returns the start indices of consecutive non-overlapping subsequences of `a`
// and `b` with the same product, i.e. `(i, j)` so
// • a = {a[0 = i_0], ..., a[i_1 - 1], a[i_1], ... , a[i_2 - 1], ...}
// • b = {b[0 = j_0], ..., b[j_1 - 1], b[j_1], ... , b[j_2 - 1], ...}
// • ∀ k . 0 <= k < CommonFactors(a, b).size - 1 =>
//         a[i_k] × a[i_k + 1] × ... × a[i_(k+1) - 1] =
//         b[j_k] × b[j_k + 1] × ... × b[j_(k+1) - 1]
// where `CommonFactors(a, b)[CommonFactors(a, b).size - 1] = (a.size, b.size)`
//
// If the given shapes have non-zero size, returns the bounds of the shortest
// possible such subsequences; else, returns `{(0, 0), (a.size, b.size)}`.
std::vector<std::pair<int64, int64>> CommonFactors(absl::Span<const int64> a,
                                                   absl::Span<const int64> b);

// Removes illegal characters from filenames.
string SanitizeFileName(string file_name);

template <typename C, typename Value>
int64 FindIndex(const C& c, Value&& value) {
  auto it = absl::c_find(c, std::forward<Value>(value));
  return std::distance(c.begin(), it);
}

template <typename C, typename Value>
void InsertAt(C* c, int64 index, Value&& value) {
  c->insert(c->begin() + index, std::forward<Value>(value));
}

template <typename C>
void EraseAt(C* c, int64 index) {
  c->erase(c->begin() + index);
}

template <typename T>
std::vector<T> ArraySliceToVector(absl::Span<const T> slice) {
  return std::vector<T>(slice.begin(), slice.end());
}

template <typename T, size_t N>
std::vector<T> InlinedVectorToVector(
    const absl::InlinedVector<T, N>& inlined_vector) {
  return std::vector<T>(inlined_vector.begin(), inlined_vector.end());
}

// Returns true if `x` fits in 32-bits.
template <typename T>
bool IsInt32(T x) {
  // Following conversion rules: "the value is unchanged if it can be
  // represented in the destination type (and bit-field width); otherwise, the
  // value is implementation-defined."
  return static_cast<int32>(x) == x;
}

template <typename T>
Status EraseElementFromVector(std::vector<T>* container, const T& value) {
  // absl::c_find returns a const_iterator which does not seem to work on
  // gcc 4.8.4, and this breaks the ubuntu/xla_gpu build bot.
  auto it = std::find(container->begin(), container->end(), value);
  TF_RET_CHECK(it != container->end());
  container->erase(it);
  return Status::OK();
}
}  // namespace xla

#define XLA_LOG_LINES(SEV, STRING) \
  ::xla::LogLines(SEV, STRING, __FILE__, __LINE__)

#define XLA_VLOG_LINES(LEVEL, STRING)                                 \
  do {                                                                \
    if (VLOG_IS_ON(LEVEL)) XLA_LOG_LINES(::tensorflow::INFO, STRING); \
  } while (false);

// Utility macro that performs the equivalent of what one would expect
// LOG_LINES(FATAL, X) to do but can be used at the end of a function that
// returns a value without getting a compiler warning that no value is returned.
#define XLA_FATAL_LOG(X)                 \
  XLA_LOG_LINES(::tensorflow::ERROR, X); \
  LOG(FATAL) << "Aborting in " << __FUNCTION__ << " due to previous errors.";

#endif  // TENSORFLOW_COMPILER_XLA_UTIL_H_
