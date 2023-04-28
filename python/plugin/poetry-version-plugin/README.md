# Poetry Version Plugin

<a href="https://github.com/tiangolo/poetry-version-plugin/actions?query=workflow%3ATest" target="_blank">
    <img src="https://github.com/tiangolo/poetry-version-plugin/workflows/Test/badge.svg" alt="Test">
</a>
<a href="https://github.com/tiangolo/poetry-version-plugin/actions?query=workflow%3APublish" target="_blank">
    <img src="https://github.com/tiangolo/poetry-version-plugin/workflows/Publish/badge.svg" alt="Publish">
</a>
<a href="https://codecov.io/gh/tiangolo/poetry-version-plugin" target="_blank">
    <img src="https://img.shields.io/codecov/c/github/tiangolo/poetry-version-plugin?color=%2334D058" alt="Coverage">
</a>
<a href="https://pypi.org/project/poetry-version-plugin" target="_blank">
    <img src="https://img.shields.io/pypi/v/poetry-version-plugin?color=%2334D058&label=pypi%20package" alt="Package version">
</a>

A [**Poetry**](https://python-poetry.org/) plugin for dynamically extracting the package **version**.

It can read the version from a file `__init__.py` with:

```python
# __init__.py

__version__ = "0.1.0"
```

Alternatively, it can read it from a **git tag**, set with a GitHub release or with:

```console
$ git tag 0.1.0
```

🚨 Consider this in the alpha stage. Read the warning below.

## When to use

This is useful mainly if you are building a package library for others to use and you want to set the version in a place different than `pyproject.toml`, but you still want to keep a [single source of truth](https://en.wikipedia.org/wiki/Single_source_of_truth).

It won't be helpful in other use cases like managing local app environments with Poetry.

## How to use

Make sure you have Poetry version `1.2.0a1` or above. Read below for instructions to install it if you haven't.

### Install Poetry Version Plugin

Install this plugin to your Poetry:

```console
$ poetry self add poetry-version-plugin

--> 100%
```

### Set version in init file

Set your package version in your file `__init__.py`, for example:

```python
from .main import do_awesome_stuff, AwesomeClass

__version__ = "0.2.3"
```

And then edit your `pyproject.toml` with a section containing:

```toml
[tool.poetry-version-plugin]
source = "init"
```

Next, build your project. It will show an output like:

```console
$ poetry build
Using __init__.py file at my_awesome_package/__init__.py for dynamic version
Setting package dynamic version to __version__ variable from __init__.py: 0.1.9
Building my-awesome-package (0.1.9)
  - Building sdist
  - Built my-awesome-package-0.1.9.tar.gz
  - Building wheel
  - Built my-awesome-package-0.1.9-py3-none-any.whl
```

### Set the version in a Git tag

Alternatively, to extract the version to use from a Git tag, add a section:

```toml
[tool.poetry-version-plugin]
source = "git-tag"
```

Then create a git tag, for example:

```console
$ git tag 0.1.3
```

In this case, when building your project, it will show an output like:

```console
$ poetry build
Git tag found, setting dynamic version to: 0.1.3
Building my-awesome-package (0.1.3)
  - Building sdist
  - Built my-awesome-package-0.1.3.tar.gz
  - Building wheel
  - Built my-awesome-package-0.1.3-py3-none-any.whl
```

## Version in `pyproject.toml`

Currently (2021-05-24) Poetry requires a `version` configuration in the `pyproject.toml`, even if you use this plugin.

When using this plugin, that `version` config won't be used, but Poetry still requires it to be present in the `pyproject.toml`.

To make it more evident that you are not using that `version` you can set it to `0`.

```toml
[tool.poetry]
name = "my-awesome-package"
version = "0"
```

That way, you will more easily notice if the plugin is not installed, as it will show that you are building a package with version `0` instead of the dynamic version set.

## An example `pyproject.toml`

A short, minimal example `pyproject.toml` could look like:

```toml
[tool.poetry]
name = "my-awesome-package"
version = "0"
description = ""
authors = ["Rick Sanchez <rick@rick-citadel.com>"]
readme = "README.md"

[tool.poetry.dependencies]
python = "^3.6"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"

[tool.poetry-version-plugin]
source = "init"
```

## Why

By default, Poetry expects you to set your package version in `pyproject.toml`. And that would work in most cases.

But imagine you want to expose the version of your package in a `__version__` variable so that your users can do things like:

```python
import my_awesome_package
print(my_awesome_package.__version__)
```

You could manually write the `__version__` variable and handle the synchronization between it and the `pyproject.toml` yourself, which is very **error-prone**.

The current [official way of doing it without duplicating the value](https://github.com/python-poetry/poetry/pull/2366#issuecomment-652418094) is with `importlib.metadata`.

But that module is only available in Python 3.8 and above. So, for Python 3.7 and 3.6 you have to install a backport as a dependency of your package:

```toml
[tool.poetry.dependencies]
importlib-metadata = {version = "^1.0", python = "<3.8"}
```

But then, when they release each new version of the backport (currently `4.0.1`), you have to update it (or not). And your users would have to manually handle conflicts with any other packages that also depend on `importlib-metadata` which could be multiple, as many packages could be doing the same trick (I've dealt with that).

The other option is not to pin any version range of your `importlib-metadata` in your `pyproject.toml` and hope for the best.

And then your `__init__.py` would have to include code using it, like:

```python
# I don't want this extra complexity 😔
# And it doesn't work in Docker 🐋
try:
    import importlib.metadata as importlib_metadata
except ModuleNotFoundError:
    import importlib_metadata

__version__ = importlib_metadata.version(__name__)
```

But that code is extra complexity and logic needed in your code, in each of your packages.

🚨 Additionally, this only works when your package is installed in a Python environment. It won't work if, for example, you simply put your code in a **container**, which is common for web apps and distributed systems.

### How this plugin solves it

With this plugin, your package doesn't depend on `importlib-metadata`, so your users won't need to handle conflicts or extra dependencies.

Instead, your build system (Poetry) is what needs to have this plugin installed.

That avoids the extra code complexity on your side, dependency conflicts for your users, and support for other use cases like code copied directly inside a container.

### Version from Git tag

Alternatively, this plugin can also extract the version from a Git tag.

So, you could only create each version in a Git tag (for example, a GitHub release) instead of writing it in code.

And then build the package on Continuous Integration (e.g. GitHub Actions). And this plugin would get the version of the package from that Git tag.

## Install Poetry `1.2.0a1`

For this plugin to work, you need Poetry version `1.2.0a1` or above.

[Poetry `1.2.0a1` was released recently](https://python-poetry.org/blog/announcing-poetry-1-2-0a1.html).

There's a high chance you already have installed Poetry `1.1.x`.

The first step is to uninstall it:

```console
$ curl -sSL https://raw.githubusercontent.com/python-poetry/poetry/master/get-poetry.py -O
--> 100%

$ python get-poetry.py --uninstall
--> 100%
```

And then install the new Poetry with the new installer:

```console
$ curl -sSL https://raw.githubusercontent.com/python-poetry/poetry/master/install-poetry.py -O
--> 100%

$ python install-poetry.py --preview
--> 100%
```

🔍 Notice that the new installer file is named `install-poetry.py` instead of `get-poetry.py`. Also, notice that, currently, you need to set `--preview` for it to install the alpha version `1.2.0a1`.

You can check that it worked with:

```console
$ poetry --version
Poetry (version 1.2.0a1)
```

## Support for version in init file

When using a `__version__` variable in your `__init__.py` you can have more logic in that file, import modules, and do more things above and below the declaration of that variable.

But the value has to be a literal string, like:

```python
___version___ = "0.2.0"
```

...instead of calling a function or something similar.

And the variable has to be at the top-level, so it can't be inside an `if` statement or similar.

This is all fine and supported in your `__init__.py`:

```python
# __init__.py

# This is all valid 👍✅

from .main import do_awesome_stuff, AwesomeClass

awesome = AwesomeClass()

# Some comment explaining why this is commented out
# __version__ = "1.0.0"

__version__ = "0.2.3"

if __name__ == "__main__":
    awesome.run()
```

This example is all valid and supported, and it includes:

* Imports
* Other objects and variables
* Comments
* The same string `__version__` inside a comment
* If blocks around

---

But this is not supported:

```python
# 🚨 Not supported

if 2 == 2:
    __version__ = "0.1.0
```

And this is not supported:

```python
# 🚨 Not supported

def get_version():
    return "0.2.0"

__version__ = get_version()
```

## How the plugin works

Poetry runs the plugin when building a package, and it sets the version right before creating the "package distributable" (e.g., the wheel).

### How the version variable works

If you have a package (a single package) declared in the `packages` config in your `pyproject.toml`, the plugin will use that package's `__init__.py` to find the `__version__` variable.

If you don't have any `packages` config, the plugin will assume that you have a single package named as your project, but in the module version (changing `-` for `_`). So, if your package is `my-awesome-project`, the plugin will use the file at `my_awesome_project/__init__.py` to find the `__version__` variable.

This file structure is the default if you create a new project with the command `poetry new`, so it should work as expected. ✨

The way the plugin works internally is by parsing the `__init__.py` file. Reading the Python's "Abstract Syntax Tree" using the `ast` standard module and extracting the literal value of the string. So, it doesn't execute the code in `__init__.py`, it only reads it as Python code.

The plugin doesn't try to import and execute that `__init__.py` file because that could require extra computation, external dependencies, etc. And it doesn't try to extract the `__version__` with regular expressions, as that would be prone to errors if, for example, there was some other `__version__` somewhere in the code, in a comment or inside a string.

## Warning

🚨 Consider this in the alpha stage. Poetry `1.2.0a1` with support for plugins was released on 2021-05-21. I started writing this plugin 3 days later, on 2021-05-24.

Things might break in Poetry or in this plugin. So, please try it and test it very carefully before fully adopting it for delicate systems.

The way it works might change, and the specific configuration might change.

Also, if you don't find the following sections intuitive:

```toml
[tool.poetry-version-plugin]
source = "init"
```

and

```toml
[tool.poetry-version-plugin]
source = "git-tag"
```

let me know what alternative configuration would make more sense and be more intuitive to you.

👍 The good news is, assuming you are building packages to then upload them to PyPI for your users to download and use them, the **worst that could happen** if something broke is that you wouldn't be able to build a new version until something is fixed or changed. But your users shouldn't be affected in any way.

## Release Notes

### Latest Changes

* ✏️ Fix typos and rewording in README.md. PR [#8](https://github.com/tiangolo/poetry-version-plugin/pull/8) by [@Gl0deanR](https://github.com/Gl0deanR).
### 0.1.3

* ✨ Improve logs, prefix with plugin name. PR [#6](https://github.com/tiangolo/poetry-version-plugin/pull/6) by [@tiangolo](https://github.com/tiangolo).
* 🔧 Update pyproject metadata. PR [#5](https://github.com/tiangolo/poetry-version-plugin/pull/5) by [@tiangolo](https://github.com/tiangolo).
* ✅ Fix coverage. PR [#4](https://github.com/tiangolo/poetry-version-plugin/pull/4) by [@tiangolo](https://github.com/tiangolo).
* 📝 Improve docs. PR [#3](https://github.com/tiangolo/poetry-version-plugin/pull/3) by [@tiangolo](https://github.com/tiangolo).
* 🐛 Fix tests for CI. PR [#1](https://github.com/tiangolo/poetry-version-plugin/pull/1) by [@tiangolo](https://github.com/tiangolo).
* 👷 Fix Latest Changes action, set branch to main. PR [#2](https://github.com/tiangolo/poetry-version-plugin/pull/2) by [@tiangolo](https://github.com/tiangolo).

## License

This project is licensed under the terms of the MIT license.
