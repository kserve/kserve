major_version: "local"
minor_version: ""
default_target_cpu: "same_as_host"

toolchain {
  abi_version: "local"
  abi_libc_version: "local"
  compiler: "compiler"
  host_system_name: "local"
  needsPic: true
  target_libc: "local"
  target_cpu: "local"
  target_system_name: "local"
  toolchain_identifier: "local_linux"

  feature {
    name: "c++11"
    flag_set {
      action: "c++-compile"
      flag_group {
        flag: "-std=c++11"
      }
    }
  }

  feature {
    name: "stdlib"
    flag_set {
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "-lstdc++"
      }
    }
  }

  feature {
    name: "determinism"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # Make C++ compilation deterministic. Use linkstamping instead of these
        # compiler symbols.
        flag: "-Wno-builtin-macro-redefined"
        flag: "-D__DATE__=\"redacted\""
        flag: "-D__TIMESTAMP__=\"redacted\""
        flag: "-D__TIME__=\"redacted\""
      }
    }
  }

  feature {
    name: "alwayslink"
    flag_set {
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      action: "c++-link-executable"
      flag_group {
        flag: "-Wl,-no-as-needed"
      }
    }
  }

  # This feature will be enabled for builds that support pic by bazel.
  feature {
    name: "pic"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        expand_if_all_available: "pic"
        flag: "-fPIC"
      }
      flag_group {
        expand_if_none_available: "pic"
        flag: "-fPIE"
      }
    }
  }

  # Security hardening on by default.
  feature {
    name: "hardening"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # Conservative choice; -D_FORTIFY_SOURCE=2 may be unsafe in some cases.
        # We need to undef it before redefining it as some distributions now
        # have it enabled by default.
        flag: "-U_FORTIFY_SOURCE"
        flag: "-D_FORTIFY_SOURCE=1"
        flag: "-fstack-protector"
      }
    }
    flag_set {
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "-Wl,-z,relro,-z,now"
      }
    }
    flag_set {
      action: "c++-link-executable"
      flag_group {
        flag: "-pie"
        flag: "-Wl,-z,relro,-z,now"
      }
    }
  }

  feature {
    name: "warnings"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # All warnings are enabled. Maybe enable -Werror as well?
        flag: "-Wall"
        %{host_compiler_warnings}
      }
    }
  }

  # Keep stack frames for debugging, even in opt mode.
  feature {
    name: "frame-pointer"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        flag: "-fno-omit-frame-pointer"
      }
    }
  }

  feature {
    name: "build-id"
    flag_set {
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        # Stamp the binary with a unique identifier.
        flag: "-Wl,--build-id=md5"
        flag: "-Wl,--hash-style=gnu"
      }
    }
  }

  feature {
    name: "no-canonical-prefixes"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "-no-canonical-prefixes"
        %{extra_no_canonical_prefixes_flags}
      }
    }
  }

  feature {
    name: "disable-assertions"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        flag: "-DNDEBUG"
      }
    }
  }

  feature {
    name: "linker-bin-path"

    flag_set {
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        %{linker_bin_path_flag}
      }
    }
  }

  feature {
    name: "common"
    implies: "stdlib"
    implies: "c++11"
    implies: "determinism"
    implies: "alwayslink"
    implies: "hardening"
    implies: "warnings"
    implies: "frame-pointer"
    implies: "build-id"
    implies: "no-canonical-prefixes"
    implies: "linker-bin-path"
  }

  feature {
    name: "opt"
    implies: "common"
    implies: "disable-assertions"

    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # No debug symbols.
        # Maybe we should enable https://gcc.gnu.org/wiki/DebugFission for opt
        # or even generally? However, that can't happen here, as it requires
        # special handling in Bazel.
        flag: "-g0"

        # Conservative choice for -O
        # -O3 can increase binary size and even slow down the resulting binaries.
        # Profile first and / or use FDO if you need better performance than this.
        flag: "-O2"

        # Removal of unused code and data at link time (can this increase binary size in some cases?).
        flag: "-ffunction-sections"
        flag: "-fdata-sections"
      }
    }
    flag_set {
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      action: "c++-link-executable"
      flag_group {
        flag: "-Wl,--gc-sections"
      }
    }
  }

  feature {
    name: "fastbuild"
    implies: "common"
  }

  feature {
    name: "dbg"
    implies: "common"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        flag: "-g"
      }
    }
  }

  # Set clang as a C/C++ compiler.
  tool_path { name: "gcc" path: "%{host_compiler_path}" }

  # Use the default system toolchain for everything else.
  tool_path { name: "ar" path: "/usr/bin/ar" }
  tool_path { name: "compat-ld" path: "/usr/bin/ld" }
  tool_path { name: "cpp" path: "/usr/bin/cpp" }
  tool_path { name: "dwp" path: "/usr/bin/dwp" }
  tool_path { name: "gcov" path: "/usr/bin/gcov" }
  tool_path { name: "ld" path: "/usr/bin/ld" }
  tool_path { name: "nm" path: "/usr/bin/nm" }
  tool_path { name: "objcopy" path: "/usr/bin/objcopy" }
  tool_path { name: "objdump" path: "/usr/bin/objdump" }
  tool_path { name: "strip" path: "/usr/bin/strip" }

  # Enabled dynamic linking.
  linking_mode_flags { mode: DYNAMIC }

