# Copyright 2025 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#    http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# ---
# Portions of this software are derived from:

# - pip-licenses (https://github.com/raimon49/pip-licenses)
#   Licensed under the MIT License:

#   Copyright (c) 2018 raimon
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

import argparse
import importlib.metadata
import os
import requests
import logging
from typing import Dict, List, Optional

try:
    import tomllib  # Python 3.11+
except ImportError:
    import tomli as tomllib

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
)

logger = logging.getLogger("piplicenses")


def read_pyproject_config(pyproject_path: str) -> Dict[str, str]:
    path = pyproject_path + "/pyproject.toml"
    if not os.path.exists(path):
        logger.info(f"No pyproject.toml found at: {pyproject_path}")
        return {}
    try:
        with open(path, "rb") as f:
            toml_data = tomllib.load(f)
        return {
            k.replace("-", "_"): v
            for k, v in toml_data.get("tool", {}).get("pip-licenses", {}).items()
        }
    except Exception as e:
        logger.error(f"Error reading pyproject.toml at {pyproject_path}: {e}")
        return {}


def parse_arguments(toml_config: Dict[str, str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate a list of installed Python packages and their license information."
    )
    parser.add_argument(
        "--from",
        dest="license_source",
        choices=["meta", "classifier", "mixed"],
        default=toml_config.get("from", "mixed"),
    )
    parser.add_argument(
        "--format",
        dest="output_format",
        choices=["plain-vertical"],
        default=toml_config.get("format", "plain-vertical"),
    )
    parser.add_argument(
        "--with-license-file",
        action="store_true",
        default=toml_config.get("with_license_file", False),
    )
    parser.add_argument(
        "--with-notice-file",
        action="store_true",
        default=toml_config.get("with_notice_file", False),
    )
    parser.add_argument(
        "--with-urls", action="store_true", default=toml_config.get("with_urls", False)
    )
    parser.add_argument(
        "--output-path", type=str, default=toml_config.get("output_path", ".")
    )
    parser.add_argument(
        "--ignore-packages",
        type=str,
        nargs="+",
        action="store",
        default=toml_config.get("ignore_packages", []),
        help="Space-separated list of packages to ignore for license check, but include license text.",
    )
    parser.add_argument(
        "--allow-only", type=str, default=toml_config.get("allow_only", None)
    )
    parser.add_argument(
        "--package-url",
        type=str,
        default=None,
        help="Specify packages and URLs to retrieve licenses from. Format: 'pkg_name:url [pkg_name2:url2 ...]'",
    )

    return parser.parse_known_args()[0]


def get_license(pkg: importlib.metadata.Distribution, source: str) -> str:
    if source == "classifier":
        for classifier in pkg.metadata.get_all("Classifier", []):
            if classifier.startswith("License ::"):
                license_name = classifier.split("::")[-1].strip()
                if license_name != "OSI Approved":
                    return license_name
    elif source == "meta":
        if "License-Expression" in pkg.metadata:
            return pkg.metadata["License-Expression"]
        else:
            return pkg.metadata.get("License", "UNKNOWN")
    elif source == "mixed":
        return get_license(pkg, "classifier") or get_license(pkg, "meta")
    return "UNKNOWN"


def get_installed_packages() -> List[importlib.metadata.Distribution]:
    return [pkg for pkg in importlib.metadata.distributions()]


def find_file_content(
    pkg: importlib.metadata.Distribution, filenames: List[str]
) -> Optional[str]:
    try:
        package_path = pkg.locate_file("")
        for root, _, files in os.walk(package_path):
            for name in files:
                if name.upper() in filenames:
                    with open(
                        os.path.join(root, name), encoding="utf-8", errors="ignore"
                    ) as f:
                        return f.read()
    except Exception as e:
        logger.error(
            f"Error reading local license/notice for {pkg.metadata.get('Name')}: {e}"
        )
    return None


def fetch_remote_file_from_repo(url: str, filenames: List[str]) -> Optional[str]:
    if "github.com" not in url:
        return None

    url = url.rstrip("/")
    if "tree" in url:
        url = url.replace("/tree", "")

    repo_base = url.replace("github.com", "raw.githubusercontent.com")

    for branch in ["main", "master"]:
        base = f"{repo_base}/{branch}"
        for name in filenames:
            try:
                raw_url = f"{base}/{name}"
                response = requests.get(raw_url, timeout=3)
                if response.ok and response.text.strip():
                    return response.text.strip()
            except Exception as e:
                logger.error(f"Failed to fetch {name} from {raw_url}: {e}")

    return None


def fetch_license_name_from_text(content: str) -> Optional[str]:
    license_name = "UNKNOWN"
    content_lines: list[str] = content.splitlines()
    # Get the license name from the first line of license text or the second line
    # if the first line is empty or whitespace
    if len(content_lines) > 0:
        license_name = content_lines[0].strip()
        if license_name.isspace() or not license_name:
            license_name = (
                content_lines[1].strip() if len(content_lines) > 1 else "UNKNOWN"
            )
    return license_name


def check_license_allowlist(
    license_info: str,
    allow_only: Optional[List[str]],
    pkg_name: str,
    ignore_list: List[str],
) -> None:
    if pkg_name not in ignore_list and (
        allow_only is None or license_info not in allow_only
    ):
        raise ValueError(
            f"License '{license_info}' is not in the allowed list found for package {pkg_name}"
        )


def format_plain_vertical(
    packages: List[importlib.metadata.Distribution],
    package_urls: Dict[str, str],
    license_source: str,
    with_license_file: bool,
    with_notice_file: bool,
    with_urls: bool,
    allow_only: Optional[List[str]],
    ignore_list: Optional[List[str]] = None,
) -> tuple[str, str]:
    license_output = []
    notice_output = []
    for pkg in packages:
        name = pkg.metadata.get("Name", "")
        version = pkg.version
        license_info = get_license(pkg, license_source)
        # If license_info is a long string it is probably a full license text, fetch the license name from the text
        if len(license_info.splitlines()) > 2:
            license_info = fetch_license_name_from_text(license_info)
        url = pkg.metadata.get("Home-page") or pkg.metadata.get("Project-URL") or ""

        lines = [name, version, license_info]
        if with_urls:
            lines.append(url)

        # Header for license and notice sections
        header = f"{'='*120}\n{name.center(0)}   {version.center(35)}   {license_info.center(35)}"
        if not url:
            url = package_urls.get(name, "")
        if with_urls:
            header += f"   {url.center(35)}"

        header += f"\n{'='*120}"

        # Check if license is in allowlist
        if license_info != "UNKNOWN" and pkg.metadata.get("Name", "").lower() not in (
            name.lower() for name in ignore_list
        ):
            check_license_allowlist(
                license_info, allow_only, pkg.metadata.get("Name"), ignore_list
            )
        else:
            content = find_file_content(pkg, ["LICENSE", "LICENCE", "COPYING"])
            if not content and license_info == "UNKNOWN" and url:
                logger.info(
                    f"Fetching license for package {pkg.metadata.get('Name')} from {url}"
                )
                content = fetch_remote_file_from_repo(
                    url, ["LICENSE", "LICENCE", "COPYING"]
                )
                license_name = fetch_license_name_from_text(content)
                check_license_allowlist(
                    license_name, allow_only, pkg.metadata.get("Name"), ignore_list
                )

        # License file
        if with_license_file:
            content = find_file_content(pkg, ["LICENSE", "LICENCE", "COPYING"])
            if not content and license_info == "UNKNOWN" and url:
                content = fetch_remote_file_from_repo(
                    url, ["LICENSE", "LICENCE", "COPYING"]
                )
            if content:
                license_output.append(f"{header}\n{content}")

        # Notice file
        if with_notice_file:
            content = find_file_content(pkg, ["NOTICE"])
            if not content and license_info == "UNKNOWN" and url:
                content = fetch_remote_file_from_repo(url, ["NOTICE"])
            if content:
                notice_output.append(f"{header}\n{content}")

    return (
        "\n\n".join(license_output),
        "\n\n".join(notice_output),
    )


def handle_package_urls(args):
    parsed_package_urls = {}
    if not args.package_url:
        return parsed_package_urls

    # Process multiple packages - split by spaces
    package_urls = args.package_url.split()

    for package_url in package_urls:
        # Split each package entry using the format pkg_name:url
        if ":" not in package_url:
            raise ValueError(
                f"Invalid format for {package_url}. Expected 'pkg_name:url'"
            )

        pkg_name, pkg_url = package_url.split(":", 1)
        pkg_name = pkg_name.strip()
        pkg_url = pkg_url.strip()
        parsed_package_urls[pkg_name] = pkg_url

    return parsed_package_urls


def main():
    parser = argparse.ArgumentParser(
        description="Generate a list of installed Python packages and their license information."
    )
    parser.add_argument("--config-path", default=".", help="Path to pyproject.toml")
    args, _ = parser.parse_known_args()
    toml_config = read_pyproject_config(args.config_path)
    logger.info(toml_config)
    args = parse_arguments(toml_config)

    ignore_list = (
        [pkg.strip() for pkg in args.ignore_packages] if args.ignore_packages else []
    )
    allow_only = (
        [lic.strip() for lic in args.allow_only.split(";")] if args.allow_only else None
    )

    package_urls = handle_package_urls(args)
    packages = get_installed_packages()
    license_text, notice_text = format_plain_vertical(
        packages,
        package_urls,
        args.license_source or "mixed",
        args.with_license_file,
        args.with_notice_file,
        args.with_urls,
        allow_only,
        ignore_list,
    )

    if args.output_path:
        license_path = os.path.join(args.output_path, "LICENSE.txt")
        notice_path = os.path.join(args.output_path, "NOTICE.txt")

        if license_text:
            with open(license_path, "w", encoding="utf-8") as f:
                f.write(license_text)
        if notice_text:
            with open(notice_path, "w", encoding="utf-8") as f:
                f.write(notice_text)


if __name__ == "__main__":
    main()
