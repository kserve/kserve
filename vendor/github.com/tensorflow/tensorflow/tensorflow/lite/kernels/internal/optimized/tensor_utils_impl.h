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
#ifndef TENSORFLOW_LITE_KERNELS_INTERNAL_OPTIMIZED_TENSOR_UTILS_IMPL_H_
#define TENSORFLOW_LITE_KERNELS_INTERNAL_OPTIMIZED_TENSOR_UTILS_IMPL_H_

// TODO(ghodrat): Remove this header file and the dependency to internal data
// structure.
#include "tensorflow/lite/c/builtin_op_data.h"

#if defined(_MSC_VER)
#define __restrict__ __restrict
#endif

#ifndef USE_NEON
#if defined(__ARM_NEON__) || defined(__ARM_NEON)
#define USE_NEON
#endif  //  defined(__ARM_NEON__) || defined(__ARM_NEON)
#endif  //  USE_NEON

namespace tflite {
namespace tensor_utils {

// Multiply a matrix by a batch vector, and store results in a batch-size
// vector.
void PortableMatrixBatchVectorMultiplyAccumulate(const float* matrix,
                                                 int m_rows, int m_cols,
                                                 const float* vector,
                                                 int n_batch, float* result,
                                                 int result_stride);
void NeonMatrixBatchVectorMultiplyAccumulate(const float* matrix, int m_rows,
                                             int m_cols, const float* vector,
                                             int n_batch, float* result,
                                             int result_stride);

// Matrix multiplication for quantized values using symmetric quantization.
void PortableMatrixBatchVectorMultiplyAccumulate(
    const int8_t* __restrict__ matrix, const int m_rows, const int m_cols,
    const int8_t* __restrict__ vectors, const float* scaling_factors,
    int n_batch, float* __restrict__ result, int result_stride);
void NeonMatrixBatchVectorMultiplyAccumulate(
    const int8_t* __restrict__ matrix, const int m_rows, const int m_cols,
    const int8_t* __restrict__ vectors, const float* scaling_factors,
    int n_batch, float* __restrict__ result, int result_stride);

// Cwise product of two vectors.
void PortableVectorVectorCwiseProduct(const float* vector1,
                                      const float* vector2, int v_size,
                                      float* result);
void NeonVectorVectorCwiseProduct(const float* vector1, const float* vector2,
                                  int v_size, float* result);

// Cwise product and accumulate of two vectors. Since it's a MAC operation, the
// assumption here is that result array is initialized to valid values.
void PortableVectorVectorCwiseProductAccumulate(const float* vector1,
                                                const float* vector2,
                                                int v_size, float* result);
void NeonVectorVectorCwiseProductAccumulate(const float* vector1,
                                            const float* vector2, int v_size,
                                            float* result);

// Dot product of two vectors.
float PortableVectorVectorDotProduct(const float* vector1, const float* vector2,
                                     int v_size);
float NeonVectorVectorDotProduct(const float* vector1, const float* vector2,
                                 int v_size);

// Dot product of two batch vectors.
void PortableBatchVectorBatchVectorDotProduct(const float* vector1,
                                              const float* vector2, int v_size,
                                              int n_batch, float* result,
                                              int result_stride);
void NeonBatchVectorBatchVectorDotProduct(const float* vector1,
                                          const float* vector2, int v_size,
                                          int n_batch, float* result,
                                          int result_stride);

// Cwise product of a vector and a batch-vector.
void PortableVectorBatchVectorCwiseProduct(const float* vector, int v_size,
                                           const float* batch_vector,
                                           int n_batch, float* result);
void NeonVectorBatchVectorCwiseProduct(const float* vector, int v_size,
                                       const float* batch_vector, int n_batch,
                                       float* result);

// Cwise product and accumulate of a vector and a batch-vector. Since it's a MAC
// operation, the assumption here is that result array is initialized to valid
// values.
void PortableVectorBatchVectorCwiseProductAccumulate(const float* vector,
                                                     int v_size,
                                                     const float* batch_vector,
                                                     int n_batch,
                                                     float* result);
void NeonVectorBatchVectorCwiseProductAccumulate(const float* vector,
                                                 int v_size,
                                                 const float* batch_vector,
                                                 int n_batch, float* result);

// Compute "1.0f - elements of vector" (used in CIFG).
void PortableSub1Vector(const float* vector, int v_size, float* result);
void NeonSub1Vector(const float* vector, int v_size, float* result);

// Clip elements of a vector using a abs_limit value.
void PortableClipVector(const float* vector, int v_size, float abs_limit,
                        float* result);
void NeonClipVector(const float* vector, int v_size, float abs_limit,
                    float* result);

// Add another vector for each batch in the batch vector.
void PortableVectorBatchVectorAdd(const float* vector, int v_size, int n_batch,
                                  float* batch_vector);

// Batch vector initialization with another vector.
void PortableVectorBatchVectorAssign(const float* vector, int v_size,
                                     int n_batch, float* batch_vector);

// Apply sigmoid to elements of a vector.
void PortableApplySigmoidToVector(const float* vector, int v_size,
                                  float* result);

// Apply activation function to elements of a vector.
void PortableApplyActivationToVector(const float* vector, int v_size,
                                     TfLiteFusedActivation activation,
                                     float* result);

// Copy vector to another vector.
void PortableCopyVector(const float* vector, int v_size, float* result);

// Fill vector with 0.f.
void PortableZeroVector(float* vector, int v_size);

// Multiply all elements of vector with a scalar.
void PortableVectorScalarMultiply(const int8_t* vector, int v_size, float scale,
                                  float* result);
void NeonVectorScalarMultiply(const int8_t* vector, int v_size, float scale,
                              float* result);

// Limit a float input f between +abs_limit and -abs_limit.
float PortableClip(float f, float abs_limit);

// Check if all entries of a vector are zero.
bool PortableIsZeroVector(const float* vector, int v_size);
bool NeonIsZeroVector(const float* vector, int v_size);

// Symmetric quantizer.
void PortableSymmetricQuantizeFloats(const float* values, const int size,
                                     int8_t* quantized_values, float* min,
                                     float* max, float* scaling_factor);
void NeonSymmetricQuantizeFloats(const float* values, const int size,
                                 int8_t* quantized_values, float* min,
                                 float* max, float* scaling_factor);

// Shift left a vector in place with v_size size.
void PortableVectorShiftLeft(float* vector, int v_size, float shift_value);
void NeonVectorShiftLeft(float* vector, int v_size, float shift_value);

// Reduce-sum on a float input vector:
// input_vector: float pointer to input vector.
// output_vector: float pointer to vector.
// output_size: output vector size.
// reduction_size: number of consecutive elements from input vector which are
// added to get one element of output.
void PortableReductionSumVector(const float* input_vector, float* output_vector,
                                int output_size, int reduction_size);
void NeonReductionSumVector(const float* input_vector, float* output_vector,
                            int output_size, int reduction_size);

void PortableMeanStddevNormalization(const float* input_vector,
                                     float* output_vector, int v_size,
                                     int n_batch, float normalization_epsilon);

}  // namespace tensor_utils
}  // namespace tflite

#endif  // TENSORFLOW_LITE_KERNELS_INTERNAL_OPTIMIZED_TENSOR_UTILS_IMPL_H_
