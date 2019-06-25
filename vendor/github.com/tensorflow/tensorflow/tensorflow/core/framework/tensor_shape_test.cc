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

#include "tensorflow/core/framework/tensor_shape.h"

#include "tensorflow/core/framework/tensor_shape.pb.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/lib/random/simple_philox.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/test.h"
#include "tensorflow/core/platform/test_benchmark.h"

namespace tensorflow {
class TensorShapeTestHelper {
 public:
  static void set_data_type(TensorShape* s, DataType t) { s->set_data_type(t); }
  static uint8 data_type(const TensorShape* s) { return s->data_type(); }
};

namespace {

TEST(TensorShapeTest, Default) {
  // The default TensorShape constructor constructs a shape of 0-dim
  // and 1-element.
  TensorShape s;
  EXPECT_EQ(s.dims(), 0);
  EXPECT_EQ(s.num_elements(), 1);
}

TEST(TensorShapeTest, set_dim) {
  TensorShape s({10, 5});

  s.set_dim(0, 20);
  ASSERT_EQ(2, s.dims());
  EXPECT_EQ(20, s.dim_size(0));
  EXPECT_EQ(100, s.num_elements());

  s.set_dim(1, 2);
  ASSERT_EQ(2, s.dims());
  EXPECT_EQ(2, s.dim_size(1));
  EXPECT_EQ(40, s.num_elements());
}

TEST(TensorShapeTest, RemoveDim) {
  TensorShape s({10, 5});
  s.RemoveDim(0);
  EXPECT_EQ(5, s.num_elements());
  ASSERT_EQ(1, s.dims());
}

TEST(TensorShapeTest, RemoveAndAddDim) {
  TensorShape s({10, 5, 20});
  s.RemoveDim(1);
  s.AddDim(100);

  EXPECT_EQ(20000, s.num_elements());
  ASSERT_EQ(3, s.dims());
}

TEST(TensorShapeTest, RemoveLastDims) {
  TensorShape s({2, 3, 5, 7});
  s.RemoveLastDims(1);

  ASSERT_EQ(3, s.dims());
  EXPECT_EQ(30, s.num_elements());

  s.RemoveLastDims(2);
  ASSERT_EQ(1, s.dims());
  EXPECT_EQ(2, s.dim_size(0));
}

TEST(TensorShapeTest, RemoveDimRange) {
  TensorShape s0({2, 3, 5, 7});
  // Empty interval => noop.
  for (int i = -4; i <= 4; ++i) {
    s0.RemoveDimRange(i, i);
    ASSERT_EQ(4, s0.dims());
    ASSERT_EQ(210, s0.num_elements());
  }

  // Positive begin and end.
  s0.RemoveDimRange(3, 1);  // Empty interval.
  ASSERT_EQ(4, s0.dims());
  ASSERT_EQ(210, s0.num_elements());
  s0.RemoveDimRange(0, 3);
  ASSERT_EQ(1, s0.dims());
  EXPECT_EQ(7, s0.dim_size(0));
  TensorShape s1({2, 3, 5, 7});
  s1.RemoveDimRange(2, 3);
  ASSERT_EQ(3, s1.dims());
  ASSERT_EQ(42, s1.num_elements());

  // Negative begin or end.
  TensorShape s2({2, 3, 5, 7});
  s2.RemoveDimRange(-2, -3);  // Empty interval.
  ASSERT_EQ(4, s2.dims());
  ASSERT_EQ(210, s2.num_elements());
  s2.RemoveDimRange(0, -2);
  ASSERT_EQ(1, s2.dims());
  ASSERT_EQ(7, s2.dim_size(0));
  TensorShape s3({2, 3, 5, 7});
  s3.RemoveDimRange(-3, -2);
  ASSERT_EQ(3, s3.dims());
  ASSERT_EQ(42, s3.num_elements());
}

TEST(TensorShapeTest, InvalidShapeProto) {
  TensorShapeProto proto;
  EXPECT_TRUE(TensorShape::IsValid(proto));

  proto.add_dim()->set_size(357);
  proto.add_dim()->set_size(982);
  EXPECT_TRUE(TensorShape::IsValid(proto));

  proto.Clear();
  proto.add_dim()->set_size(-357);
  proto.add_dim()->set_size(-982);
  EXPECT_FALSE(TensorShape::IsValid(proto));

  proto.Clear();
  proto.add_dim()->set_size(1LL << 35);
  proto.add_dim()->set_size((1LL << 35) + 1);
  EXPECT_FALSE(TensorShape::IsValid(proto));
}

TEST(TensorShapeTest, TooManyDimsProto) {
  TensorShapeProto proto;
  // Deliberate redundancy to ensure that both paths work.
  EXPECT_TRUE(TensorShape::IsValid(proto));
  TF_EXPECT_OK(TensorShape::IsValidShape(proto));
  for (int i = 0; i < TensorShape::MaxDimensions(); i++) {
    proto.add_dim()->set_size(1);
  }
  EXPECT_TRUE(TensorShape::IsValid(proto));
  TF_EXPECT_OK(TensorShape::IsValidShape(proto));
  proto.add_dim()->set_size(1);
  EXPECT_FALSE(TensorShape::IsValid(proto));
  EXPECT_FALSE(TensorShape::IsValidShape(proto).ok());
}

TEST(TensorShapeTest, SetDimForEmptyTensor) {
  TensorShape s({10, 5, 20});
  EXPECT_EQ(1000, s.num_elements());
  s.set_dim(1, 0);
  EXPECT_EQ(0, s.num_elements());
  s.set_dim(1, 7);
  EXPECT_EQ(1400, s.num_elements());
}

TEST(TensorShapeTest, AppendShape64BitIndices) {
  TensorShape s({10, 2147483648});

  EXPECT_EQ(10, s.dim_size(0));
  EXPECT_EQ(2147483648, s.dim_size(1));

  TensorShape s2;
  s2.AppendShape(s);
  EXPECT_EQ(10, s2.dim_size(0));
  EXPECT_EQ(2147483648, s2.dim_size(1));
}

TEST(TensorShapeTest, DataType) {
  TensorShape s({});
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s), DT_INVALID);
  TensorShapeTestHelper::set_data_type(&s, DT_INT32);
  s.AddDim(1);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s), DT_INT32);
  s.AddDim(100000);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s), DT_INT32);
  TensorShapeTestHelper::set_data_type(&s, DT_UINT16_REF);
  s.AddDim(2);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s), DT_UINT16_REF);
  s.AddDim(4);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s), DT_UINT16_REF);
  s.AddDim(3);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s), DT_UINT16_REF);

  TensorShape s2 = s;
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s2), DT_UINT16_REF);
  s2.RemoveDim(2);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s2), DT_UINT16_REF);
  TensorShapeTestHelper::set_data_type(&s2, DT_FLOAT);
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s2), DT_FLOAT);
  s2.Clear();
  EXPECT_EQ(TensorShapeTestHelper::data_type(&s2), DT_INVALID);
}