%{host_compiler_includes}
}

toolchain {
  abi_version: "local"
  abi_libc_version: "local"
  compiler: "compiler"
  host_system_name: "local"
  needsPic: true
  target_libc: "macosx"
  target_cpu: "darwin"
  target_system_name: "local"
  toolchain_identifier: "local_darwin"
  feature {
    name: "c++11"
    flag_set {
      action: "c++-compile"
      flag_group {
        flag: "-std=c++11"
      }
    }
  }

  feature {
    name: "stdlib"
    flag_set {
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "-lc++"
      }
    }
  }

  feature {
    name: "determinism"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # Make C++ compilation deterministic. Use linkstamping instead of these
        # compiler symbols.
        flag: "-Wno-builtin-macro-redefined"
        flag: "-D__DATE__=\"redacted\""
        flag: "-D__TIMESTAMP__=\"redacted\""
        flag: "-D__TIME__=\"redacted\""
      }
    }
  }

  # This feature will be enabled for builds that support pic by bazel.
  feature {
    name: "pic"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        expand_if_all_available: "pic"
        flag: "-fPIC"
      }
      flag_group {
        expand_if_none_available: "pic"
        flag: "-fPIE"
      }
    }
  }

  # Security hardening on by default.
  feature {
    name: "hardening"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # Conservative choice; -D_FORTIFY_SOURCE=2 may be unsafe in some cases.
        # We need to undef it before redefining it as some distributions now
        # have it enabled by default.
        flag: "-U_FORTIFY_SOURCE"
        flag: "-D_FORTIFY_SOURCE=1"
        flag: "-fstack-protector"
      }
    }
    flag_set {
      action: "c++-link-executable"
      flag_group {
        flag: "-pie"
      }
    }
  }

  feature {
    name: "warnings"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # All warnings are enabled. Maybe enable -Werror as well?
        flag: "-Wall"
        %{host_compiler_warnings}
      }
    }
  }

  # Keep stack frames for debugging, even in opt mode.
  feature {
    name: "frame-pointer"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        flag: "-fno-omit-frame-pointer"
      }
    }
  }

  feature {
    name: "no-canonical-prefixes"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag:"-no-canonical-prefixes"
      }
    }
  }

  feature {
    name: "disable-assertions"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        flag: "-DNDEBUG"
      }
    }
  }

  feature {
    name: "linker-bin-path"

    flag_set {
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        %{linker_bin_path_flag}
      }
    }
  }

  feature {
    name: "undefined-dynamic"
    flag_set {
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      action: "c++-link-executable"
      flag_group {
        flag: "-undefined"
        flag: "dynamic_lookup"
      }
    }
  }

  feature {
    name: "common"
    implies: "stdlib"
    implies: "c++11"
    implies: "determinism"
    implies: "hardening"
    implies: "warnings"
    implies: "frame-pointer"
    implies: "no-canonical-prefixes"
    implies: "linker-bin-path"
    implies: "undefined-dynamic"
  }

  feature {
    name: "opt"
    implies: "common"
    implies: "disable-assertions"

    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        # No debug symbols.
        # Maybe we should enable https://gcc.gnu.org/wiki/DebugFission for opt
        # or even generally? However, that can't happen here, as it requires
        # special handling in Bazel.
        flag: "-g0"

        # Conservative choice for -O
        # -O3 can increase binary size and even slow down the resulting binaries.
        # Profile first and / or use FDO if you need better performance than this.
        flag: "-O2"

        # Removal of unused code and data at link time (can this increase binary size in some cases?).
        flag: "-ffunction-sections"
        flag: "-fdata-sections"
      }
    }
  }

  feature {
    name: "fastbuild"
    implies: "common"
  }

  feature {
    name: "dbg"
    implies: "common"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      flag_group {
        flag: "-g"
      }
    }
  }

  # Set clang as a C/C++ compiler.
  tool_path { name: "gcc" path: "%{host_compiler_path}" }

  # Use the default system toolchain for everything else.
  tool_path { name: "ar" path: "/usr/bin/libtool" }
  tool_path { name: "compat-ld" path: "/usr/bin/ld" }
  tool_path { name: "cpp" path: "/usr/bin/cpp" }
  tool_path { name: "dwp" path: "/usr/bin/dwp" }
  tool_path { name: "gcov" path: "/usr/bin/gcov" }
  tool_path { name: "ld" path: "/usr/bin/ld" }
  tool_path { name: "nm" path: "/usr/bin/nm" }
  tool_path { name: "objcopy" path: "/usr/bin/objcopy" }
  tool_path { name: "objdump" path: "/usr/bin/objdump" }
  tool_path { name: "strip" path: "/usr/bin/strip" }

  # Enabled dynamic linking.
  linking_mode_flags { mode: DYNAMIC }

