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

#include <set>
#include <unordered_map>
#include <utility>
#include <vector>

#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/service/hlo_parser.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/test.h"
#include "tensorflow/compiler/xla/test_helpers.h"
#include "tensorflow/compiler/xla/tests/hlo_test_base.h"
#include "tensorflow/compiler/xla/util.h"

namespace xla {
namespace {

Array<int64> MakeArray(absl::Span<const int64> dimensions,
                       absl::Span<const int64> contents) {
  Array<int64> a(dimensions);
  std::copy(contents.begin(), contents.end(), a.begin());
  return a;
}

class HloShardingTest : public HloTestBase {};

TEST_F(HloShardingTest, Replicate) {
  HloSharding sharding = HloSharding::Replicate();
  EXPECT_TRUE(sharding.IsReplicated());
  EXPECT_TRUE(sharding.IsTileMaximal());
  EXPECT_TRUE(sharding.UsesDevice(0));
  EXPECT_TRUE(sharding.UsesDevice(65535));

  HloSharding other = HloSharding::Replicate();
  EXPECT_EQ(other, sharding);

  EXPECT_IS_OK(sharding.Validate(ShapeUtil::MakeShape(U32, {4}),
                                 /*num_devices=*/2));
  EXPECT_FALSE(sharding.HasUniqueDevice());
}

TEST_F(HloShardingTest, DevicePlacement) {
  HloSharding sharding = HloSharding::AssignDevice(5);
  EXPECT_FALSE(sharding.IsReplicated());
  EXPECT_TRUE(sharding.IsTileMaximal());
  EXPECT_FALSE(sharding.UsesDevice(0));
  EXPECT_TRUE(sharding.UsesDevice(5));
  EXPECT_EQ(5, sharding.GetUniqueDevice());

  HloSharding other = HloSharding::Replicate();
  EXPECT_NE(other, sharding);

  EXPECT_IS_OK(sharding.Validate(ShapeUtil::MakeShape(U32, {4}),
                                 /*num_devices=*/6));
  EXPECT_IS_NOT_OK(
      sharding.Validate(ShapeUtil::MakeShape(U32, {4}), /*num_devices=*/5));

  ShapeTree<HloSharding> shape_tree =
      sharding.GetAsShapeTree(ShapeUtil::MakeShape(U32, {4}));
  EXPECT_EQ(shape_tree.element({}), sharding);
  EXPECT_TRUE(shape_tree.IsLeaf({}));
}

TEST_F(HloShardingTest, Tile) {
  {
    // Test should fail because of a duplicate tile assignment.
    HloSharding sharding = HloSharding::Tile(MakeArray({2, 2}, {0, 0, 2, 3}));
    EXPECT_IS_NOT_OK(sharding.Validate(ShapeUtil::MakeShape(F32, {4, 6}),
                                       /*num_devices=*/4));
  }

  {
    // Test should fail because of more devices used then `num_device`.
    HloSharding sharding = HloSharding::Tile(MakeArray({2, 2}, {0, 1, 2, 3}));
    EXPECT_IS_NOT_OK(sharding.Validate(ShapeUtil::MakeShape(U32, {4, 6}),
                                       /*num_devices=*/2));
  }

  {
    // Test should pass.
    Shape shape = ShapeUtil::MakeShape(U32, {4, 5});
    HloSharding sharding = HloSharding::Tile(MakeArray({2, 2}, {0, 3, 2, 1}));
    EXPECT_IS_OK(sharding.Validate(ShapeUtil::MakeShape(F32, {3, 5}),
                                   /*num_devices=*/5));

    EXPECT_EQ(0, sharding.DeviceForTileIndex({0, 0}));
    EXPECT_EQ(3, sharding.DeviceForTileIndex({0, 1}));
    EXPECT_EQ(2, sharding.DeviceForTileIndex({1, 0}));
    EXPECT_EQ(1, sharding.DeviceForTileIndex({1, 1}));

    EXPECT_EQ(sharding.TileOffsetForDevice(shape, 0),
              (std::vector<int64>{0, 0}));
    EXPECT_EQ(sharding.TileOffsetForDevice(shape, 3),
              (std::vector<int64>{0, 3}));
    EXPECT_EQ(sharding.TileOffsetForDevice(shape, 2),
              (std::vector<int64>{2, 0}));
    EXPECT_EQ(sharding.TileOffsetForDevice(shape, 1),
              (std::vector<int64>{2, 3}));

    EXPECT_FALSE(sharding.HasUniqueDevice());
  }
}

// Tests that empty tuple is supported.
TEST_F(HloShardingTest, EmptySingleTuple) {
  HloSharding sharding = HloSharding::SingleTuple(ShapeUtil::MakeTupleShape({}),
                                                  HloSharding::AssignDevice(0));
  EXPECT_TRUE(sharding.ExtractSingleSharding());
}

TEST_F(HloShardingTest, NestedTuple) {
  // nested_tuple_shape = (f32[], (f32[3]), f32[4, 6])
  Shape nested_tuple_shape = ShapeUtil::MakeTupleShape({
      ShapeUtil::MakeShape(F32, {}),
      ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {3})}),
      ShapeUtil::MakeShape(F32, {4, 6}),
  });

  HloSharding tiled_sharding = HloSharding::Tile(Array<int64>({{0, 1}}));
  OpSharding proto;
  proto.set_type(OpSharding::Type::OpSharding_Type_TUPLE);
  *proto.add_tuple_shardings() = HloSharding::Replicate().ToProto();
  *proto.add_tuple_shardings() = HloSharding::AssignDevice(0).ToProto();
  *proto.add_tuple_shardings() = tiled_sharding.ToProto();
  HloSharding tuple_sharding =
      HloSharding::FromProto(proto).ConsumeValueOrDie();

  ShapeTree<HloSharding> shape_tree =
      tuple_sharding.GetAsShapeTree(nested_tuple_shape);
  EXPECT_EQ(shape_tree.element({0}), HloSharding::Replicate());
  EXPECT_EQ(shape_tree.element({1, 0}), HloSharding::AssignDevice(0));
  EXPECT_EQ(shape_tree.element({2}), tiled_sharding);

  EXPECT_IS_OK(tuple_sharding.Validate(nested_tuple_shape, /*num_devices=*/5));
  // Test should fail because tuple element count does not match.
  EXPECT_IS_NOT_OK(tuple_sharding.Validate(ShapeUtil::MakeTupleShape({}),
                                           /*num_devices=*/5));
  // Test should fail because the input type is not a tuple.
  EXPECT_IS_NOT_OK(tuple_sharding.Validate(ShapeUtil::MakeShape(F32, {}),
                                           /*num_devices=*/5));
}

