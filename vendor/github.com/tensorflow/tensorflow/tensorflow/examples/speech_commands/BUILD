package(
    default_visibility = [
        "//visibility:public",
    ],
)

licenses(["notice"])  # Apache 2.0

exports_files([
    "LICENSE",
])

load("//tensorflow:tensorflow.bzl", "tf_cc_binary")
load("//tensorflow:tensorflow.bzl", "tf_cc_test")
load("//tensorflow:tensorflow.bzl", "tf_py_test")

py_library(
    name = "models",
    srcs = [
        "models.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        "//tensorflow:tensorflow_py",
        "//third_party/py/numpy",
        "@six_archive//:six",
    ],
)

tf_py_test(
    name = "models_test",
    size = "small",
    srcs = ["models_test.py"],
    additional_deps = [
        ":models",
        "//tensorflow/python:client_testlib",
    ],
)

py_library(
    name = "input_data",
    srcs = [
        "input_data.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        "//tensorflow:tensorflow_py",
        "//third_party/py/numpy",
        "@six_archive//:six",
    ],
)

tf_py_test(
    name = "input_data_test",
    size = "small",
    srcs = ["input_data_test.py"],
    additional_deps = [
        ":input_data",
        ":models",
        "//tensorflow/python:client_testlib",
    ],
)

py_binary(
    name = "train",
    srcs = [
        "train.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        ":input_data",
        ":models",
        "//tensorflow:tensorflow_py",
        "//third_party/py/numpy",
        "@six_archive//:six",
    ],
)

py_binary(
    name = "freeze",
    srcs = [
        "freeze.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        ":input_data",
        ":models",
        "//tensorflow:tensorflow_py",
        "//third_party/py/numpy",
        "@six_archive//:six",
    ],
)

tf_py_test(
    name = "freeze_test",
    size = "small",
    srcs = ["freeze_test.py"],
    additional_deps = [
        ":freeze",
        "//tensorflow/python:client_testlib",
    ],
)

py_binary(
    name = "wav_to_features",
    srcs = [
        "wav_to_features.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        ":input_data",
        ":models",
        "//tensorflow:tensorflow_py",
        "//third_party/py/numpy",
        "@six_archive//:six",
    ],
)

tf_py_test(
    name = "wav_to_features_test",
    size = "small",
    srcs = ["wav_to_features_test.py"],
    additional_deps = [
        ":wav_to_features",
        "//tensorflow/python:client_testlib",
    ],
)

py_binary(
    name = "generate_streaming_test_wav",
    srcs = [
        "generate_streaming_test_wav.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        ":input_data",
        ":models",
        "//tensorflow:tensorflow_py",
        "//third_party/py/numpy",
        "@six_archive//:six",
    ],
)

tf_py_test(
    name = "generate_streaming_test_wav_test",
    size = "small",
    srcs = ["generate_streaming_test_wav_test.py"],
    additional_deps = [
        ":generate_streaming_test_wav",
        "//tensorflow/python:client_testlib",
    ],
)

tf_cc_binary(
    name = "label_wav_cc",
    srcs = [
        "label_wav.cc",
    ],
    deps = [
        "//tensorflow/core:core_cpu",
        "//tensorflow/core:framework",
        "//tensorflow/core:framework_internal",
        "//tensorflow/core:lib",
        "//tensorflow/core:protos_all_cc",
        "//tensorflow/core:tensorflow",
    ],
)

py_binary(
    name = "label_wav",
    srcs = [
        "label_wav.py",
    ],
    srcs_version = "PY2AND3",
    deps = [
        "//tensorflow:tensorflow_py",
    ],
)

tf_py_test(
    name = "label_wav_test",
    size = "medium",
    srcs = ["label_wav_test.py"],
    additional_deps = [
        ":label_wav",
        "//tensorflow/python:client_testlib",
    ],
)

cc_library(
    name = "recognize_commands",
    srcs = [
        "recognize_commands.cc",
    ],
    hdrs = [
        "recognize_commands.h",
    ],
    deps = [
        "//tensorflow/core:core_cpu",
        "//tensorflow/core:framework",
        "//tensorflow/core:framework_internal",
        "//tensorflow/core:lib",
        "//tensorflow/core:protos_all_cc",
        "//tensorflow/core:tensorflow",
    ],
)

tf_cc_test(
    name = "recognize_commands_test",
    size = "medium",
    srcs = [
        "recognize_commands_test.cc",
    ],
    deps = [
        ":recognize_commands",
        "//tensorflow/core:lib",
        "//tensorflow/core:lib_internal",
        "//tensorflow/core:test",
        "//tensorflow/core:test_main",
        "//tensorflow/core:testlib",
    ],
)

cc_library(
    name = "accuracy_utils",
    srcs = [
        "accuracy_utils.cc",
    ],
    hdrs = [
        "accuracy_utils.h",
    ],
    deps = [
        "//tensorflow/core:core_cpu",
        "//tensorflow/core:framework",
        "//tensorflow/core:framework_internal",
        "//tensorflow/core:lib",
        "//tensorflow/core:protos_all_cc",
        "//tensorflow/core:tensorflow",
    ],
)

tf_cc_test(
    name = "accuracy_utils_test",
    size = "medium",
    srcs = [
        "accuracy_utils_test.cc",
    ],
    deps = [
        ":accuracy_utils",
        "//tensorflow/core:lib",
        "//tensorflow/core:lib_internal",
        "//tensorflow/core:test",
        "//tensorflow/core:test_main",
        "//tensorflow/core:testlib",
    ],
)

tf_cc_binary(
    name = "test_streaming_accuracy",
    srcs = [
        "test_streaming_accuracy.cc",
    ],
    deps = [
        ":accuracy_utils",
        ":recognize_commands",
        "//tensorflow/core:core_cpu",
        "//tensorflow/core:framework",
        "//tensorflow/core:framework_internal",
        "//tensorflow/core:lib",
        "//tensorflow/core:lib_internal",
        "//tensorflow/core:protos_all_cc",
    ],
)
