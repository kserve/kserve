/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

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
#ifndef TENSORFLOW_LITE_EXPERIMENTAL_C_C_API_H_
#define TENSORFLOW_LITE_EXPERIMENTAL_C_C_API_H_

#include <stdarg.h>
#include <stdint.h>

// Eventually the various C APIs defined in context.h will be migrated into
// the appropriate /c/c_api*.h header. For now, we pull in existing definitions
// for convenience.
#include "tensorflow/lite/context.h"

// --------------------------------------------------------------------------
// Experimental C API for TensorFlowLite.
//
// The API leans towards simplicity and uniformity instead of convenience, as
// most usage will be by language-specific wrappers.
//
// Conventions:
// * We use the prefix TFL_ for everything in the API.
// * size_t is used to represent byte sizes of objects that are
//   materialized in the address space of the calling process.
// * int is used as an index into arrays.

#ifdef SWIG
#define TFL_CAPI_EXPORT
#else
#if defined(_WIN32)
#ifdef TF_COMPILE_LIBRARY
#define TFL_CAPI_EXPORT __declspec(dllexport)
#else
#define TFL_CAPI_EXPORT __declspec(dllimport)
#endif  // TF_COMPILE_LIBRARY
#else
#define TFL_CAPI_EXPORT __attribute__((visibility("default")))
#endif  // _WIN32
#endif  // SWIG