%{host_compiler_includes}
}

toolchain {
  toolchain_identifier: "local_windows"
  host_system_name: "local"
  target_system_name: "local"

  abi_version: "local"
  abi_libc_version: "local"
  target_cpu: "x64_windows"
  compiler: "msvc-cl"
  target_libc: "msvcrt"

%{cxx_builtin_include_directory}

  tool_path {
    name: "ar"
    path: "%{msvc_lib_path}"
  }
  tool_path {
    name: "ml"
    path: "%{msvc_ml_path}"
  }
  tool_path {
    name: "cpp"
    path: "%{msvc_cl_path}"
  }
  tool_path {
    name: "gcc"
    path: "%{msvc_cl_path}"
  }
  tool_path {
    name: "gcov"
    path: "wrapper/bin/msvc_nop.bat"
  }
  tool_path {
    name: "ld"
    path: "%{msvc_link_path}"
  }
  tool_path {
    name: "nm"
    path: "wrapper/bin/msvc_nop.bat"
  }
  tool_path {
    name: "objcopy"
    path: "wrapper/bin/msvc_nop.bat"
  }
  tool_path {
    name: "objdump"
    path: "wrapper/bin/msvc_nop.bat"
  }
  tool_path {
    name: "strip"
    path: "wrapper/bin/msvc_nop.bat"
  }
  supports_interface_shared_objects: true

  # TODO(pcloudy): Review those flags below, they should be defined by cl.exe
  compiler_flag: "/DCOMPILER_MSVC"

  # Don't define min/max macros in windows.h.
  compiler_flag: "/DNOMINMAX"

  # Platform defines.
  compiler_flag: "/D_WIN32_WINNT=0x0600"
  # Turn off warning messages.
  compiler_flag: "/D_CRT_SECURE_NO_DEPRECATE"
  compiler_flag: "/D_CRT_SECURE_NO_WARNINGS"
  compiler_flag: "/D_SILENCE_STDEXT_HASH_DEPRECATION_WARNINGS"

  # Useful options to have on for compilation.
  # Increase the capacity of object files to 2^32 sections.
  compiler_flag: "/bigobj"
  # Allocate 500MB for precomputed headers.
  compiler_flag: "/Zm500"
  # Use unsigned char by default.
  compiler_flag: "/J"
  # Use function level linking.
  compiler_flag: "/Gy"
  # Use string pooling.
  compiler_flag: "/GF"
  # Catch C++ exceptions only and tell the compiler to assume that functions declared
  # as extern "C" never throw a C++ exception.
  compiler_flag: "/EHsc"

  # Globally disabled warnings.
  # Don't warn about elements of array being be default initialized.
  compiler_flag: "/wd4351"
  # Don't warn about no matching delete found.
  compiler_flag: "/wd4291"
  # Don't warn about diamond inheritance patterns.
  compiler_flag: "/wd4250"
  # Don't warn about insecure functions (e.g. non _s functions).
  compiler_flag: "/wd4996"

  linker_flag: "/MACHINE:X64"

  feature {
    name: "no_legacy_features"
  }

  # TODO(klimek): Previously we were using a .bat file to start python to run
  # the python script that can redirect to nvcc - unfortunately .bat files
  # have a rather short maximum length for command lines (8k). Instead, we
  # now use the python binary as the compiler and pass the python script to
  # it at the start of the command line. Investigate different possibilities
  # to run the nvcc wrapper, either using pyinstaller --onefile, or writing
  # a small C++ wrapper to redirect.
  feature {
    name: "redirector"
    enabled: true
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      action: "c++-module-compile"
      action: "c++-module-codegen"
      action: "c++-header-parsing"
      action: "assemble"
      action: "preprocess-assemble"
      flag_group {
        flag: "-B"
        flag: "external/local_config_cuda/crosstool/windows/msvc_wrapper_for_nvcc.py"
      }
    }
  }

  # Suppress startup banner.
  feature {
    name: "nologo"
    flag_set {
      action: "c-compile"
      action: "c++-compile"
      action: "c++-module-compile"
      action: "c++-module-codegen"
      action: "c++-header-parsing"
      action: "assemble"
      action: "preprocess-assemble"
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      action: "c++-link-static-library"
      flag_group {
        flag: "/nologo"
      }
    }
  }

  feature {
    name: 'has_configured_linker_path'
  }

  # This feature indicates strip is not supported, building stripped binary will just result a copy of orignial binary
  feature {
    name: 'no_stripping'
  }

  # This feature indicates this is a toolchain targeting Windows.
  feature {
    name: 'targets_windows'
    implies: 'copy_dynamic_libraries_to_binary'
    enabled: true
  }

  feature {
    name: 'copy_dynamic_libraries_to_binary'
  }

  action_config {
    config_name: 'assemble'
    action_name: 'assemble'
    tool {
      tool_path: '%{msvc_ml_path}'
    }
    implies: 'compiler_input_flags'
    implies: 'compiler_output_flags'
    implies: 'nologo'
    implies: 'msvc_env'
    implies: 'sysroot'
  }

  action_config {
    config_name: 'preprocess-assemble'
    action_name: 'preprocess-assemble'
    tool {
      tool_path: '%{msvc_ml_path}'
    }
    implies: 'compiler_input_flags'
    implies: 'compiler_output_flags'
    implies: 'nologo'
    implies: 'msvc_env'
    implies: 'sysroot'
  }

  action_config {
    config_name: 'c-compile'
    action_name: 'c-compile'
    tool {
      tool_path: '%{msvc_cl_path}'
    }
    implies: 'compiler_input_flags'
    implies: 'compiler_output_flags'
    implies: 'legacy_compile_flags'
    implies: 'nologo'
    implies: 'msvc_env'
    implies: 'parse_showincludes'
    implies: 'user_compile_flags'
    implies: 'sysroot'
    implies: 'unfiltered_compile_flags'
  }

  action_config {
    config_name: 'c++-compile'
    action_name: 'c++-compile'
    tool {
      tool_path: '%{msvc_cl_path}'
    }
    implies: 'compiler_input_flags'
    implies: 'compiler_output_flags'
    implies: 'legacy_compile_flags'
    implies: 'nologo'
    implies: 'msvc_env'
    implies: 'parse_showincludes'
    implies: 'user_compile_flags'
    implies: 'sysroot'
    implies: 'unfiltered_compile_flags'
  }

  action_config {
    config_name: 'c++-link-executable'
    action_name: 'c++-link-executable'
    tool {
      tool_path: '%{msvc_link_path}'
    }
    implies: 'nologo'
    implies: 'linkstamps'
    implies: 'output_execpath_flags'
    implies: 'input_param_flags'
    implies: 'user_link_flags'
    implies: 'legacy_link_flags'
    implies: 'linker_subsystem_flag'
    implies: 'linker_param_file'
    implies: 'msvc_env'
    implies: 'no_stripping'
  }

  action_config {
    config_name: 'c++-link-dynamic-library'
    action_name: 'c++-link-dynamic-library'
    tool {
      tool_path: '%{msvc_link_path}'
    }
    implies: 'nologo'
    implies: 'shared_flag'
    implies: 'linkstamps'
    implies: 'output_execpath_flags'
    implies: 'input_param_flags'
    implies: 'user_link_flags'
    implies: 'legacy_link_flags'
    implies: 'linker_subsystem_flag'
    implies: 'linker_param_file'
    implies: 'msvc_env'
    implies: 'no_stripping'
    implies: 'has_configured_linker_path'
    implies: 'def_file'
  }

  action_config {
      config_name: 'c++-link-nodeps-dynamic-library'
      action_name: 'c++-link-nodeps-dynamic-library'
      tool {
        tool_path: '%{msvc_link_path}'
      }
      implies: 'nologo'
      implies: 'shared_flag'
      implies: 'linkstamps'
      implies: 'output_execpath_flags'
      implies: 'input_param_flags'
      implies: 'user_link_flags'
      implies: 'legacy_link_flags'
      implies: 'linker_subsystem_flag'
      implies: 'linker_param_file'
      implies: 'msvc_env'
      implies: 'no_stripping'
      implies: 'has_configured_linker_path'
      implies: 'def_file'
    }

  action_config {
    config_name: 'c++-link-static-library'
    action_name: 'c++-link-static-library'
    tool {
      tool_path: '%{msvc_lib_path}'
    }
    implies: 'nologo'
    implies: 'archiver_flags'
    implies: 'input_param_flags'
    implies: 'linker_param_file'
    implies: 'msvc_env'
  }

  # TODO(b/65151735): Remove legacy_compile_flags feature when legacy fields are
  # not used in this crosstool
  feature {
    name: 'legacy_compile_flags'
    flag_set {
      expand_if_all_available: 'legacy_compile_flags'
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      action: 'c++-module-codegen'
      flag_group {
        iterate_over: 'legacy_compile_flags'
        flag: '%{legacy_compile_flags}'
      }
    }
  }

  feature {
    name: "msvc_env"
    env_set {
      action: "c-compile"
      action: "c++-compile"
      action: "c++-module-compile"
      action: "c++-module-codegen"
      action: "c++-header-parsing"
      action: "assemble"
      action: "preprocess-assemble"
      action: "c++-link-executable"
      action: "c++-link-dynamic-library"
      action: "c++-link-nodeps-dynamic-library"
      action: "c++-link-static-library"
      env_entry {
        key: "PATH"
        value: "%{msvc_env_path}"
      }
      env_entry {
        key: "INCLUDE"
        value: "%{msvc_env_include}"
      }
      env_entry {
        key: "LIB"
        value: "%{msvc_env_lib}"
      }
      env_entry {
        key: "TMP"
        value: "%{msvc_env_tmp}"
      }
      env_entry {
        key: "TEMP"
        value: "%{msvc_env_tmp}"
      }
    }
  }

  feature {
    name: 'include_paths'
    flag_set {
      action: "assemble"
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      flag_group {
        iterate_over: 'quote_include_paths'
        flag: '/I%{quote_include_paths}'
      }
      flag_group {
        iterate_over: 'include_paths'
        flag: '/I%{include_paths}'
      }
      flag_group {
        iterate_over: 'system_include_paths'
        flag: '/I%{system_include_paths}'
      }
    }
  }

  feature {
    name: "preprocessor_defines"
    flag_set {
      action: "assemble"
      action: "preprocess-assemble"
      action: "c-compile"
      action: "c++-compile"
      action: "c++-header-parsing"
      action: "c++-module-compile"
      flag_group {
        flag: "/D%{preprocessor_defines}"
        iterate_over: "preprocessor_defines"
      }
    }
  }

  # Tell Bazel to parse the output of /showIncludes
  feature {
    name: 'parse_showincludes'
    flag_set {
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-module-compile'
      action: 'c++-header-parsing'
      flag_group {
        flag: "/showIncludes"
      }
    }
  }


  feature {
    name: 'generate_pdb_file'
    requires: {
      feature: 'dbg'
    }
    requires: {
      feature: 'fastbuild'
    }
  }

  feature {
    name: 'shared_flag'
    flag_set {
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: '/DLL'
      }
    }
  }

  feature {
    name: 'linkstamps'
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      expand_if_all_available: 'linkstamp_paths'
      flag_group {
        iterate_over: 'linkstamp_paths'
        flag: '%{linkstamp_paths}'
      }
    }
  }

  feature {
    name: 'output_execpath_flags'
    flag_set {
      expand_if_all_available: 'output_execpath'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: '/OUT:%{output_execpath}'
      }
    }
  }

  feature {
    name: 'archiver_flags'
    flag_set {
      expand_if_all_available: 'output_execpath'
      action: 'c++-link-static-library'
      flag_group {
        flag: '/OUT:%{output_execpath}'
      }
    }
  }

  feature {
    name: 'input_param_flags'
    flag_set {
      expand_if_all_available: 'interface_library_output_path'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/IMPLIB:%{interface_library_output_path}"
      }
    }
    flag_set {
      expand_if_all_available: 'libopts'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        iterate_over: 'libopts'
        flag: '%{libopts}'
      }
    }
    flag_set {
      expand_if_all_available: 'libraries_to_link'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      action: 'c++-link-static-library'
      flag_group {
        iterate_over: 'libraries_to_link'
        flag_group {
          expand_if_equal: {
            variable: 'libraries_to_link.type'
            value: 'object_file_group'
          }
          iterate_over: 'libraries_to_link.object_files'
          flag_group {
            flag: '%{libraries_to_link.object_files}'
          }
        }
        flag_group {
          expand_if_equal: {
            variable: 'libraries_to_link.type'
            value: 'object_file'
          }
          flag_group {
            flag: '%{libraries_to_link.name}'
          }
        }
        flag_group {
          expand_if_equal: {
            variable: 'libraries_to_link.type'
            value: 'interface_library'
          }
          flag_group {
            flag: '%{libraries_to_link.name}'
          }
        }
        flag_group {
          expand_if_equal: {
            variable: 'libraries_to_link.type'
            value: 'static_library'
          }
          flag_group {
            expand_if_false: 'libraries_to_link.is_whole_archive'
            flag: '%{libraries_to_link.name}'
          }
          flag_group {
            expand_if_true: 'libraries_to_link.is_whole_archive'
            flag: '/WHOLEARCHIVE:%{libraries_to_link.name}'
          }
        }
      }
    }
  }

  # Since this feature is declared earlier in the CROSSTOOL than
  # "user_link_flags", this feature will be applied prior to it anwyhere they
  # are both implied. And since "user_link_flags" contains the linkopts from
  # the build rule, this allows the user to override the /SUBSYSTEM in the BUILD
  # file.
  feature {
    name: 'linker_subsystem_flag'
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: '/SUBSYSTEM:CONSOLE'
      }
    }
  }

  # The "user_link_flags" contains user-defined linkopts (from build rules)
  # so it should be defined after features that declare user-overridable flags.
  # For example the "linker_subsystem_flag" defines a default "/SUBSYSTEM" flag
  # but we want to let the user override it, therefore "link_flag_subsystem" is
  # defined earlier in the CROSSTOOL file than "user_link_flags".
  feature {
    name: 'user_link_flags'
    flag_set {
      expand_if_all_available: 'user_link_flags'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        iterate_over: 'user_link_flags'
        flag: '%{user_link_flags}'
      }
    }
  }
  feature {
    name: 'legacy_link_flags'
    flag_set {
      expand_if_all_available: 'legacy_link_flags'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        iterate_over: 'legacy_link_flags'
        flag: '%{legacy_link_flags}'
      }
    }
  }

  feature {
    name: 'linker_param_file'
    flag_set {
      expand_if_all_available: 'linker_param_file'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      action: 'c++-link-static-library'
      flag_group {
        flag: '@%{linker_param_file}'
      }
    }
  }

  feature {
    name: 'static_link_msvcrt'
  }

  feature {
    name: 'static_link_msvcrt_no_debug'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/MT"
      }
    }
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEFAULTLIB:libcmt.lib"
      }
    }
    requires: { feature: 'fastbuild'}
    requires: { feature: 'opt'}
  }

  feature {
    name: 'dynamic_link_msvcrt_no_debug'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/MD"
      }
    }
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEFAULTLIB:msvcrt.lib"
      }
    }
    requires: { feature: 'fastbuild'}
    requires: { feature: 'opt'}
  }

  feature {
    name: 'static_link_msvcrt_debug'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/MTd"
      }
    }
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEFAULTLIB:libcmtd.lib"
      }
    }
    requires: { feature: 'dbg'}
  }

  feature {
    name: 'dynamic_link_msvcrt_debug'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/MDd"
      }
    }
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEFAULTLIB:msvcrtd.lib"
      }
    }
    requires: { feature: 'dbg'}
  }

  feature {
    name: 'dbg'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/Od"
        flag: "/Z7"
        flag: "/DDEBUG"
      }
    }
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEBUG:FULL"
        flag: "/INCREMENTAL:NO"
      }
    }
    implies: 'generate_pdb_file'
  }

  feature {
    name: 'fastbuild'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/Od"
        flag: "/Z7"
        flag: "/DDEBUG"
      }
    }
    flag_set {
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEBUG:FASTLINK"
        flag: "/INCREMENTAL:NO"
      }
    }
    implies: 'generate_pdb_file'
  }

  feature {
    name: 'opt'
    flag_set {
      action: 'c-compile'
      action: 'c++-compile'
      flag_group {
        flag: "/O2"
        flag: "/DNDEBUG"
      }
    }
  }

  feature {
    name: 'user_compile_flags'
    flag_set {
      expand_if_all_available: 'user_compile_flags'
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      action: 'c++-module-codegen'
      flag_group {
        iterate_over: 'user_compile_flags'
        flag: '%{user_compile_flags}'
      }
    }
  }

  feature {
    name: 'sysroot'
    flag_set {
      expand_if_all_available: 'sysroot'
      action: 'assemble'
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      action: 'c++-module-codegen'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        iterate_over: 'sysroot'
        flag: '--sysroot=%{sysroot}'
      }
    }
  }

  feature {
    name: 'unfiltered_compile_flags'
    flag_set {
      expand_if_all_available: 'unfiltered_compile_flags'
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      action: 'c++-module-codegen'
      flag_group {
        iterate_over: 'unfiltered_compile_flags'
        flag: '%{unfiltered_compile_flags}'
      }
    }
  }

  feature {
    name: 'compiler_output_flags'
    flag_set {
      action: 'assemble'
      flag_group {
        expand_if_all_available: 'output_file'
        expand_if_none_available: 'output_assembly_file'
        expand_if_none_available: 'output_preprocess_file'
        flag: '/Fo%{output_file}'
        flag: '/Zi'
      }
    }
    flag_set {
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      action: 'c++-module-codegen'
      flag_group {
        expand_if_all_available: 'output_file'
        expand_if_none_available: 'output_assembly_file'
        expand_if_none_available: 'output_preprocess_file'
        flag: '/Fo%{output_file}'
      }
      flag_group {
        expand_if_all_available: 'output_file'
        expand_if_all_available: 'output_assembly_file'
        flag: '/Fa%{output_file}'
      }
      flag_group {
        expand_if_all_available: 'output_file'
        expand_if_all_available: 'output_preprocess_file'
        flag: '/P'
        flag: '/Fi%{output_file}'
      }
    }
  }

  feature {
    name: 'compiler_input_flags'
    flag_set {
      action: 'assemble'
      action: 'preprocess-assemble'
      action: 'c-compile'
      action: 'c++-compile'
      action: 'c++-header-parsing'
      action: 'c++-module-compile'
      action: 'c++-module-codegen'
      flag_group {
        expand_if_all_available: 'source_file'
        flag: '/c'
        flag: '%{source_file}'
      }
    }
  }

  feature {
    name : 'def_file',
    flag_set {
      expand_if_all_available: 'def_file_path'
      action: 'c++-link-executable'
      action: 'c++-link-dynamic-library'
      action: "c++-link-nodeps-dynamic-library"
      flag_group {
        flag: "/DEF:%{def_file_path}"
        # We can specify a different DLL name in DEF file, /ignore:4070 suppresses
        # the warning message about DLL name doesn't match the default one.
        # See https://msdn.microsoft.com/en-us/library/sfkk2fz7.aspx
        flag: "/ignore:4070"
      }
    }
  }

  feature {
    name: 'windows_export_all_symbols'
  }

  feature {
    name: 'no_windows_export_all_symbols'
  }

  linking_mode_flags { mode: DYNAMIC }
}
