#!/usr/bin/env python3
# *_* coding: utf-8 *_*

"""Generate Download page for split-sync & proxy."""

import logging
import pathlib
import subprocess
import re
import sys

# The following regex allows semver with or without the `v` prefix
# and filters out any odd suffix (including RCs)
_VALID_TAG = re.compile(r'^v{0,1}\d{1,2}\.\d{1,2}\.\d{1,2}$')
_LOGGER = logging.getLogger(__file__)

def main():
    logging.basicConfig(level=logging.INFO)
    basepath = pathlib.Path(__file__).parent.resolve()
    with open(f"{basepath}/versions.pre.html", 'r') as flo: tpl_pre = flo.read()
    with open(f"{basepath}/versions.pos.html", 'r') as flo: tpl_post = flo.read()
    with open(f"{basepath}/versions.download-row.html", 'r') as flo: tpl_row = flo.read()

    try:
        tags = sorted([
            tag.replace('v', '') for tag in
            subprocess.check_output(['git', 'tag', '-l']).decode('utf-8').split('\n')
            if tag and re.match(_VALID_TAG, tag)
        ])
    except:
        _LOGGER.error('Error parsing tags', exc_info=True)
        sys.exit(1)
      
    print(tpl_pre)
    for tag in tags:
        print(tpl_row.replace('{{VERSION}}', tag))
    print(tpl_post)


if __name__ == '__main__':
    main()
