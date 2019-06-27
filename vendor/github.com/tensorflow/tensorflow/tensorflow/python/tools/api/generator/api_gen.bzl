"""Targets for generating TensorFlow Python API __init__.py files."""

load("//tensorflow/python/tools/api/generator:api_init_files.bzl", "TENSORFLOW_API_INIT_FILES")

def get_compat_files(
        file_paths,
        compat_api_version):
    """Prepends compat/v<compat_api_version> to file_paths."""
    return ["compat/v%d/%s" % (compat_api_version, f) for f in file_paths]

def gen_api_init_files(
        name,
        output_files = TENSORFLOW_API_INIT_FILES,
        root_init_template = None,
        srcs = [],
        api_name = "tensorflow",
        api_version = 2,
        compat_api_versions = [],
        compat_init_templates = [],
        packages = ["tensorflow.python", "tensorflow.lite.python.lite"],
        package_deps = ["//tensorflow/python:no_contrib"],
        output_package = "tensorflow",
        output_dir = "",
        root_file_name = "__init__.py"):
    """Creates API directory structure and __init__.py files.

    Creates a genrule that generates a directory structure with __init__.py
    files that import all exported modules (i.e. modules with tf_export
    decorators).

    Args:
      name: name of genrule to create.
      output_files: List of __init__.py files that should be generated.
        This list should include file name for every module exported using
        tf_export. For e.g. if an op is decorated with
        @tf_export('module1.module2', 'module3'). Then, output_files should
        include module1/module2/__init__.py and module3/__init__.py.
      root_init_template: Python init file that should be used as template for
        root __init__.py file. "# API IMPORTS PLACEHOLDER" comment inside this
        template will be replaced with root imports collected by this genrule.
      srcs: genrule sources. If passing root_init_template, the template file
        must be included in sources.
      api_name: Name of the project that you want to generate API files for
        (e.g. "tensorflow" or "estimator").
      api_version: TensorFlow API version to generate. Must be either 1 or 2.
      compat_api_versions: Older TensorFlow API versions to generate under
        compat/ directory.
      compat_init_templates: Python init file that should be used as template
        for top level __init__.py files under compat/vN directories.
        "# API IMPORTS PLACEHOLDER" comment inside this
        template will be replaced with root imports collected by this genrule.
      packages: Python packages containing the @tf_export decorators you want to
        process
      package_deps: Python library target containing your packages.
      output_package: Package where generated API will be added to.
      output_dir: Subdirectory to output API to.
        If non-empty, must end with '/'.
      root_file_name: Name of the root file with all the root imports.
    """
    root_init_template_flag = ""
    if root_init_template:
        root_init_template_flag = "--root_init_template=$(location " + root_init_template + ")"

    primary_package = packages[0]
    api_gen_binary_target = ("create_" + primary_package + "_api_%d_%s") % (api_version, name)
    native.py_binary(
        name = api_gen_binary_target,
        srcs = ["//tensorflow/python/tools/api/generator:create_python_api.py"],
        main = "//tensorflow/python/tools/api/generator:create_python_api.py",
        srcs_version = "PY2AND3",
        visibility = ["//visibility:public"],
        deps = package_deps + [
            "//tensorflow/python:util",
            "//tensorflow/python/tools/api/generator:doc_srcs",
        ],
    )

    # Replace name of root file with root_file_name.
    output_files = [
        root_file_name if f == "__init__.py" else f
        for f in output_files
    ]
    all_output_files = ["%s%s" % (output_dir, f) for f in output_files]
    compat_api_version_flags = ""
    for compat_api_version in compat_api_versions:
        compat_api_version_flags += " --compat_apiversion=%d" % compat_api_version

    compat_init_template_flags = ""
    for compat_init_template in compat_init_templates:
        compat_init_template_flags += (
            " --compat_init_template=$(location %s)" % compat_init_template
        )

    native.genrule(
        name = name,
        outs = all_output_files,
        cmd = (
            "$(location :" + api_gen_binary_target + ") " +
            root_init_template_flag + " --apidir=$(@D)" + output_dir +
            " --apiname=" + api_name + " --apiversion=" + str(api_version) +
            compat_api_version_flags + " " + compat_init_template_flags +
            " --package=" + ",".join(packages) +
            " --output_package=" + output_package + " $(OUTS)"
        ),
        srcs = srcs,
        tools = [":" + api_gen_binary_target],
        visibility = [
            "//tensorflow:__pkg__",
            "//tensorflow/tools/api/tests:__pkg__",
        ],
    )
