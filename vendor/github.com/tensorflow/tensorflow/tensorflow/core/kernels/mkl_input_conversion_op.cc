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

#ifdef INTEL_MKL

#include <algorithm>
#include <vector>
#include "tensorflow/core/framework/numeric_op.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/register_types.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/kernels/ops_util.h"
#include "tensorflow/core/platform/byte_order.h"
#include "tensorflow/core/platform/cpu_info.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/util/tensor_format.h"

#include "mkldnn.hpp"
#include "tensorflow/core/kernels/mkl_tfconv_op.h"
#include "tensorflow/core/util/mkl_util.h"
using mkldnn::stream;

namespace tensorflow {
typedef Eigen::ThreadPoolDevice CPUDevice;

///////////////////////////////////////////////////////////
//               Op kernel
// Checks and ensures that the 2 inputs are compatible for mkl binary ops.
// Here's the basic logic:
//
// if both inputs are in TF format:
//   pass the inputs through to the output
// else if both inputs are in mkl format:
//   if both have the same shape:
//     pass the inputs through to the output
//   else:
//     convert both to TF
// else if one is TF and one is MKL:
//   if broadcast is needed:
//     convert the MKL format input to TF format
//   else:
//     convert the TF format input to MKL format
///////////////////////////////////////////////////////////

template <typename Device, typename T>
class MklInputConversionOp : public OpKernel {
 public:
  explicit MklInputConversionOp(OpKernelConstruction* context)
      : OpKernel(context) {
    OP_REQUIRES_OK(context, context->GetAttr("data_format", &data_format_str));
    OP_REQUIRES_OK(context, context->GetAttr("T", &op_data_type));
    has_avx512f_ = port::TestCPUFeature(port::CPUFeature::AVX512F);
  }

