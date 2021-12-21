#!/usr/bin/env python3
# *_* coding: utf-8 *_*

"""
Generate Download pages for split-sync & proxy.

This script properly handles the evolution in versioning and executables of the repo.

- Starting with version 4.0.0, tags are prefixed with a `v`, which is not present in the final executables
- Starting with version 5.0.0, we have 2 different binaries `split-sync` and `split-proxy`
"""

import logging
import pathlib
import subprocess
import re
import sys
from argparse import ArgumentParser
from typing import List, Dict
from distutils.version import StrictVersion

# regex to filter rcs/betas/etc
_VALID_TAG = re.compile(r'^v{0,1}\d{1,2}\.\d{1,2}\.\d{1,2}$')

# milestone versions
_FIRST_MODULES_VERSION = StrictVersion('4.0.0')
_FIRST_MULTIEXEC_VERSION = StrictVersion('5.0.0')

_LOGGER = logging.getLogger(__file__)

# Example release files (as they'll be updated to S3)
#
# build
# ├── 5.0.0
# │   ├── install_split_proxy_linux_5.0.0.bin
# │   ├── install_split_proxy_osx_5.0.0.bin
# │   ├── install_split_sync_linux_5.0.0.bin
# │   ├── install_split_sync_osx_5.0.0.bin
# │   ├── split_proxy_windows_5.0.0.zip
# │   └── split_sync_windows_5.0.0.zip
# ├── install_split_proxy_linux.bin
# ├── install_split_proxy_osx.bin
# ├── install_split_sync_linux.bin
# ├── install_split_sync_osx.bin
# ├── split_proxy_windows.zip
# └── split_sync_windows.zip

_SYNC_PRE_VARS = {
    'title': 'Split Sync Download Page',
    'description': 'Download latest version of split-sync. A background service to synchronize Split information with your SDK',
    'dockerhub_url': 'https://hub.docker.com/r/splitsoftware/split-synchronizer/',
    'latest_linux': 'install_split_sync_linux.bin',
    'latest_osx': 'install_split_sync_osx.bin',
    'latest_windows': 'split_sync_windows.zip',
}

_PROXY_PRE_VARS = {
    'title': 'Split Proxy Download Page',
    'description': ('Download latest version of split-proxy. A background service that mimics our BE to deploy in your own infra.\n'
                    'Prior to version 5.0.0, the split-synchronizer & proxy were a single app. Those versions can be found in the '
                    'Split-Synchronizer download page.'),
    'dockerhub_url': 'https://hub.docker.com/r/splitsoftware/split-proxy/',
    'latest_linux': 'install_split_proxy_linux.bin',
    'latest_osx': 'install_split_proxy_osx.bin',
    'latest_windows': 'split_proxy_windows.zip',
}

def make_row_vars_pre_multiexec(version: str) -> Dict[str,str]:
    """Format template variables for a post-multiexec version."""
    return {
        'version': version,
        'old_file_osx': f"./{version}/install_osx_{version}.bin",
        'old_file_linux': f"./{version}/install_linux_{version}.bin",
        'old_file_windows': f"./{version}/split-sync-win_{version}.zip",
    }

def make_row_vars_post_multiexec(app: str, version: str) -> Dict[str,str]:
    """Format template variables for a post-multiexec version."""
    return {
        'version': version,
        'old_file_osx': f"./{version}/install_split_{app}_osx_{version}.bin",
        'old_file_linux': f"./{version}/install_split_{app}_linux_{version}.bin",
        'old_file_windows': f"./{version}/split_{app}_windows_{version}.zip",
    }

def get_tags() -> List[str]:
    """Fetch all the tags, format them appropriately and build comparable version objects."""
    return sorted([
        tag.replace('v', '') for tag in
        subprocess.check_output(['git', 'tag', '-l']).decode('utf-8').split('\n')
        if tag and re.match(_VALID_TAG, tag)
    ], reverse=True)


def parse_args() -> object:
    """Parse CLI arguments and return an object indicating presence/absence of args & it's values."""
    parser = ArgumentParser()
    parser.add_argument('-a', '--app', type=str, required=True, help='App to build download page for [sync|proxy]')
    return parser.parse_args()


def main():
    args = parse_args()
    logging.basicConfig(level=logging.INFO)
    basepath = pathlib.Path(__file__).parent.resolve()
    with open(f"{basepath}/versions.css.tpl", 'r') as flo: style = flo.read()
    with open(f"{basepath}/versions.pre.html.tpl", 'r') as flo: tpl_pre = flo.read()
    with open(f"{basepath}/versions.pos.html.tpl", 'r') as flo: tpl_post = flo.read()
    with open(f"{basepath}/versions.download-row.html.tpl", 'r') as flo: tpl_row = flo.read()

    tags = get_tags()

    if args.app == 'sync':
        print(tpl_pre.format(**_SYNC_PRE_VARS,style=style))
        for tag in filter(lambda v: StrictVersion(v) >= _FIRST_MULTIEXEC_VERSION, tags):
            print(tpl_row.format(**make_row_vars_post_multiexec('sync', tag)))
        for tag in filter(lambda v: StrictVersion(v) < _FIRST_MULTIEXEC_VERSION, tags):
            print(tpl_row.format(**make_row_vars_pre_multiexec(tag)))
        print(tpl_post)
    elif args.app == 'proxy':
        print(tpl_pre.format(**_PROXY_PRE_VARS,style=style))
        for tag in filter(lambda v: StrictVersion(v) >= _FIRST_MULTIEXEC_VERSION, tags):
            print(tpl_row.format(**make_row_vars_post_multiexec('proxy', tag)))
        print(tpl_post)
    else:
        _LOGGER.error(f'Unknown app {args.app}: must be "sync" or "proxy"')
        sys.exit(1)


if __name__ == '__main__':
    main()