TEST(TensorShapeTest, ostream) {
  TensorShape s({10, 5, 4});
  std::stringstream ss;
  ss << s;
  EXPECT_EQ(ss.str(), "[10,5,4]");
}

// -----------------------------------------------------------------------
// An old implementation of TensorShape using a different representation,
// preserved here in the unittest to allow us to have a randomized unittest
// that makes sure the behavior of TensorShape and TensorShapeOld are
// the same.
class TensorShapeIterOld;  // Declared below

/// Manages the dimensions of a Tensor and their sizes.
class TensorShapeOld {
 public:
  /// \brief Construct a `TensorShape` from the provided sizes.
  /// REQUIRES: `dim_sizes[i] >= 0`
  explicit TensorShapeOld(gtl::ArraySlice<int64> dim_sizes);
  TensorShapeOld(std::initializer_list<int64> dim_sizes)
      : TensorShapeOld(gtl::ArraySlice<int64>(dim_sizes)) {}

  /// REQUIRES: `IsValid(proto)`
  explicit TensorShapeOld(const TensorShapeProto& proto);

  /// Create a tensor shape with no dimensions and one element, which you can
  /// then call `AddDim()` on.
  TensorShapeOld();

  /// Returns `true` iff `proto` is a valid tensor shape.
  static bool IsValid(const TensorShapeProto& proto);

  /// Returns `OK` iff `proto` is a valid tensor shape, and a descriptive error
  /// status otherwise.
  static Status IsValidShape(const TensorShapeProto& proto);