TEST_F(HloShardingTest, Hash) {
  auto hash_compare_equal = [](const HloSharding& a, const HloSharding& b) {
    if (a.Hash() != b.Hash()) {
      return false;
    }
    return a == b;
  };

  {
    HloSharding sharding1 = HloSharding::Replicate();
    HloSharding sharding2 = HloSharding::Replicate();
    EXPECT_TRUE(hash_compare_equal(sharding1, sharding2));
  }

  {
    HloSharding sharding1 = HloSharding::AssignDevice(1);
    HloSharding sharding2 = HloSharding::AssignDevice(1);
    EXPECT_TRUE(hash_compare_equal(sharding1, sharding2));
  }

  {
    HloSharding sharding1 = HloSharding::AssignDevice(1);
    HloSharding sharding2 = HloSharding::AssignDevice(2);
    EXPECT_FALSE(hash_compare_equal(sharding1, sharding2));
  }

  {
    HloSharding sharding1 = HloSharding::Tile(MakeArray({2, 2}, {0, 3, 2, 1}));
    HloSharding sharding2 = HloSharding::Tile(MakeArray({2, 2}, {0, 3, 2, 1}));
    EXPECT_TRUE(hash_compare_equal(sharding1, sharding2));
  }

  HloSharding default_sharding = HloSharding::Replicate();
  {
    ShapeTree<HloSharding> shape_tree(ShapeUtil::MakeTupleShape({}),
                                      default_sharding);
    HloSharding sharding1 = HloSharding::Replicate();
    HloSharding sharding2 = HloSharding::Tuple(shape_tree);
    EXPECT_FALSE(hash_compare_equal(sharding1, sharding2));
  }

  {
    ShapeTree<HloSharding> shape_tree(ShapeUtil::MakeTupleShape({}),
                                      default_sharding);
    HloSharding sharding1 = HloSharding::Tuple(shape_tree);
    HloSharding sharding2 = HloSharding::Tuple(shape_tree);
    EXPECT_TRUE(hash_compare_equal(sharding1, sharding2));
  }

  {
    ShapeTree<HloSharding> shape_tree1(
        ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {4})}),
        default_sharding);
    *shape_tree1.mutable_element({0}) = HloSharding::Replicate();
    ShapeTree<HloSharding> shape_tree2(
        ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {4})}),
        default_sharding);
    *shape_tree2.mutable_element({0}) = HloSharding::AssignDevice(0);
    HloSharding sharding1 = HloSharding::Tuple(shape_tree1);
    HloSharding sharding2 = HloSharding::Tuple(shape_tree2);
    EXPECT_FALSE(hash_compare_equal(sharding1, sharding2));
  }

  {
    ShapeTree<HloSharding> shape_tree1(
        ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {4})}),
        default_sharding);
    *shape_tree1.mutable_element({0}) = HloSharding::AssignDevice(0);
    ShapeTree<HloSharding> shape_tree2(
        ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {4})}),
        default_sharding);
    *shape_tree2.mutable_element({0}) = HloSharding::AssignDevice(0);
    HloSharding sharding1 = HloSharding::Tuple(shape_tree1);
    HloSharding sharding2 = HloSharding::Tuple(shape_tree2);
    EXPECT_TRUE(hash_compare_equal(sharding1, sharding2));
  }
}

