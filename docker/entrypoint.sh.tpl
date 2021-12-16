#!/usr/bin/env bash

FLAGS=({{ARGS}})

source functions.sh
cli_args=$(parse_env {{PREFIX}} "${FLAGS[@]}")
exec {{EXECUTABLE}} $cli_args
