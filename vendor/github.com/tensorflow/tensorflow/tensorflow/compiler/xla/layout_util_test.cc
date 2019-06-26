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

#include "tensorflow/compiler/xla/layout_util.h"

#include <sstream>

#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/compiler/xla/test.h"
#include "tensorflow/compiler/xla/test_helpers.h"

namespace xla {
namespace {

class LayoutUtilTest : public ::testing::Test {
 protected:
  Shape MakeShapeWithLayout(PrimitiveType element_type,
                            absl::Span<const int64> dimensions,
                            absl::Span<const int64> minor_to_major) {
    Shape shape = ShapeUtil::MakeShape(element_type, dimensions);
    *shape.mutable_layout() = LayoutUtil::MakeLayout(minor_to_major);
    return shape;
  }

  Shape MakeShapeWithSparseLayout(PrimitiveType element_type,
                                  absl::Span<const int64> dimensions,
                                  int64 max_sparse_elements) {
    Shape shape = ShapeUtil::MakeShape(element_type, dimensions);
    *shape.mutable_layout() = LayoutUtil::MakeSparseLayout(max_sparse_elements);
    return shape;
  }
};

TEST_F(LayoutUtilTest, TupleLayoutComparison) {
  Shape shape =
      ShapeUtil::MakeTupleShape({MakeShapeWithLayout(F32, {2, 3}, {0, 1})});
  Shape other_shape =
      ShapeUtil::MakeTupleShape({MakeShapeWithLayout(F32, {2, 2}, {0, 1})});

  Shape tuple0 = ShapeUtil::MakeTupleShape({});
  Shape tuple1 = ShapeUtil::MakeTupleShape({shape});
  Shape tuple2 = ShapeUtil::MakeTupleShape({shape, shape});

  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(tuple0, tuple0));
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(tuple0, tuple1));
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(tuple0, tuple2));
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(tuple1, tuple0));
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(tuple2, tuple0));

  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(tuple1, tuple1));
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(tuple1, tuple2));
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(tuple2, tuple1));

  Shape other_tuple2 = ShapeUtil::MakeTupleShape({shape, other_shape});
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(tuple2, tuple2));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(tuple2, other_tuple2));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(other_tuple2, tuple2));
}

TEST_F(LayoutUtilTest, CopyLayoutArray) {
  Shape src = MakeShapeWithLayout(F32, {2, 3}, {0, 1});
  Shape dst = MakeShapeWithLayout(F32, {2, 3}, {1, 0});

  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));

  // Should work if destination has no layout.
  dst.clear_layout();
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));

  // If source is cleared, then destination should be cleared.
  src.clear_layout();
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_TRUE(dst.has_layout());
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_FALSE(dst.has_layout());
}

TEST_F(LayoutUtilTest, CopyLayoutSparse) {
  Shape src = MakeShapeWithSparseLayout(F32, {2, 3}, 2);
  Shape dst = MakeShapeWithLayout(F32, {2, 3}, {1, 0});

  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));

  // Should work if destination has no layout.
  dst.clear_layout();
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));

  // If source is cleared, then destination should be cleared.
  src.clear_layout();
  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_TRUE(dst.has_layout());
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_FALSE(dst.has_layout());
}

TEST_F(LayoutUtilTest, CopyLayoutTuple) {
  Shape src = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {0, 1}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {0, 2, 1})})});
  Shape dst = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {1, 0}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {1, 2, 0})})});

  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, CopyLayoutTupleSparse) {
  Shape src = ShapeUtil::MakeTupleShape(
      {MakeShapeWithSparseLayout(F32, {2, 3}, 4),
       MakeShapeWithSparseLayout(F32, {42, 123}, 4),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithSparseLayout(F32, {1, 2, 3}, 6)})});
  Shape dst = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {1, 0}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {1, 2, 0})})});

  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, CopyLayoutNotCompatibleSameRank) {
  Shape src = MakeShapeWithLayout(F32, {123, 42, 7}, {2, 0, 1});
  Shape dst = MakeShapeWithLayout(F32, {2, 3, 5}, {1, 0});
  ASSERT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, CopyLayoutSparseNotCompatibleSameRank) {
  Shape src = MakeShapeWithSparseLayout(F32, {123, 42, 7}, 6);
  Shape dst = MakeShapeWithLayout(F32, {2, 3, 5}, {1, 0});
  ASSERT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, CopyLayoutNotCompatibleDifferentRank) {
  Shape src = MakeShapeWithLayout(F32, {123, 42, 7}, {2, 0, 1});
  Shape dst = MakeShapeWithLayout(F32, {2, 3}, {1, 0});
  auto status = LayoutUtil::CopyLayoutBetweenShapes(src, &dst);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::ContainsRegex("cannot copy layout from shape"));
}

