#!/bin/env sh

FLAGS=(
# Proxy CLI ARGS
    "apikey"
    "ip-address-enabled"
    "timeout-ms"
    "snapshot"
    "force-fresh-startup"
    "client-apikeys"
    "server-host"
    "server-port"
    "http-cache-size"
    "persistent-storage-fn"
    "split-refresh-rate-ms"
    "segment-refresh-rate-ms"
    "streaming-enabled"
    "http-timeout-ms"
    "impressions-buffer-size"
    "events-buffer-size"
    "telemetry-buffer-size"
    "impressions-workers"
    "events-workers"
    "telemetry-workers"
    "internal-metrics-rate-ms"
    "dependencies-check-rate-ms"

# Common CLI ARGS
    "log-level"
    "log-output"
    "log-rotation-max-files"
    "log-rotation-max-size-kb"
    "admin-host"
    "admin-port"
    "admin-username"
    "admin-password"
    "admin-secure-hc"
    "impression-listener-endpoint"
    "impression-listener-queue-size"
    "slack-webhook"
    "slack-channel"
)

source functions.sh
cli_args=$(parse_env "SPLIT_PROXY" "${FLAGS[@]}")
echo $cli_args
split-proxy $cli_args