 private:
  void Compute(OpKernelContext* context) override {
    const int kInputIndex_0 = 0, kInputIndex_1 = 1;
    const Tensor& input_tensor_0 = MklGetInput(context, kInputIndex_0);
    MklDnnShape input_shape_0;
    GetMklShape(context, kInputIndex_0, &input_shape_0);

    const Tensor& input_tensor_1 = MklGetInput(context, kInputIndex_1);
    MklDnnShape input_shape_1;
    GetMklShape(context, kInputIndex_1, &input_shape_1);

    VLOG(1) << "MklInputConversionOp: Input shapes are: "
            << context->input(kInputIndex_0).shape().DebugString() << " and "
            << context->input(kInputIndex_1).shape().DebugString();

    // - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
    // if both inputs are in TF format, just copy input tensors to output.
    if (!input_shape_0.IsMklTensor() && !input_shape_1.IsMklTensor()) {
      VLOG(1) << "MklInputConversionOp: No conversion needed, "
              << "copying TF inputs to output";

      ForwardTfTensorInToOut(context, kInputIndex_0, kInputIndex_0);
      ForwardTfTensorInToOut(context, kInputIndex_1, kInputIndex_1);
      return;
    }

    // - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
    // If both inputs are in MKL format
    if (input_shape_0.IsMklTensor() && input_shape_1.IsMklTensor()) {
      // It is safer to compare the original TensorFlow shapes than to compare
      // Mkl shapes since element wise ops are forwarded to Eigen
      // implementation.
      TensorShape tf_shape0 = input_shape_0.GetTfShape();
      TensorShape tf_shape1 = input_shape_1.GetTfShape();
      TensorShape tensor_shape0 = input_tensor_0.shape();
      TensorShape tensor_shape1 = input_tensor_1.shape();
      if (tf_shape0 == tf_shape1 && tensor_shape0 == tensor_shape1) {
        auto input0_md = input_shape_0.GetMklLayout();
        auto input1_md = input_shape_1.GetMklLayout();

        // If both have the same shape and same format, pass them through
        if (input0_md.data.format == input1_md.data.format) {
          VLOG(1) << "MklInputConversionOp: No conversion needed, "
                  << "copying MKL inputs with identical shapes to output";

          ForwardMklTensorInToOut(context, kInputIndex_0, kInputIndex_0);
          ForwardMklTensorInToOut(context, kInputIndex_1, kInputIndex_1);
          return;
        } else {
          VLOG(1) << "MklInputConversionOp: Shape is same, but format is "
                     "different, "
                  << "need to convert to same format";
          // TODO: For now, input0 is converted and input1 is unchanged
          //       we should choose the optimal MKL format to convert to.
          Tensor* tensor_out;
          MklDnnShape mkl_output_mkl_shape;
          mkl_output_mkl_shape.SetMklTensor(true);
          mkl_output_mkl_shape.SetElemType(MklDnnType<T>());
          mkl_output_mkl_shape.SetTfLayout(input_shape_0.GetDimension(),
                                           input_shape_0.GetSizesAsMklDnnDims(),
                                           input_shape_0.GetTfDataFormat());

          // Get MKL layout from input1 as destination layout
          mkl_output_mkl_shape.SetMklLayout(&input1_md);

          // Create output Mkl tensor for index 0
          AllocateOutputSetMklShape(context, kInputIndex_0, &tensor_out,
                                    input_tensor_0.shape(),
                                    mkl_output_mkl_shape);

          // Create MklDnnData object for input0 tesnsor
          auto cpu_engine = engine(engine::cpu, 0);
          MklDnnData<T> input(&cpu_engine);
          input.SetUsrMem(input0_md, &input_tensor_0);

          // Create reorder from input0's layout to input1's layout
          std::vector<primitive> net;
          CHECK_EQ(input.CheckReorderToOpMem(
                       memory::primitive_desc(input1_md, cpu_engine),
                       tensor_out, &net),
                   true);
          stream(stream::kind::eager).submit(net).wait();

          // Input1 will be passed through
          ForwardMklTensorInToOut(context, kInputIndex_1, kInputIndex_1);
          return;
        }
      }

      // Sanity check
      bool mkl_shapes_are_same = ((input_shape_0 == input_shape_1) &&
                                  (tensor_shape0 == tensor_shape1));
      if (mkl_shapes_are_same) {
        CHECK(false) << "MklInputConversionOp: Unexpected: TF shapes are "
                        "different but MKL shapes are same";
      }

      // Both have different shapes, so broadcast will be necessary.
      // Convert to TF and pass both tensors through (we can't do broadcast
      // with MKL tensors)
      VLOG(1) << "MklInputConversionOp: Broadcast needed, "
              << "converted MKL inputs to TF format";
      // TODO: Cleanup op_data_type and has_avx512f_ after these two parameters
      //       are removed from ConvertMklToTf
      MklToTfOp<Device, T>::ConvertMklToTf(this, context, data_format_str,
                                           op_data_type, has_avx512f_,
                                           kInputIndex_0);
      MklToTfOp<Device, T>::ConvertMklToTf(this, context, data_format_str,
                                           op_data_type, has_avx512f_,
                                           kInputIndex_1);
      SetDummyMklDnnShapeOutput(context, kInputIndex_0);
      SetDummyMklDnnShapeOutput(context, kInputIndex_1);
      return;
    }

    // - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
    // One input is MKL and one is TF. If no broadcast is needed, convert
    // the TF tensor to MKL, otherwise convert the MKL tensor to TF format
    VLOG(1) << "MklInputConversionOp: Inputs in different formats (MKL/TF)";

    const Tensor* mkl_tensor;
    const MklDnnShape* mkl_shape;
    const Tensor* tf_tensor;
    uint mkl_tensor_index;
    uint tf_tensor_index;
    if (input_shape_0.IsMklTensor() && !input_shape_1.IsMklTensor()) {
      mkl_tensor = &input_tensor_0;
      mkl_shape = &input_shape_0;
      mkl_tensor_index = 0;
      tf_tensor = &input_tensor_1;
      tf_tensor_index = 1;
    } else if (!input_shape_0.IsMklTensor() && input_shape_1.IsMklTensor()) {
      mkl_tensor = &input_tensor_1;
      mkl_shape = &input_shape_1;
      mkl_tensor_index = 1;
      tf_tensor = &input_tensor_0;
      tf_tensor_index = 0;
    } else {
      CHECK(false) << "MklInputConversionOp: Unexpected combination of input "
                      "shapes for MKL "
                   << "element-wise op";
    }

    // Broadcast is needed if the shapes are not the same
    if (mkl_shape->GetTfShape().num_elements() ==
        tf_tensor->shape().num_elements()) {
      // Both shapes are same, convert the TF input to MKL
      VLOG(1) << "MklInputConversionOp: No broadcast needed.";
      VLOG(1) << "MklInputConversionOp: Converting input " << tf_tensor_index
              << " to MKL format";

      // Create MklDnnShape for output Mkl tensor.
      Tensor* tensor_out;
      MklDnnShape mkl_output_mkl_shape;
      mkl_output_mkl_shape.SetMklTensor(true);
      mkl_output_mkl_shape.SetElemType(MklDnnType<T>());
      mkl_output_mkl_shape.SetTfLayout(mkl_shape->GetDimension(),
                                       mkl_shape->GetSizesAsMklDnnDims(),
                                       mkl_shape->GetTfDataFormat());
      // ** Temporarily borrow the layout from the MKL input **
      auto output_mkl_md = mkl_shape->GetMklLayout();
      mkl_output_mkl_shape.SetMklLayout(&output_mkl_md);

      // Create output Mkl tensor
      AllocateOutputSetMklShape(context, tf_tensor_index, &tensor_out,
                                mkl_tensor->shape(), mkl_output_mkl_shape);

      // Create MklDnnData object for input tensor. Input tensor is in
      // Tensorflow layout.
      auto cpu_engine = engine(engine::cpu, 0);
      MklDnnData<T> tf_input(&cpu_engine);
      auto input_tf_md = mkl_output_mkl_shape.GetTfLayout();
      tf_input.SetUsrMem(input_tf_md, tf_tensor);

      // Create reorder between tensorflow layout and Mkl layout if necessary
      std::vector<primitive> net;
      bool reordered = tf_input.CheckReorderToOpMem(
          memory::primitive_desc(output_mkl_md, cpu_engine), tensor_out, &net);

      if (!reordered) {
        // This is the case that the TF tensor has the same shape and format of
        // mkl tensor. However, tf_tensor can not be simply forwarded to the
        // output tensor since mkl data tensor is always one dimensional tensor.
        // Tensor::CopyFrom shares the buffer of the other tensor while set its
        // shape to the other tensor.
        CHECK(tensor_out->CopyFrom(*tf_tensor, tensor_out->shape()));
      } else {
        stream(stream::kind::eager).submit(net).wait();
      }

      // -- The tensor in MKL format passes through --
      ForwardMklTensorInToOut(context, mkl_tensor_index, mkl_tensor_index);
    } else {
      // Broadcast is needed, so convert the MKL input to TF
      VLOG(1) << "MklInputConversionOp: Broadcast needed.";
      VLOG(1) << "MklInputConversionOp: Converting input " << mkl_tensor_index
              << " to TF format";
      MklToTfOp<Device, T>::ConvertMklToTf(this, context, data_format_str,
                                           op_data_type, has_avx512f_,
                                           mkl_tensor_index);
      SetDummyMklDnnShapeOutput(context, mkl_tensor_index);

      // The tensor in TF format passes through
      ForwardTfTensorInToOut(context, tf_tensor_index, tf_tensor_index);
    }

    VLOG(1) << "MklInputConversionOp: Shapes (output): "
            << context->mutable_output(kInputIndex_0)->shape().DebugString()
            << " and "
            << context->mutable_output(kInputIndex_1)->shape().DebugString();

    VLOG(1) << "MklInputConversion completed successfully.";
  }

 private:
  /// Data format of the operation
  string data_format_str;

  /// Data type of the operation
  DataType op_data_type;

  /// CPUIDInfo
  bool has_avx512f_ = false;
};

///////////////////////////////////////////////////////////
//               Register kernel
///////////////////////////////////////////////////////////

#define REGISTER_CPU(T)                                             \
  REGISTER_KERNEL_BUILDER(Name("_MklInputConversion")               \
                              .Device(DEVICE_CPU)                   \
                              .TypeConstraint<T>("T")               \
                              .Label(mkl_op_registry::kMklOpLabel), \
                          MklInputConversionOp<CPUDevice, T>);

// TODO(nhasabni): We cannot support all number types since MklDnn does
// not support types.
// TF_CALL_NUMBER_TYPES(REGISTER_CPU);
TF_CALL_float(REGISTER_CPU);
#undef REGISTER_CPU
}  // namespace tensorflow
#endif  // INTEL_MKL
