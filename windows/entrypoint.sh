#!/usr/bin/env bash

set -e

cd buildenv/windows
make setup_ms_go binaries
