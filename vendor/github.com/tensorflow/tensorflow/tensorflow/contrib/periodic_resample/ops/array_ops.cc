// =============================================================================
// Copyright 2016 The TensorFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// =============================================================================

#include "tensorflow/core/framework/common_shape_fns.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/shape_inference.h"

namespace tensorflow {

REGISTER_OP("PeriodicResample")
    .Attr("T: numbertype")
    .Input("values: T")
    .Attr("shape: shape")
    .Output("output: T")
    .SetShapeFn([](shape_inference::InferenceContext* c) {
      tensorflow::PartialTensorShape desired_shape;
      TF_RETURN_IF_ERROR(c->GetAttr("shape", &desired_shape));
      shape_inference::ShapeHandle input_tensor_shape = c->input(0);
      shape_inference::DimensionHandle num_input_elements =
          c->NumElements(input_tensor_shape);
      shape_inference::ShapeHandle result_shape_handle;
      if (!shape_inference::InferenceContext::ValueKnown(num_input_elements)) {
        TF_RETURN_IF_ERROR(c->MakeShapeFromPartialTensorShape(
            desired_shape, &result_shape_handle));
      } else {
        const int rank = c->Rank(input_tensor_shape);
        std::vector<tensorflow::int64> target_dimensions(rank);
        tensorflow::int64 new_sliced_size = 1;
        int adjustable_dimension = 0;
        for (int i = 0; i < rank; ++i) {
          if (desired_shape.dim_size(i) < 1) {
            adjustable_dimension = i;
          } else {
            target_dimensions[i] = desired_shape.dim_size(i);
            new_sliced_size *= target_dimensions[i];
          }
        }
        target_dimensions[adjustable_dimension] =
            shape_inference::InferenceContext::Value(
                num_input_elements) / new_sliced_size;
        tensorflow::TensorShape result_shape;
        for (int i = 0; i < rank; ++i) {
          result_shape.AddDim(target_dimensions[i]);
        }
        TF_RETURN_IF_ERROR(c->MakeShapeFromTensorShape(
            result_shape, &result_shape_handle));
      }
      c->set_output(0, result_shape_handle);
      return Status::OK();
    })
    .Doc(R"doc(
Periodically resample elements of a tensor to conform to `shape`.

This function implements a slightly more generic version of the subpixel
convolutions found in this [paper](https://arxiv.org/abs/1609.05158).

The formula for computing the elements in the `output` tensor is as follows:

  `T` = `values` tensor of rank `R`

  `S` = desired `shape` of output tensor (vector of length `R`)

  `P` = `output` tensor of rank `R`

  \\((T_1,\\ldots,T_R)\\) = shape(`T`)

  \\([S_1,\\ldots,S_q,\\ldots,S_R]\\) = elements of vector `S`

  A single element in `S` is left unspecified (denoted \\(S_q=-1\\)).

  Let \\(f_i\\) denote the (possibly non-integer) factor that relates the original
  dimension to the desired dimensions, \\(S_i=f_i T_i\\), for \\(i\\neq q\\) where
  \\(f_i>0\\).

  Define the following:

  \\(g_i=\\lceil f_i\\rceil\\)

  \\(t=\\prod_i T_i\\)

  \\(s=\\prod_{i\\neq q} S_i\\)

  \\(S_q\\) can then be defined by \\(S_q=\\lfloor t/s\\rfloor\\).
  The elements of the resulting tensor are defined as

  \\(P_{s_1,\\ldots,s_R}=T_{h_1,\\ldots,h_q,\\ldots,h_R}\\).

  The \\(h_i\\) (\\(i\\neq q\\)) are defined by \\(h_i=\\lfloor s_i/g_i\\rfloor\\).

  \\(h_q=S_q\\sum_{j\\neq q}^{q-1}G_j \\mathrm{mod}(s_j,g_j) + s_q\\), where
  \\(G_j=\\prod_{i}^{j-1}g_i\\) (\\(G_0=1\\)).

One drawback of this method is that whenever the output dimensions are slightly
less than integer multiples of the input dimensions, many of the tensor elements
are repeated in an inefficient way. This is resolved by specifying that all
desired dimensions are integer multiples of the input tensor.

For example:

```prettyprint
`input` is [[ 0  1  2  3]
            [ 4  5  6  7]
            [ 8  9 10 11]]

tf.periodic_resample(input, [6, None]) ==> [[ 0  1]
                                            [ 2  3]
                                            [ 4  5]
                                            [ 6  7]
                                            [ 8  9]
                                            [10 11]]
```

values: The tensor of rank `R` to periodic_resample
shape: A 1-D tensor representing the desired shape of the output tensor.
  Exactly one element of this tensor must have the value `None` which represents
  that this dimension of `values` can be adjusted downward in order to
  accommodate increases in other dimensions. The specified sizes of the
  non-adjustable dimensions must by at least as large as in the `values` tensor.
output: Periodically resampled tensor that has dimensions specified as in
  `shape` except that the dimension specified as `None` will be minimally
  decreased as necessary.

)doc");


REGISTER_OP("PeriodicResampleOpGrad")
    .Attr("T: numbertype")
    .Input("grad: T")
    .Attr("original_shape: shape")
    .Attr("desired_shape: shape")
    .Output("grad_values: T")
    .SetShapeFn([](shape_inference::InferenceContext* c) {
      tensorflow::TensorShape original_shape;
      TF_RETURN_IF_ERROR(c->GetAttr("original_shape", &original_shape));
      shape_inference::ShapeHandle s;
      TF_RETURN_IF_ERROR(c->MakeShapeFromTensorShape(original_shape, &s));
      c->set_output(0, s);
      return Status::OK();
});

}  // namespace tensorflow
