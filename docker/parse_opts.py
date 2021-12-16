"""
Split Sync & Proxy CLI options parser

To generate split-synchronizer CLI option list
`python parse_opts.py -f splitio/common/conf/sections.go,splitio/producer/conf/sections.go`

To generate split-synchronizer CLI option list
`python parse_opts.py -f splitio/common/conf/sections.go,splitio/producer/conf/sections.go`

This file is meant to be used from the Makefile to generate docker entry points
"""

import argparse
import re
from typing import Optional, List, Tuple
 

_CLI_REGEX = 's-cli:"([^"]*)" '


def main():
    """Execute."""
    parser = argparse.ArgumentParser(add_help=True, usage='python parse_opts.py a/sections.go b/sections.go')
    parser.add_argument('-f', '--files', help='files to process', required=True)
    args = parser.parse_args()

    opts = [
        item for fn in args.files.split(',')
        for item in parse_section_file(fn) 
    ]

    print(' '.join(map(lambda o: f'"{o}"', opts)))

def parse_line(line: str) -> Optional[Tuple[str, str, str]]:
    """Parse a `sections.go` line and return either a config option or None."""
    cli_search = re.search(_CLI_REGEX, line)
    return cli_search.group(1) if cli_search else None

def parse_section_file(fn: str) -> List[str]:
    """Parse a `sections.go` file and return markdown formatted table in a string."""
    with open(fn, 'r') as f:
        lines = f.read().split('\n')
    return [item for item in (parse_line(line) for line in lines) if item is not None]

if __name__ == '__main__':
    main()