  /// Clear a tensor shape
  void Clear();

  /// \brief Add a dimension to the end ("inner-most").
  /// REQUIRES: `size >= 0`
  void AddDim(int64 size);

  /// Appends all the dimensions from `shape`.
  void AppendShape(const TensorShapeOld& shape);

  /// \brief Insert a dimension somewhere in the `TensorShape`.
  /// REQUIRES: `0 <= d <= dims()`
  /// REQUIRES: `size >= 0`
  void InsertDim(int d, int64 size);

  /// \brief Modifies the size of the dimension `d` to be `size`
  /// REQUIRES: `0 <= d < dims()`
  /// REQUIRES: `size >= 0`
  void set_dim(int d, int64 size);

  /// \brief Removes dimension `d` from the `TensorShape`.
  /// REQUIRES: `0 <= d < dims()`
  void RemoveDim(int d);

  /// Return the number of dimensions in the tensor.
  int dims() const { return dim_sizes_.size(); }

  /// \brief Returns the number of elements in dimension `d`.
  /// REQUIRES: `0 <= d < dims()`
  // TODO(touts): Rename to `dimension()` to match
  // `Eigen::Tensor::dimension()`?
  int64 dim_size(int d) const {
    DCHECK_GE(d, 0);
    DCHECK_LT(d, dims());
    return dim_sizes_[d];
  }

  /// Returns sizes of all dimensions.
  gtl::ArraySlice<int64> dim_sizes() const { return dim_sizes_; }

  /// \brief Returns the number of elements in the tensor.
  ///
  /// We use `int64` and not `size_t` to be compatible with `Eigen::Tensor`
  /// which uses `ptrdiff_t`.
  int64 num_elements() const { return num_elements_; }

  /// Returns true if `*this` and `b` have the same sizes. Ignores
  /// dimension names.
  bool IsSameSize(const TensorShapeOld& b) const;
  bool operator==(const TensorShapeOld& b) const { return IsSameSize(b); }

  /// Fill `*proto` from `*this`.
  void AsProto(TensorShapeProto* proto) const;

  /// Fill `*dsizes` from `*this`.
  template <int NDIMS>
  Eigen::DSizes<Eigen::DenseIndex, NDIMS> AsEigenDSizes() const;

  /// Same as `AsEigenDSizes()` but allows for `NDIMS > dims()` -- in
  /// which case we pad the rest of the sizes with 1.
  template <int NDIMS>
  Eigen::DSizes<Eigen::DenseIndex, NDIMS> AsEigenDSizesWithPadding() const;

  /// For iterating through the dimensions.
  TensorShapeIterOld begin() const;
  TensorShapeIterOld end() const;

  /// For error messages.
  string DebugString() const;

  /// Same as `TensorShape(proto).DebugString()` but doesn't crash for
  /// invalid protos.
  static string DebugString(const TensorShapeProto& proto);

 private:
  // Recalculates the dimensions of this tensor after they are modified.
  void recompute_dims();

  // TODO(josh11b): Maybe use something from the Eigen Tensor library
  // for the sizes.
  gtl::InlinedVector<int64, 4> dim_sizes_;

  // total number of elements (avoids recomputing it each time).
  int64 num_elements_;
};

struct TensorShapeDimOld {
  explicit TensorShapeDimOld(int64 s) : size(s) {}
  int64 size;
};

class TensorShapeIterOld {
 public:
  TensorShapeIterOld(const TensorShapeOld* shape, int d)
      : shape_(shape), d_(d) {}
  bool operator==(const TensorShapeIterOld& rhs) {
    DCHECK(shape_ == rhs.shape_);
    return d_ == rhs.d_;
  }
  bool operator!=(const TensorShapeIterOld& rhs) {
    DCHECK(shape_ == rhs.shape_);
    return d_ != rhs.d_;
  }
  void operator++() { ++d_; }
  TensorShapeDimOld operator*() {
    return TensorShapeDimOld(shape_->dim_size(d_));
  }

 private:
  const TensorShapeOld* shape_;
  int d_;
};

// An upper limit of the total number of elements in a tensor.
static const int64 kMaxElements = (1LL << 40);