TEST_F(LayoutUtilTest, CopyLayoutSparseNotCompatibleDifferentRank) {
  Shape src = MakeShapeWithLayout(F32, {123, 42, 7}, {2, 0, 1});
  Shape dst = MakeShapeWithSparseLayout(F32, {2, 3}, 4);
  auto status = LayoutUtil::CopyLayoutBetweenShapes(src, &dst);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::ContainsRegex("cannot copy layout from shape"));
}

TEST_F(LayoutUtilTest, CopyLayoutNotCompatibleTuple) {
  Shape src =
      ShapeUtil::MakeTupleShape({MakeShapeWithLayout(F32, {2, 3}, {0, 1}),
                                 MakeShapeWithLayout(F32, {42, 123}, {1, 0}),
                                 ShapeUtil::MakeTupleShape({MakeShapeWithLayout(
                                     F32, {1, 2, 3}, {0, 2, 1})})});
  Shape dst = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {1, 0}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {1, 2, 0})})});

  auto status = LayoutUtil::CopyLayoutBetweenShapes(src, &dst);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::ContainsRegex("cannot copy layout from shape"));
}

TEST_F(LayoutUtilTest, CopyLayoutBogusLayout) {
  Shape src = ShapeUtil::MakeShape(F32, {2, 3});
  Shape dst = ShapeUtil::MakeShape(F32, {2, 3});
  // Set layout to invalid value.
  *src.mutable_layout() = LayoutUtil::MakeLayout({1, 2, 3, 4});

  auto status = LayoutUtil::CopyLayoutBetweenShapes(src, &dst);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(
      status.error_message(),
      ::testing::ContainsRegex("layout minor_to_major field contains .* "
                               "elements, but shape is rank"));
}

TEST_F(LayoutUtilTest, CopyTokenLayout) {
  Shape src = ShapeUtil::MakeTokenShape();
  Shape dst = ShapeUtil::MakeTokenShape();

  // Layouts are trivially the same for token types and copying layouts should
  // be a nop.
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, CopyOpaqueLayout) {
  Shape src = ShapeUtil::MakeOpaqueShape();
  Shape dst = ShapeUtil::MakeOpaqueShape();

  // Layouts are trivially the same for opaque types and copying layouts should
  // be a nop.
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, CopyTupleLayoutWithTokenAndOpaque) {
  Shape src = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {0, 1}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}), ShapeUtil::MakeTokenShape(),
       ShapeUtil::MakeTupleShape(
           {ShapeUtil::MakeOpaqueShape(), MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {0, 2, 1})})});
  Shape dst = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {1, 0}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}), ShapeUtil::MakeTokenShape(),
       ShapeUtil::MakeTupleShape(
           {ShapeUtil::MakeOpaqueShape(), MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {1, 2, 0})})});

  EXPECT_FALSE(LayoutUtil::LayoutsInShapesEqual(src, dst));
  EXPECT_IS_OK(LayoutUtil::CopyLayoutBetweenShapes(src, &dst));
  EXPECT_TRUE(LayoutUtil::LayoutsInShapesEqual(src, dst));
}

TEST_F(LayoutUtilTest, ClearLayoutTuple) {
  Shape shape = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3}, {1, 0}),
       MakeShapeWithLayout(F32, {42, 123}, {1, 0}),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3}, {1, 2, 0})})});
  EXPECT_TRUE(LayoutUtil::HasLayout(shape));
  EXPECT_TRUE(shape.tuple_shapes(0).has_layout());
  EXPECT_TRUE(shape.tuple_shapes(2).tuple_shapes(1).has_layout());

  LayoutUtil::ClearLayout(&shape);

  EXPECT_FALSE(LayoutUtil::HasLayout(shape));
  EXPECT_FALSE(shape.tuple_shapes(0).has_layout());
  EXPECT_FALSE(shape.tuple_shapes(2).tuple_shapes(1).has_layout());
}

TEST_F(LayoutUtilTest, ClearLayoutOpaqueAndToken) {
  // Opaque and token types trivially have layouts.
  for (Shape shape :
       {ShapeUtil::MakeOpaqueShape(), ShapeUtil::MakeTokenShape()}) {
    EXPECT_TRUE(LayoutUtil::HasLayout(shape));
    LayoutUtil::ClearLayout(&shape);
    EXPECT_TRUE(LayoutUtil::HasLayout(shape));
  }
}