#ifdef __cplusplus
extern "C" {
#endif  // __cplusplus

typedef TfLiteQuantizationParams TFL_QuantizationParams;
typedef TfLiteRegistration TFL_Registration;
typedef TfLiteStatus TFL_Status;
typedef TfLiteTensor TFL_Tensor;
typedef TfLiteType TFL_Type;

// --------------------------------------------------------------------------
// TFL_Model wraps a loaded TensorFlow Lite model.
typedef struct TFL_Model TFL_Model;

// Returns a model from the provided buffer, or null on failure.
TFL_CAPI_EXPORT extern TFL_Model* TFL_NewModel(const void* model_data,
                                               size_t model_size);

// Returns a model from the provided file, or null on failure.
TFL_CAPI_EXPORT extern TFL_Model* TFL_NewModelFromFile(const char* model_path);

// Destroys the model instance.
TFL_CAPI_EXPORT extern void TFL_DeleteModel(TFL_Model* model);

// --------------------------------------------------------------------------
// TFL_InterpreterOptions allows customized interpreter configuration.
typedef struct TFL_InterpreterOptions TFL_InterpreterOptions;

// Returns a new interpreter options instances.
TFL_CAPI_EXPORT extern TFL_InterpreterOptions* TFL_NewInterpreterOptions();

// Destroys the interpreter options instance.
TFL_CAPI_EXPORT extern void TFL_DeleteInterpreterOptions(
    TFL_InterpreterOptions* options);

// Sets the number of CPU threads to use for the interpreter.
TFL_CAPI_EXPORT extern void TFL_InterpreterOptionsSetNumThreads(
    TFL_InterpreterOptions* options, int32_t num_threads);

// Sets a custom error reporter for interpreter execution.
//
// * `reporter` takes the provided `user_data` object, as well as a C-style
//   format string and arg list (see also vprintf).
// * `user_data` is optional. If provided, it is owned by the client and must
//   remain valid for the duration of the interpreter lifetime.
TFL_CAPI_EXPORT extern void TFL_InterpreterOptionsSetErrorReporter(
    TFL_InterpreterOptions* options,
    void (*reporter)(void* user_data, const char* format, va_list args),
    void* user_data);

// --------------------------------------------------------------------------
// TFL_Interpreter provides inference from a provided model.
typedef struct TFL_Interpreter TFL_Interpreter;

// Returns a new interpreter using the provided model and options, or null on
// failure.
//
// * `model` must be a valid model instance. The caller retains ownership of the
//   object, and can destroy it immediately after creating the interpreter; the
//   interpreter will maintain its own reference to the underlying model data.
// * `optional_options` may be null. The caller retains ownership of the object,
//   and can safely destroy it immediately after creating the interpreter.
//
// NOTE: The client *must* explicitly allocate tensors before attempting to
// access input tensor data or invoke the interpreter.
TFL_CAPI_EXPORT extern TFL_Interpreter* TFL_NewInterpreter(
    const TFL_Model* model, const TFL_InterpreterOptions* optional_options);

// Destroys the interpreter.
TFL_CAPI_EXPORT extern void TFL_DeleteInterpreter(TFL_Interpreter* interpreter);

// Returns the number of input tensors associated with the model.
TFL_CAPI_EXPORT extern int TFL_InterpreterGetInputTensorCount(
    const TFL_Interpreter* interpreter);

// Returns the tensor associated with the input index.
// REQUIRES: 0 <= input_index < TFL_InterpreterGetInputTensorCount(tensor)
TFL_CAPI_EXPORT extern TFL_Tensor* TFL_InterpreterGetInputTensor(
    const TFL_Interpreter* interpreter, int32_t input_index);

// Resizes the specified input tensor.
//
// NOTE: After a resize, the client *must* explicitly allocate tensors before
// attempting to access the resized tensor data or invoke the interpreter.
// REQUIRES: 0 <= input_index < TFL_InterpreterGetInputTensorCount(tensor)
TFL_CAPI_EXPORT extern TFL_Status TFL_InterpreterResizeInputTensor(
    TFL_Interpreter* interpreter, int32_t input_index, const int* input_dims,
    int32_t input_dims_size);

// Updates allocations for all tensors, resizing dependent tensors using the
// specified input tensor dimensionality.
//
// This is a relatively expensive operation, and need only be called after
// creating the graph and/or resizing any inputs.
TFL_CAPI_EXPORT extern TFL_Status TFL_InterpreterAllocateTensors(
    TFL_Interpreter* interpreter);

// Runs inference for the loaded graph.
//
// NOTE: It is possible that the interpreter is not in a ready state to
// evaluate (e.g., if a ResizeInputTensor() has been performed without a call to
// AllocateTensors()).
TFL_CAPI_EXPORT extern TFL_Status TFL_InterpreterInvoke(
    TFL_Interpreter* interpreter);

// Returns the number of output tensors associated with the model.
TFL_CAPI_EXPORT extern int32_t TFL_InterpreterGetOutputTensorCount(
    const TFL_Interpreter* interpreter);

// Returns the tensor associated with the output index.
// REQUIRES: 0 <= input_index < TFL_InterpreterGetOutputTensorCount(tensor)
//
// NOTE: The shape and underlying data buffer for output tensors may be not
// be available until after the output tensor has been both sized and allocated.
// In general, best practice is to interact with the output tensor *after*
// calling TFL_InterpreterInvoke().
TFL_CAPI_EXPORT extern const TFL_Tensor* TFL_InterpreterGetOutputTensor(
    const TFL_Interpreter* interpreter, int32_t output_index);

// --------------------------------------------------------------------------
// TFL_Tensor wraps data associated with a graph tensor.
//
// Note that, while the TFL_Tensor struct is not currently opaque, and its
// fields can be accessed directly, these methods are still convenient for
// language bindings. In the future the tensor struct will likely be made opaque
// in the public API.

// Returns the type of a tensor element.
TFL_CAPI_EXPORT extern TFL_Type TFL_TensorType(const TFL_Tensor* tensor);

// Returns the number of dimensions that the tensor has.
TFL_CAPI_EXPORT extern int32_t TFL_TensorNumDims(const TFL_Tensor* tensor);

// Returns the length of the tensor in the "dim_index" dimension.
// REQUIRES: 0 <= dim_index < TFLiteTensorNumDims(tensor)
TFL_CAPI_EXPORT extern int32_t TFL_TensorDim(const TFL_Tensor* tensor,
                                             int32_t dim_index);

// Returns the size of the underlying data in bytes.
TFL_CAPI_EXPORT extern size_t TFL_TensorByteSize(const TFL_Tensor* tensor);

// Returns a pointer to the underlying data buffer.
//
// NOTE: The result may be null if tensors have not yet been allocated, e.g.,
// if the Tensor has just been created or resized and `TFL_AllocateTensors()`
// has yet to be called, or if the output tensor is dynamically sized and the
// interpreter hasn't been invoked.
TFL_CAPI_EXPORT extern void* TFL_TensorData(const TFL_Tensor* tensor);

// Returns the (null-terminated) name of the tensor.
TFL_CAPI_EXPORT extern const char* TFL_TensorName(const TFL_Tensor* tensor);

// Returns the parameters for asymmetric quantization. The quantization
// parameters are only valid when the tensor type is `kTfLiteUInt8` and the
// `scale != 0`. Quantized values can be converted back to float using:
//    real_value = scale * (quantized_value - zero_point);
TFL_CAPI_EXPORT extern TFL_QuantizationParams TFL_TensorQuantizationParams(
    const TFL_Tensor* tensor);

// Copies from the provided input buffer into the tensor's buffer.
// REQUIRES: input_data_size == TFL_TensorByteSize(tensor)
TFL_CAPI_EXPORT extern TFL_Status TFL_TensorCopyFromBuffer(
    TFL_Tensor* tensor, const void* input_data, size_t input_data_size);

// Copies to the provided output buffer from the tensor's buffer.
// REQUIRES: output_data_size == TFL_TensorByteSize(tensor)
TFL_CAPI_EXPORT extern TFL_Status TFL_TensorCopyToBuffer(
    const TFL_Tensor* output_tensor, void* output_data,
    size_t output_data_size);

#ifdef __cplusplus
}  // extern "C"
#endif  // __cplusplus

#endif  // TENSORFLOW_LITE_EXPERIMENTAL_C_C_API_H_