bool TensorShapeOld::IsValid(const TensorShapeProto& proto) {
  int64 num_elements = 1;
  for (const auto& d : proto.dim()) {
    if (d.size() < 0) return false;
    num_elements *= d.size();
    if (num_elements > kMaxElements) return false;
  }
  return true;
}

Status TensorShapeOld::IsValidShape(const TensorShapeProto& proto) {
  int64 num_elements = 1;
  for (const auto& d : proto.dim()) {
    if (d.size() < 0) {
      return errors::InvalidArgument("Shape ", DebugString(proto),
                                     " has negative dimensions; ",
                                     "perhaps an un-fed placeholder?");
    }
    num_elements *= d.size();
    if (num_elements > kMaxElements) {
      return errors::InvalidArgument("Shape ", DebugString(proto),
                                     " is too large (more than ", kMaxElements,
                                     " entries)");
    }
  }
  return Status::OK();
}

TensorShapeOld::TensorShapeOld(const TensorShapeProto& proto) {
  dim_sizes_.reserve(proto.dim_size());
  num_elements_ = 1;
  for (const auto& d : proto.dim()) {
    AddDim(d.size());
  }
}

TensorShapeOld::TensorShapeOld(gtl::ArraySlice<int64> dim_sizes) {
  dim_sizes_.reserve(dim_sizes.size());
  num_elements_ = 1;
  for (auto s : dim_sizes) {
    AddDim(s);
  }
}

TensorShapeOld::TensorShapeOld() : num_elements_(1) {}

void TensorShapeOld::Clear() {
  dim_sizes_.clear();
  num_elements_ = 1;
}

void TensorShapeOld::AddDim(int64 size) {
  CHECK_GE(size, 0);
  dim_sizes_.push_back(size);
  num_elements_ *= size;
  CHECK_LE(0, num_elements_);
  CHECK_LE(num_elements_, kMaxElements);
}

void TensorShapeOld::AppendShape(const TensorShapeOld& shape) {
  for (auto d : shape) AddDim(d.size);
}

void TensorShapeOld::InsertDim(int d, int64 size) {
  CHECK_GE(d, 0);
  CHECK_LE(d, dims());
  CHECK_GE(size, 0);
  dim_sizes_.insert(dim_sizes_.begin() + d, size);
  num_elements_ *= size;
  CHECK_LE(0, num_elements_);
  CHECK_LE(num_elements_, kMaxElements);
}

void TensorShapeOld::set_dim(int d, int64 size) {
  CHECK_GE(d, 0);
  CHECK_LT(d, dims());
  CHECK_GE(size, 0);

  // Update the number of elements. num_elements_ is int64.
  dim_sizes_[d] = size;
  recompute_dims();
}

void TensorShapeOld::RemoveDim(int d) {
  CHECK_GE(d, 0);
  CHECK_LT(d, dims());

  // Update the number of elements and remove the dimension from the
  // sizes.
  dim_sizes_.erase(dim_sizes_.begin() + d);
  recompute_dims();
}

void TensorShapeOld::recompute_dims() {
  num_elements_ = 1;
  for (auto s : dim_sizes_) {
    num_elements_ *= s;
    CHECK_LE(0, num_elements_);
    CHECK_LE(num_elements_, kMaxElements);
  }
}

bool TensorShapeOld::IsSameSize(const TensorShapeOld& b) const {
  if (b.dims() != dims()) return false;
  for (int d = 0; d < dims(); d++) {
    if (dim_size(d) != b.dim_size(d)) return false;
  }
  return true;
}

void TensorShapeOld::AsProto(TensorShapeProto* proto) const {
  proto->Clear();
  for (size_t d = 0; d < dim_sizes_.size(); ++d) {
    auto* dim = proto->add_dim();
    dim->set_size(dim_sizes_[d]);
  }
}

TensorShapeIterOld TensorShapeOld::begin() const {
  return TensorShapeIterOld(this, 0);
}

TensorShapeIterOld TensorShapeOld::end() const {
  return TensorShapeIterOld(this, dims());
}

string TensorShapeOld::DebugString() const {
  return strings::StrCat(
      "[", str_util::Join(gtl::ArraySlice<int64>(dim_sizes_), ","), "]");
}