TEST_F(HloShardingTest, ToStringReplicatedTest) {
  HloSharding sharding = HloSharding::Replicate();
  EXPECT_EQ(sharding.ToString(), "{replicated}");
}

TEST_F(HloShardingTest, ToStringAssignDeviceTest) {
  HloSharding sharding = HloSharding::AssignDevice(7);
  EXPECT_EQ(sharding.ToString(), "{maximal device=7}");
}

TEST_F(HloShardingTest, ToStringTiledTest) {
  HloSharding sharding =
      HloSharding::Tile(Array3D<int64>({{{2, 3}}, {{5, 7}}}));
  EXPECT_EQ(sharding.ToString(), "{devices=[2,1,2]2,3,5,7}");
}

TEST_F(HloShardingTest, ToStringTupleTest) {
  HloSharding sharding = HloSharding::Tuple(
      ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {3, 5}),
                                 ShapeUtil::MakeShape(U32, {7, 25}),
                                 ShapeUtil::MakeShape(S32, {9, 11})}),
      {HloSharding::Replicate(), HloSharding::Tile(Array2D<int64>({{3, 5}})),
       HloSharding::AssignDevice(3)});
  EXPECT_EQ(sharding.ToString(),
            "{{replicated}, {devices=[1,2]3,5}, {maximal device=3}}");
}

TEST_F(HloShardingTest, OstreamTest) {
  HloSharding sharding =
      HloSharding::Tile(Array4D<int64>({{{{0, 1}, {2, 3}}}}));
  std::ostringstream oss;
  oss << sharding;
  EXPECT_EQ(oss.str(), "{devices=[1,1,2,2]0,1,2,3}");
}

TEST_F(HloShardingTest, ParseHloString) {
  auto check = [](const HloSharding& sharding) {
    TF_ASSERT_OK_AND_ASSIGN(auto parsed_sharding,
                            ParseSharding(sharding.ToString()));
    EXPECT_EQ(sharding, parsed_sharding);
  };
  check(HloSharding::Replicate());
  check(HloSharding::AssignDevice(2));
  check(HloSharding::Tile(Array4D<int64>({{{{0}, {1}}}})));
  // Empty tuple. One sharding is required for empty tuples, as we need to be
  // able to assign sharding to them, even though they have no leaves.
  check(HloSharding::Tuple(ShapeUtil::MakeTupleShape({}),
                           {HloSharding::Replicate()}));
  {
    // Non-nested tuple.
    auto tuple_shape =
        ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {3, 1, 5, 7}),
                                   ShapeUtil::MakeShape(F32, {3, 5, 7}),
                                   ShapeUtil::MakeShape(F32, {3, 7})});
    check(HloSharding::Tuple(
        tuple_shape, {HloSharding::Tile(Array4D<int64>({{{{0}, {1}}}})),
                      HloSharding::Replicate(), HloSharding::AssignDevice(1)}));
  }
  {
    // Nested tuple.
    auto tuple_shape = ShapeUtil::MakeTupleShape(
        {ShapeUtil::MakeShape(F32, {3, 1, 5, 7}),
         ShapeUtil::MakeTupleShape({ShapeUtil::MakeShape(F32, {3, 5, 7}),
                                    ShapeUtil::MakeShape(F32, {3, 7})})});
    std::vector<HloSharding> leaf_shardings = {
        HloSharding::Tile(Array4D<int64>({{{{0}, {1}}}})),
        HloSharding::Replicate(), HloSharding::AssignDevice(1)};
    ShapeTree<HloSharding> sharding_tree(tuple_shape, HloSharding::Replicate());
    // Assign leaf_shardings to sharding_tree leaves.
    auto it = leaf_shardings.begin();
    for (auto& index_to_sharding : sharding_tree.leaves()) {
      index_to_sharding.second = *it++;
    }
    check(HloSharding::Tuple(sharding_tree));
  }
}

}  // namespace
}  // namespace xla
