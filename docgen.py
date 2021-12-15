"""
Split Sync & Proxy config documentation generator.

To generate split-synchronizer documentation:
`python docgen.py -e SPLIT_SYNC -f splitio/common/conf/sections.go,splitio/producer/conf/sections.go`

To generate split-proxy documentation:
`python docgen.py -e SPLIT_PROXY -f splitio/common/conf/sections.go,splitio/proxy/conf/sections.go`

Ideally, paste the output in `https://markdownlivepreview.com/` and check that it's properly renderd. If not: FIX IT :)
"""

import argparse
import re
from typing import Optional, List, Tuple
 

_CLI_REGEX = 's-cli:"([^"]*)" '
_JSON_REGEX = 'json:"([^"]*)" '
_DESC_REGEX = 's-desc:"([^"]*)"'


def main():
    """Execute."""
    parser = argparse.ArgumentParser(add_help=True, usage='python docgen.py a/sections.go b/sections.go')
    parser.add_argument('-e', '--env-prefix', help='environment variable prefix', required=True)
    parser.add_argument('-f', '--files', help='files to process', required=True)
    args = parser.parse_args()

    header = ['| **Command line option** | **JSON option** | **Environment variable** (container-only) | **Description** |',
              '| --- | --- | --- | --- |']

    opts_by_file = [parse_section_file(args.env_prefix, fn) for fn in args.files.split(',')]

    print('\n'.join(header + [line for flines in opts_by_file for line in flines]))


def parse_line(line: str) -> Optional[Tuple[str, str, str]]:
    """Parse a `sections.go` line and return either a config option or None."""
    cli_search = re.search(_CLI_REGEX, line)
    json_search = re.search(_JSON_REGEX, line)
    desc_search = re.search(_DESC_REGEX, line)


    return (cli_search.group(1), json_search.group(1), desc_search.group(1)) \
        if cli_search and json_search and desc_search else None

#   for debugging use this:
#   return (cli_search.group(1) if cli_search  else None,
#          json_search.group(1) if json_search else None,
#          desc_search.group(1) if desc_search else None)


def cli_to_env(prefix: str, cli: str) -> str:
    """Build the environment variable for a specific cli option."""
    return prefix + '_' + cli.upper().replace('-', '_')


def parse_section_file(prefix: str, fn: str) -> List[str]:
    """Parse a `sections.go` file and return markdown formatted table in a string."""
    with open(fn, 'r') as f:
        lines = f.read().replace('|', '&#124;').split('\n') # be sure to remove pipes

    return [
        '| %s | %s | %s | %s |' % (item[0], item[1], cli_to_env(prefix, item[0]), item[2])
        for item in (parse_line(line) for line in lines)
        if item is not None
    ]


if __name__ == '__main__':
    main()