string TensorShapeOld::DebugString(const TensorShapeProto& proto) {
  string s = "[";
  bool first = true;
  for (const auto& d : proto.dim()) {
    strings::StrAppend(&s, first ? "" : ",", d.size());
    first = false;
  }
  strings::StrAppend(&s, "]");
  return s;
}
// End of old implementation
// ------------------------------------------------------------------------

static int64 SkewedSize(random::SimplePhilox* gen, int64 current_elements) {
  int64 result = 0;
  do {
    if (current_elements < 100) {
      result = gen->Uniform(100000);
    } else {
      result = gen->Uniform(2);
    }
  } while ((result * current_elements >= 1LL << 34) ||
           (result * current_elements < 0));
  return result;
}

TEST(TensorShapeTest, Randomized) {
  // We do a randomized test to verify that the behavior of the
  // TensorShape implementation (which changes representations depending
  // on the values) is identical to our older, more straightforward (but
  // more memory hungry) implementation (TensorShapeOld).
  random::PhiloxRandom philox(7, 7);
  random::SimplePhilox gen(&philox);
  TensorShape s;
  TensorShapeOld sold;
  TensorShapeProto sp;
  TensorShapeProto spold;
  LOG(INFO) << "Sizes: " << sizeof(TensorShape) << " vs "
            << sizeof(TensorShapeOld);
  for (int i = 0; i < 100000; i++) {
    s.AsProto(&sp);
    sold.AsProto(&spold);
    EXPECT_EQ(sp.DebugString(), spold.DebugString());
    if ((i % 1000) == 0) {
      fprintf(stderr, "ITERATION %d: %s\n", i, sp.DebugString().c_str());
    }
    EXPECT_EQ(s.num_elements(), sold.num_elements());

    // Test moves.
    TensorShape copy = s;
    TensorShape moved(std::move(copy));
    EXPECT_EQ(s, moved);
    copy = s;
    moved = std::move(copy);
    EXPECT_EQ(s, moved);

    int64 ne = sold.num_elements();
    int r = gen.Uniform(100);
    if (r < 10) {
      int64 sz = SkewedSize(&gen, sold.num_elements());
      s.AddDim(sz);
      sold.AddDim(sz);
    } else if (r < 15) {
      s.Clear();
      sold.Clear();
    } else if (r < 35 && s.dims() > 0 && ne > 0 && ne < 100000000) {
      int dim = gen.Uniform(s.dims());
      s.RemoveDim(dim);
      sold.RemoveDim(dim);
    } else if (r < 50 && ne > 0 && ne < 100000000) {
      int dim = gen.Uniform(s.dims() + 1);
      int64 sz = SkewedSize(&gen, sold.num_elements());
      s.InsertDim(dim, sz);
      sold.InsertDim(dim, sz);
    } else {
      std::vector<int64> sizes;
      const int N = (gen.Uniform(4) == 0) ? gen.Uniform(10) : gen.Uniform(3);
      int64 num_elements = 1;
      for (int i = 0; i < N; i++) {
        int64 sz = SkewedSize(&gen, num_elements);
        sizes.push_back(sz);
        num_elements *= std::max<int64>(1, sz);
      }

      s = TensorShape(sizes);
      sold = TensorShapeOld(sizes);
    }
  }
}

TEST(TensorShapeTest, Large) {
  // We used to cap shapes at 2**40 elements.  Ensure the
  // bound is now higher.
  int64 one = 1;
  int64 max = std::numeric_limits<int64>::max();
  EXPECT_EQ(TensorShape({max}).num_elements(), max);
  EXPECT_EQ(TensorShape({1, max}).num_elements(), max);
  EXPECT_EQ(TensorShape({max, 1}).num_elements(), max);
  EXPECT_EQ(TensorShape({one << 62}).num_elements(), one << 62);
  EXPECT_EQ(TensorShape({one << 20, one << 41}).num_elements(), one << 61);
  EXPECT_EQ(TensorShape({1000, 1000, 1000, 1000, 1000, 1000}).num_elements(),
            1e18);
}

TEST(TensorShapeTest, Overflow) {
  int64 one = 1;
  std::vector<std::vector<int64>> overflows = {
      {1 << 30, 1 << 30, 1 << 30},
      {1 << 5, (one << 60) + 1},
  };
  for (const auto& overflow : overflows) {
    TensorShapeProto proto;
    for (auto dim : overflow) {
      proto.add_dim()->set_size(dim);
    }
    EXPECT_EQ(tensorflow::error::INVALID_ARGUMENT,
              TensorShape::IsValidShape(proto).code());
    TensorShape shape;
    EXPECT_EQ(tensorflow::error::INVALID_ARGUMENT,
              TensorShapeUtils::MakeShape(overflow, &shape).code());
  }
}

