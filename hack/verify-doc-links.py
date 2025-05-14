#!/usr/bin/env python3

# Copyright 2022 The KServe Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.#

# This script finds MD-style links and URLs in markdown file and verifies
# that the referenced resources exist.

import concurrent.futures
import itertools
import re
from datetime import datetime, timedelta
from glob import glob
from os import environ as env
from os.path import abspath, dirname, exists, relpath
from time import sleep
from urllib.request import Request, urlopen
from urllib.parse import urlparse
from urllib.error import URLError, HTTPError


GITHUB_REPO = env.get("GITHUB_REPO", "https://github.com/kserve/kserve/")
BRANCH = "master"

# glob expressions to find markdown files in project
md_file_path_expressions = [
    "/**/*.md",
    "/.github/**/*.md",
]

# don't process .md files in excluded folders
excluded_paths = [
    "/node_modules/",
    "/temp/",
    "/.venv/",
]

# don't analyze URLs with variables to be replaced by the user
url_excludes = ["<", ">", "$", "{", "}"]

# also exclude non-public or local URLs in a <host>:<port> style
url_excludes.extend(
    [
        "0.0.0.0",
        ":80",
        ":90",
        ":port",
        ":predict",
        ".default",
        "blob.core.windows.net",
        "customdomain.com",
        "example.com",
        "localhost",
        "somecluster",
        "sslip.io",
        "svc.cluster.local",
        "xip.io",
    ]
)

# GitHub rate-limiting is 60 requests per minute, then we sleep a bit
parallel_requests = 60  # use no more than 60 parallel requests
retry_wait = 60  # 1 minute before retrying GitHub requests after 429 error
extra_wait = 5  # additional wait time before retrying GitHub requests

script_folder = abspath(dirname(__file__))
project_root_dir = abspath(dirname(script_folder))
github_repo_master_path = "{}/blob/{}".format(GITHUB_REPO.rstrip("/"), BRANCH)
url_status_cache = dict()


def find_md_files() -> [str]:

    list_of_lists = [
        glob(project_root_dir + path_expr, recursive=True)
        for path_expr in md_file_path_expressions
    ]

    flattened_list = list(itertools.chain(*list_of_lists))

    filtered_list = [
        path for path in flattened_list if not any(s in path for s in excluded_paths)
    ]

    return sorted(filtered_list)


def get_links_from_md_file(
    md_file_path: str,
) -> [(int, str, str)]:  # -> [(line, link_text, URL)]

    with open(md_file_path, "r") as f:
        try:
            md_file_content = f.read()
        except ValueError as e:
            print(f"Error trying to load file {md_file_path}")
            raise e

    folder = relpath(dirname(md_file_path), project_root_dir)

    # replace relative links that are siblings to the README, i.e. [link text](FEATURES.md)
    md_file_content = re.sub(
        r"\[([^]]+)\]\((?!http|#|/)([^)]+)\)",
        r"[\1]({}/{}/\2)".format(github_repo_master_path, folder).replace("/./", "/"),
        md_file_content,
    )

    # replace links that are relative to the project root, i.e. [link text](/sdk/FEATURES.md)
    md_file_content = re.sub(
        r"\[([^]]+)\]\(/([^)]+)\)",
        r"[\1]({}/\2)".format(github_repo_master_path),
        md_file_content,
    )

    # find all the links
    line_text_url = []
    for line_number, line_text in enumerate(md_file_content.splitlines()):

        all_urls_in_this_line = set()

        # find markdown-styled links [text](url)
        for link_text, url in re.findall(
            r"\[([^][]+)\]\((%s[^)]+)\)" % "http", line_text
        ):
            if not any(s in url for s in url_excludes):
                line_text_url.append((line_number + 1, link_text, url))
                all_urls_in_this_line.add(url)

        # find plain http(s)-style links
        for url in re.findall(r"https?://[a-zA-Z0-9./?=_&%${}<>:-]+", line_text):
            if url not in all_urls_in_this_line and not any(
                s in url for s in url_excludes
            ):
                try:
                    urlparse(url)
                    line_text_url.append((line_number + 1, "", url.strip(".")))
                except URLError:
                    pass

    # return completed links
    return line_text_url


def test_url(
    file: str, line: int, text: str, url: str
) -> (str, int, str, str, int):  # (file, line, text, url, status)

    short_url = url.split("#", maxsplit=1)[0]
    status = 0

    if short_url not in url_status_cache:

        # mind GitHub rate-limiting, use local files to verify link
        if short_url.startswith(github_repo_master_path):
            local_path = short_url.replace(github_repo_master_path, "")
            if exists(abspath(project_root_dir + local_path)):
                status = 200
            else:
                status = 404
        else:
            status = request_url(short_url, method="HEAD")

        if status == 403:  # forbidden, try with web browser header
            headers = {
                "User-Agent": "Mozilla/5.0",  # most pages want User-Agent
                "Accept-Encoding": "gzip, deflate, br",  # GitHub wants Accept-Encoding
            }
            status = request_url(short_url, method="GET", headers=headers)

        if status == 405:  # method not allowed, use GET instead of HEAD
            status = request_url(short_url, method="GET")

        if status in [
            429,
            503,
        ]:  # GitHub rate-limiting or service unavailable, try again after 1 minute
            sleep(retry_wait + extra_wait)
            status = request_url(short_url, method="GET")

        if status in [
            444,
            555,
        ]:  # other URLError or Exception, retry with longer timeout
            status = request_url(short_url, method="GET", timeout=15)

        # if we keep getting the same error, mark it as 404 to be reported at the end
        if status in [444, 555]:
            status = 404

        url_status_cache[short_url] = status

    status = url_status_cache[short_url]

    return file, line, text, url, status


