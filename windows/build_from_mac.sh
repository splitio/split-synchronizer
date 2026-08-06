#!/usr/bin/env bash

set -e

docker build -t sync_fips_win_builder -f ./macos_builder.Dockerfile .
docker run --rm -v $(dirname $(pwd)):/buildenv sync_fips_win_builder 