TEST(TensorShapeTest, UnknownRank) {
  // NOTE(irving): Unfortunately, for historical reasons we have to allow an
  // TensorShapeProto with unknown_rank() set to be parsed as a TensorShape.
  // Would be nice to tighten this, but it's tricky given backwards
  // compatibility requirements.
  TensorShapeProto proto;
  proto.set_unknown_rank(true);
  EXPECT_TRUE(TensorShape::IsValid(proto));
  TF_EXPECT_OK(TensorShape::IsValidShape(proto));
  EXPECT_EQ(TensorShape(), TensorShape(proto));

  proto.add_dim()->set_size(7);
  EXPECT_TRUE(TensorShape::IsValid(proto));
  TF_EXPECT_OK(TensorShape::IsValidShape(proto));
  EXPECT_EQ(TensorShape({7}), TensorShape(proto));
}

TEST(TensorShapeUtilsTest, StartsWith) {
  EXPECT_TRUE(TensorShapeUtils::StartsWith(TensorShape({}), TensorShape({})));
  EXPECT_TRUE(
      TensorShapeUtils::StartsWith(TensorShape({2, 3}), TensorShape({})));
  EXPECT_TRUE(
      TensorShapeUtils::StartsWith(TensorShape({2, 3}), TensorShape({2})));
  EXPECT_TRUE(
      TensorShapeUtils::StartsWith(TensorShape({2, 3}), TensorShape({2, 3})));
  EXPECT_TRUE(TensorShapeUtils::StartsWith(TensorShape({2, 3, 4}),
                                           TensorShape({2, 3})));
  EXPECT_FALSE(
      TensorShapeUtils::StartsWith(TensorShape({2, 3}), TensorShape({3})));
  EXPECT_FALSE(
      TensorShapeUtils::StartsWith(TensorShape({2, 3}), TensorShape({2, 4})));
  EXPECT_FALSE(TensorShapeUtils::StartsWith(TensorShape({2, 3}),
                                            TensorShape({2, 3, 4})));
  EXPECT_FALSE(TensorShapeUtils::StartsWith(TensorShape({2, 3, 4}),
                                            TensorShape({3, 4})));
}

TEST(TensorShapeUtilsTest, EndsWith) {
  EXPECT_TRUE(TensorShapeUtils::EndsWith(TensorShape({}), TensorShape({})));
  EXPECT_TRUE(TensorShapeUtils::EndsWith(TensorShape({2, 3}), TensorShape({})));
  EXPECT_TRUE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3}), TensorShape({3})));
  EXPECT_TRUE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3}), TensorShape({2, 3})));
  EXPECT_TRUE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3, 4}), TensorShape({3, 4})));
  EXPECT_FALSE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3}), TensorShape({2})));
  EXPECT_FALSE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3}), TensorShape({2, 4})));
  EXPECT_FALSE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3}), TensorShape({2, 3, 4})));
  EXPECT_FALSE(
      TensorShapeUtils::EndsWith(TensorShape({2, 3, 4}), TensorShape({2, 3})));
}

// A few different test cases for tensor sizes for benchmarks
static std::vector<int64> MakeSizes(int arg) {
  std::vector<int64> sizes;
  switch (arg) {
    case 0:
      sizes = {100};
      break;
    case 1:
      sizes = {100, 1000};
      break;
    case 2:
      sizes = {100, 1000000};
      break;
    case 3:
      sizes = {100, 256, 192, 3};
      break;
    case 4:
      sizes = {1, 2, 1ll << 34, 1, 1, 1};
      break;
  }
  return sizes;
}

static void BM_TensorShape_Assign(int iters, int arg) {
  TensorShape s(MakeSizes(arg));
  while (--iters > 0) {
    TensorShape s2 = s;
  }
}
BENCHMARK(BM_TensorShape_Assign)->Arg(0)->Arg(1)->Arg(2)->Arg(3)->Arg(4);

}  // namespace
}  // namespace tensorflow
