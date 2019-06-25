# Description:
#    LIBXSMM: Library for small matrix-matrix multiplications targeting Intel Architecture (x86).

licenses(["notice"])  # BSD 3-clause

exports_files(["LICENSE.md"])

# Arguments to ./scripts/libxsmm_interface.py, see that file for detailed description.
#  precision: SP & DP
#  prefetch: 1 (auto)
libxsmm_interface_arguments = "0 1"

# Arguments to ./scripts/libxsmm_config.py, see that file for detailed description.
# rely on default arguments
libxsmm_config_arguments = ""

# Arguments to ./scripts/libxsmm_dispatch.py, see that file for detailed description.
#  (dummy argument)
libxsmm_dispatch_arguments = "0"

genrule(
    name = "libxsmm_headers",
    srcs = [
        "src/template/libxsmm.h",
        "src/template/libxsmm_config.h",
    ],
    outs = [
        "include/libxsmm.h",
        "include/libxsmm_config.h",
        "include/libxsmm_dispatch.h",
    ],
    cmd = "$(location :libxsmm_interface) $(location src/template/libxsmm.h) " + libxsmm_interface_arguments + " > $(location include/libxsmm.h);" +
          "$(location :libxsmm_config) $(location src/template/libxsmm_config.h) " + libxsmm_config_arguments + " > $(location include/libxsmm_config.h);" +
          "$(location :libxsmm_dispatch) " + libxsmm_dispatch_arguments + " > $(location include/libxsmm_dispatch.h)",
    tools = [
        ":libxsmm_config",
        ":libxsmm_dispatch",
        ":libxsmm_interface",
    ],
    visibility = [
        "//tensorflow/core/kernels:__pkg__",
        "//third_party/eigen3:__pkg__",
    ],
)

cc_library(
    name = "xsmm_avx",
    srcs = [
        "src/libxsmm_cpuid_x86.c",
        "src/libxsmm_dnn.c",
        "src/libxsmm_dnn_convolution_backward.c",
        "src/libxsmm_dnn_convolution_forward.c",
        "src/libxsmm_dnn_convolution_weight_update.c",
        "src/libxsmm_dnn_convolution_winograd_backward.c",
        "src/libxsmm_dnn_convolution_winograd_forward.c",
        "src/libxsmm_dnn_convolution_winograd_weight_update.c",
        "src/libxsmm_dnn_handle.c",
        "src/libxsmm_dump.c",
        "src/libxsmm_ext_gemm.c",
        "src/libxsmm_ext_trans.c",
        "src/libxsmm_fsspmdm.c",
        "src/libxsmm_gemm.c",
        "src/libxsmm_main.c",
        "src/libxsmm_malloc.c",
        "src/libxsmm_perf.c",
        "src/libxsmm_spmdm.c",
        "src/libxsmm_sync.c",
        "src/libxsmm_timer.c",
        "src/libxsmm_trace.c",
        "src/libxsmm_trans.c",
    ] + glob([
        "src/generator_*.c",
    ]),
    hdrs = [
        "include/libxsmm_cpuid.h",
        "include/libxsmm_dnn.h",
        "include/libxsmm_frontend.h",
        "include/libxsmm_fsspmdm.h",
        "include/libxsmm_generator.h",
        "include/libxsmm_intrinsics_x86.h",
        "include/libxsmm_macros.h",
        "include/libxsmm_malloc.h",
        "include/libxsmm_spmdm.h",
        "include/libxsmm_sync.h",
        "include/libxsmm_timer.h",
        "include/libxsmm_typedefs.h",
        # Source files #included internally:
        "src/libxsmm_gemm_diff.c",
        "src/libxsmm_hash.c",
        # Generated:
        "include/libxsmm.h",
        "include/libxsmm_config.h",
        "include/libxsmm_dispatch.h",
    ] + glob([
        # trigger rebuild if template changed
        "src/template/*.c",
    ]),
    copts = [
        "-mavx",  # JIT does not work without avx anyway, and this silences some CRC32 warnings.
        "-Wno-vla",  # Libxsmm convolutions heavily use VLA.
    ],
    defines = [
        "LIBXSMM_BUILD",
        "__BLAS=0",
    ],
    includes = [
        "include",
        "src",
        "src/template",
    ],
    visibility = ["//visibility:public"],
)

py_library(
    name = "libxsmm_scripts",
    srcs = glob(["scripts/*.py"]),
    data = ["version.txt"],
)

py_binary(
    name = "libxsmm_interface",
    srcs = ["scripts/libxsmm_interface.py"],
    deps = [":libxsmm_scripts"],
)

py_binary(
    name = "libxsmm_config",
    srcs = ["scripts/libxsmm_config.py"],
    deps = [":libxsmm_scripts"],
)

py_binary(
    name = "libxsmm_dispatch",
    srcs = ["scripts/libxsmm_dispatch.py"],
    deps = [":libxsmm_scripts"],
)