TEST_F(LayoutUtilTest, SetToDefaultLayoutTuple) {
  Shape shape = ShapeUtil::MakeTupleShape(
      {MakeShapeWithLayout(F32, {2, 3, 4}, {1, 0, 2}),
       MakeShapeWithLayout(F32, {42, 123, 7}, {1, 2, 0}),
       ShapeUtil::MakeTupleShape(
           {MakeShapeWithLayout(F32, {}, {}),
            MakeShapeWithLayout(F32, {1, 2, 3, 4}, {3, 1, 2, 0})})});
  EXPECT_FALSE(LayoutUtil::Equal(shape.tuple_shapes(0).layout(),
                                 shape.tuple_shapes(1).layout()));
  LayoutUtil::SetToDefaultLayout(&shape);
  EXPECT_TRUE(LayoutUtil::Equal(shape.tuple_shapes(0).layout(),
                                shape.tuple_shapes(1).layout()));
  EXPECT_TRUE(LayoutUtil::Equal(
      LayoutUtil::GetDefaultLayoutForShape(shape.tuple_shapes(0)),
      shape.tuple_shapes(1).layout()));
}

TEST_F(LayoutUtilTest, DefaultLayoutGettersMajorToMinor) {
  EXPECT_TRUE(LayoutUtil::Equal(LayoutUtil::MakeLayout({1, 0}),
                                LayoutUtil::GetDefaultLayoutForR2()));
  EXPECT_TRUE(LayoutUtil::Equal(LayoutUtil::MakeLayout({2, 1, 0}),
                                LayoutUtil::GetDefaultLayoutForR3()));
  EXPECT_TRUE(LayoutUtil::Equal(LayoutUtil::MakeLayout({3, 2, 1, 0}),
                                LayoutUtil::GetDefaultLayoutForR4()));
  EXPECT_TRUE(
      LayoutUtil::Equal(LayoutUtil::MakeLayout({4, 3, 2, 1, 0}),
                        LayoutUtil::GetDefaultLayoutForShape(
                            ShapeUtil::MakeShape(F32, {10, 20, 30, 15, 25}))));
}

TEST_F(LayoutUtilTest, ValidateLayout_ValidArrayLayout) {
  Shape shape = ShapeUtil::MakeShapeWithLayout(F32, {2, 3}, {0, 1});
  auto status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/false);
  EXPECT_TRUE(status.ok());
  status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/true);
  EXPECT_TRUE(status.ok());
}

TEST_F(LayoutUtilTest, ValidateLayout_InvalidArrayLayout) {
  Shape shape = ShapeUtil::MakeShape(F32, {2, 3});
  *shape.mutable_layout() = LayoutUtil::MakeLayout({0, 1, 2});
  auto status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/false);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("layout minor_to_major field "
                                   "contains 3 elements, but shape is rank 2"));
  status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/true);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("layout minor_to_major field "
                                   "contains 3 elements, but shape is rank 2"));
}

TEST_F(LayoutUtilTest, ValidateLayout_MissingArrayLayout) {
  Shape shape = ShapeUtil::MakeShape(F32, {2, 3});
  LayoutUtil::ClearLayout(&shape);
  auto status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/false);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("shape f32[2,3] does not have a layout"));
  status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/true);
  EXPECT_TRUE(status.ok());
}

TEST_F(LayoutUtilTest, ValidateLayout_TupleWithLayout) {
  Shape shape = ShapeUtil::MakeTupleShape({});
  *shape.mutable_layout() = LayoutUtil::MakeLayout({0});
  auto status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/false);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("tuple should not have a layout field"));
  status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/true);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("tuple should not have a layout field"));
}

TEST_F(LayoutUtilTest, ValidateLayout_TupleSubshapesWithMissingLayouts) {
  Shape sub_1_1_1 = ShapeUtil::MakeShape(F32, {1, 2});
  Shape sub_1_1 = ShapeUtil::MakeTupleShape({sub_1_1_1});
  Shape sub_1_2 = ShapeUtil::MakeShape(F32, {1, 2});
  LayoutUtil::ClearLayout(&sub_1_2);
  Shape sub_1 = ShapeUtil::MakeTupleShape({sub_1_1, sub_1_2});
  Shape sub_2_1 = ShapeUtil::MakeShape(F32, {9});
  LayoutUtil::ClearLayout(&sub_2_1);
  Shape sub_2 = ShapeUtil::MakeTupleShape({sub_2_1});
  Shape shape = ShapeUtil::MakeTupleShape({sub_1, sub_2});

  auto status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/false);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("shape f32[1,2] does not have a layout"));
  status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/true);
  EXPECT_TRUE(status.ok());

  // Add invalid layout on one of sub-shapes.
  *shape.mutable_tuple_shapes(1)->mutable_tuple_shapes(0)->mutable_layout() =
      LayoutUtil::MakeLayout({0, 2, 3});

  status =
      LayoutUtil::ValidateLayoutInShape(shape, /*allow_missing_layouts=*/true);
  EXPECT_FALSE(status.ok());
  EXPECT_THAT(status.error_message(),
              ::testing::HasSubstr("layout minor_to_major field "
                                   "contains 3 elements, but shape is rank 1"));
}

}  // namespace
}  // namespace xla
