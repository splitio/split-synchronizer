#!/bin/bash

# Environment vars:
#
#   Common vars:
#    - SPLIT_SYNC_API_KEY                      Split service API-KEY grabbed from webconsole
#    - SPLIT_SYNC_SPLITS_REFRESH_RATE          Refresh rate of splits fetcher
#    - SPLIT_SYNC_SEGMENTS_REFRESH_RATE        Refresh rate of segments fetcher
#    - SPLIT_SYNC_IMPRESSIONS_REFRESH_RATE     Refresh rate of impressions recorder
#    - SPLIT_SYNC_METRICS_REFRESH_RATE         Refresh rate of metrics recorder
#    - SPLIT_SYNC_HTTP_TIMEOUT                 Timeout specifies a time limit for requests
#    - SPLIT_SYNC_LOG_DEBUG                    Enable debug mode: Set as 'on'
#    - SPLIT_SYNC_LOG_VERBOSE                  Enable verbose mode: Set as 'on'
#    - SPLIT_SYNC_LOG_STDOUT                   Enable standard output: Set as 'on'
#    - SPLIT_SYNC_LOG_FILE                     Set the log file
#    - SPLIT_SYNC_LOG_FILE_MAX_SIZE            Max file log size in bytes
#    - SPLIT_SYNC_LOG_BACKUP_COUNT             Number of last log files to keep in filesystem
#    - SPLIT_SYNC_LOG_SLACK_CHANNEL            Set the Slack channel or user
#    - SPLIT_SYNC_LOG_SLACK_WEBHOOK            Set the Slack webhook url
#
#    - SPLIT_SYNC_ADVANCED_PARAMETERS          Set custom parameters that are not configured via provided Env vars.
#                                              Sample:
#                                                SPLIT_SYNC_ADVANCED_PARAMETERS="-redis-read-timeout=20 -redis-max-retries=10"
#    - SPLIT_SYNC_IMPRESSION_LISTENER_ENDPOINT Custom user HTTP Endpoint where impressions will be posted.
#
#   Proxy vars:
#    - SPLIT_SYNC_PROXY                        Enables the proxy mode: Set as 'on'
#    - SPLIT_SYNC_PROXY_SDK_APIKEYS            List of custom API-KEYs for your SDKs (Comma separated string)
#    - SPLIT_SYNC_PROXY_ADMIN_USER             HTTP basic auth username for admin endpoints
#    - SPLIT_SYNC_PROXY_ADMIN_PASS             HTTP basic auth password for admin endpoints
#    - SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE   Max size, in bytes, to send impressions in proxy mode
#
#   Producer vars:
#    - SPLIT_SYNC_REDIS_HOST                   Redis server hostname
#    - SPLIT_SYNC_REDIS_PORT                   Redis Server port
#    - SPLIT_SYNC_REDIS_DB                     Redis DB number
#    - SPLIT_SYNC_REDIS_PASS                   Redis password
#    - SPLIT_SYNC_REDIS_PREFIX                 Redis key prefix
#    - SPLIT_SYNC_IMPRESSIONS_PER_POST         Number of impressions to send in a POST request
#    - SPLIT_SYNC_IMPRESSIONS_THREADS          Number of impressions recorder threads

# COMMON PARAMETERS
PARAMETERS="-api-key=${SPLIT_SYNC_API_KEY}"