# datetime to wait until before attempting requests to github.com
next_time_for_github_request = datetime.now()


def wait_before_retry(url):
    if "github.com" in url:
        if datetime.now() < next_time_for_github_request:
            sleep((next_time_for_github_request - datetime.now()).seconds + extra_wait)


def set_retry_time(url, status):
    global next_time_for_github_request
    if "github.com" in url and status == 429:
        next_time_for_github_request = datetime.now() + timedelta(
            seconds=retry_wait + extra_wait
        )


def request_url(url, method="HEAD", timeout=5, headers={}) -> int:
    wait_before_retry(url)
    try:
        req = Request(url, method=method, headers=headers)
        resp = urlopen(req, timeout=timeout)
        status = resp.code
        if any(s in url for s in ["youtube.com", "youtu.be"]):
            html = resp.read().decode("utf8")
            if "This video isn't available anymore" in html:
                status = 404
        resp.close()
    except HTTPError as e:
        status = e.code
        set_retry_time(url, status)
    except URLError:
        status = 444  # custom code used script-internally
    except Exception:
        status = 555  # custom code used script-internally

    return status


def verify_urls_concurrently(
    file_line_text_url: [(str, int, str, str)],
) -> [(str, int, str, str)]:
    file_line_text_url_status = []

    with concurrent.futures.ThreadPoolExecutor(
        max_workers=parallel_requests
    ) as executor:
        check_urls = (
            executor.submit(test_url, file, line, text, url)
            for (file, line, text, url) in file_line_text_url
        )
        for url_check in concurrent.futures.as_completed(check_urls):
            try:
                file, line, text, url, status = url_check.result()
                file_line_text_url_status.append((file, line, text, url, status))
            except Exception:
                # set 555 status as a custom code used script-internally
                file_line_text_url_status.append((file, line, text, url, 555))
            finally:
                print(
                    "{}/{}".format(
                        len(file_line_text_url_status), len(file_line_text_url)
                    ),
                    end="\r",
                )

    return file_line_text_url_status


def verify_doc_links() -> [(str, int, str, str)]:

    # 1. find all relevant Markdown files
    md_file_paths = find_md_files()

    # 2. extract all links with text and URL
    file_line_text_url = [
        (file, line, text, url)
        for file in md_file_paths
        for (line, text, url) in get_links_from_md_file(file)
    ]

    # 3. validate the URLs
    file_line_text_url_status = verify_urls_concurrently(file_line_text_url)

    # 4. filter for the invalid URLs (status 404: "Not Found") to be reported
    file_line_text_url_404 = [
        (f, l, t, u, s) for (f, l, t, u, s) in file_line_text_url_status if s == 404
    ]

    # 5. print some stats for confidence
    print(
        "{} {} links ({} unique URLs) in {} Markdown files.\n".format(
            "Checked" if file_line_text_url_404 else "Verified",
            len(file_line_text_url_status),
            len(url_status_cache),
            len(md_file_paths),
        )
    )

    # 6. report invalid links, exit with error for CI/CD
    if file_line_text_url_404:

        for file, line, text, url, status in sorted(file_line_text_url_404):
            print(
                "{}:{}: {} -> {}".format(
                    relpath(file, project_root_dir),
                    line,
                    url.replace(github_repo_master_path, ""),
                    status,
                )
            )

        # print a summary line for clear error discovery at the bottom of Travis job log
        print(
            "\nERROR: Found {} invalid Markdown links".format(
                len(file_line_text_url_404)
            )
        )

        exit(1)


def apply_monkey_patch_to_force_ipv4_connections():
    # Monkey-patch socket.getaddrinfo to force IPv4 conections, since some older
    # routers and some internet providers don't support IPv6, in which case Python
    # will first try an IPv6 connection which will hang until timeout and only
    # then attempt a successful IPv4 connection
    import socket

    # get a reference to the original getaddrinfo function
    getaddrinfo_original = socket.getaddrinfo

    # create a patched getaddrinfo function which uses the original function
    # but filters out IPv6 (socket.AF_INET6) entries of host and port address infos
    def getaddrinfo_patched(*args, **kwargs):
        res = getaddrinfo_original(*args, **kwargs)
        return [r for r in res if r[0] == socket.AF_INET]

    # replace the original socket.getaddrinfo function with our patched version
    socket.getaddrinfo = getaddrinfo_patched


if __name__ == "__main__":
    apply_monkey_patch_to_force_ipv4_connections()
    verify_doc_links()