if [ ! -z ${SPLIT_SYNC_SPLITS_REFRESH_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -split-refresh-rate=${SPLIT_SYNC_SPLITS_REFRESH_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_SEGMENTS_REFRESH_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -segment-refresh-rate=${SPLIT_SYNC_SEGMENTS_REFRESH_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_IMPRESSIONS_REFRESH_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -impressions-post-rate=${SPLIT_SYNC_IMPRESSIONS_REFRESH_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_METRICS_REFRESH_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -metrics-post-rate=${SPLIT_SYNC_METRICS_REFRESH_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_HTTP_TIMEOUT+x} ]; then
  PARAMETERS="${PARAMETERS} -http-timeout=${SPLIT_SYNC_HTTP_TIMEOUT}"
fi

if [ "$SPLIT_SYNC_LOG_DEBUG" = "on" ]; then
  PARAMETERS="${PARAMETERS} -log-debug"
fi

if [ "$SPLIT_SYNC_LOG_VERBOSE" = "on" ]; then
  PARAMETERS="${PARAMETERS} -log-verbose"
fi

if [ "$SPLIT_SYNC_LOG_STDOUT" = "on" ]; then
  PARAMETERS="${PARAMETERS} -log-stdout"
fi

if [ ! -z ${SPLIT_SYNC_LOG_FILE+x} ]; then
  PARAMETERS="${PARAMETERS} -log-file=${SPLIT_SYNC_LOG_FILE}"
fi

if [ ! -z ${SPLIT_SYNC_LOG_FILE_MAX_SIZE+x} ]; then
  PARAMETERS="${PARAMETERS} -log-file-max-size=${SPLIT_SYNC_LOG_FILE_MAX_SIZE}"
fi

if [ ! -z ${SPLIT_SYNC_LOG_BACKUP_COUNT+x} ]; then
  PARAMETERS="${PARAMETERS} -log-file-backup-count=${SPLIT_SYNC_LOG_BACKUP_COUNT}"
fi

if [ ! -z ${SPLIT_SYNC_LOG_SLACK_CHANNEL+x} ]; then
  PARAMETERS="${PARAMETERS} -log-slack-channel=${SPLIT_SYNC_LOG_SLACK_CHANNEL}"
fi

if [ ! -z ${SPLIT_SYNC_LOG_SLACK_WEBHOOK+x} ]; then
  PARAMETERS="${PARAMETERS} -log-slack-webhook-url=${SPLIT_SYNC_LOG_SLACK_WEBHOOK}"
fi

if [ ! -z ${SPLIT_SYNC_IMPRESSION_LISTENER_ENDPOINT+x} ]; then
  echo "HAY IMPRESSION LISTENER!"
  PARAMETERS="${PARAMETERS} -impression-listener-endpoint=${SPLIT_SYNC_IMPRESSION_LISTENER_ENDPOINT}"
fi


# PROXY MODE ON
if [ "$SPLIT_SYNC_PROXY" = "on" ];
then
  echo "Running in PROXY mode"
  PARAMETERS="${PARAMETERS} -proxy"

  if [ ! -z ${SPLIT_SYNC_PROXY_SDK_APIKEYS+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-apikeys=${SPLIT_SYNC_PROXY_SDK_APIKEYS}"
  fi

  if [ ! -z ${SPLIT_SYNC_PROXY_ADMIN_USER+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-admin-username=${SPLIT_SYNC_PROXY_ADMIN_USER}"
  fi

  if [ ! -z ${SPLIT_SYNC_PROXY_ADMIN_PASS+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-admin-password=${SPLIT_SYNC_PROXY_ADMIN_PASS}"
  fi

  if [ ! -z ${SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-impressions-max-size=${SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE}"
  fi

#PRODUCER MODE ON
else
  echo "Running in PRODUCER mode"

  if [ ! -z ${SPLIT_SYNC_REDIS_HOST+x} ]; then
    PARAMETERS="${PARAMETERS} -redis-host=${SPLIT_SYNC_REDIS_HOST}"
  fi

  if [ ! -z ${SPLIT_SYNC_REDIS_PORT+x} ]; then
    PARAMETERS="${PARAMETERS} -redis-port=${SPLIT_SYNC_REDIS_PORT}"
  fi

  if [ ! -z ${SPLIT_SYNC_REDIS_DB+x} ]; then
    PARAMETERS="${PARAMETERS} -redis-db=${SPLIT_SYNC_REDIS_DB}"
  fi

  if [ ! -z ${SPLIT_SYNC_REDIS_PASS+x} ]; then
    PARAMETERS="${PARAMETERS} -redis-pass=${SPLIT_SYNC_REDIS_PASS}"
  fi

  if [ ! -z ${SPLIT_SYNC_REDIS_PREFIX+x} ]; then
    PARAMETERS="${PARAMETERS} -redis-prefix=${SPLIT_SYNC_REDIS_PREFIX}"
  fi

  if [ ! -z ${SPLIT_SYNC_IMPRESSIONS_PER_POST+x} ]; then
    PARAMETERS="${PARAMETERS} -impressions-per-post=${SPLIT_SYNC_IMPRESSIONS_PER_POST}"
  fi

  if [ ! -z ${SPLIT_SYNC_IMPRESSIONS_THREADS+x} ]; then
    PARAMETERS="${PARAMETERS} -impressions-recorder-threads=${SPLIT_SYNC_IMPRESSIONS_THREADS}"
  fi

fi

if [ ! -z ${SPLIT_SYNC_ADVANCED_PARAMETERS+x} ]; then
  PARAMETERS="${PARAMETERS} ${SPLIT_SYNC_ADVANCED_PARAMETERS}"
fi

exec split-sync ${PARAMETERS}
