#!/bin/bash

# Environment vars:
#
#   Common vars:
#    - SPLIT_SYNC_API_KEY                      Split service API-KEY grabbed from webconsole
#    - SPLIT_SYNC_SPLITS_REFRESH_RATE          Refresh rate of splits fetcher
#    - SPLIT_SYNC_SEGMENTS_REFRESH_RATE        Refresh rate of segments fetcher
#    - SPLIT_SYNC_IMPRESSIONS_POST_RATE        Post rate of impressions recorder
#    - SPLIT_SYNC_EVENTS_POST_RATE             Post rate of events recorder
#    - SPLIT_SYNC_METRICS_POST_RATE            Post rate of metrics recorder
#    - SPLIT_SYNC_HTTP_TIMEOUT                 Timeout specifies a time limit for requests
#    - SPLIT_SYNC_LOG_DEBUG                    Enable debug mode: Set as 'on'
#    - SPLIT_SYNC_LOG_VERBOSE                  Enable verbose mode: Set as 'on'
#    - SPLIT_SYNC_LOG_STDOUT                   Enable standard output: Set as 'on'
#    - SPLIT_SYNC_LOG_FILE                     Set the log file
#    - SPLIT_SYNC_LOG_FILE_MAX_SIZE            Max file log size in bytes
#    - SPLIT_SYNC_LOG_BACKUP_COUNT             Number of last log files to keep in filesystem
#    - SPLIT_SYNC_LOG_SLACK_CHANNEL            Set the Slack channel or user
#    - SPLIT_SYNC_LOG_SLACK_WEBHOOK            Set the Slack webhook url
#    - SPLIT_SYNC_IP_ADDRESSES_ENABLED         Flag to disable IP addresses and host name from being sent to the Split backend
#    - SPLIT_SYNC_STREAMING_ENABLED            Flag to enable/disable streaming
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
#    - SPLIT_SYNC_PROXY_DASHBOARD_TITLE        Title to be shown in admin dashboard
#    - SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE   Max size, in bytes, to send impressions in proxy mode
#    - SPLIT_SYNC_PROXY_EVENTS_MAX_SIZE        Max size, in bytes, to send events in proxy mode
#
#   Producer vars:
#    - SPLIT_SYNC_REDIS_HOST                        Redis server hostname
#    - SPLIT_SYNC_REDIS_PORT                        Redis Server port
#    - SPLIT_SYNC_REDIS_DB                          Redis DB number
#    - SPLIT_SYNC_REDIS_PASS                        Redis password
#    - SPLIT_SYNC_REDIS_PREFIX                      Redis key prefix
#    - SPLIT_SYNC_IMPRESSIONS_PER_POST              Number of impressions to send in a POST request
#    - SPLIT_SYNC_IMPRESSIONS_THREADS               Number of impressions recorder threads
#    - SPLIT_SYNC_ADMIN_USER                        HTTP basic auth username for admin endpoints
#    - SPLIT_SYNC_ADMIN_PASS                        HTTP basic auth password for admin endpoints
#    - SPLIT_SYNC_DASHBOARD_TITLE                   Title to be shown in admin dashboard
#    - SPLIT_SYNC_EVENTS_PER_POST                   Number of events to send in a POST request
#    - SPLIT_SYNC_EVENTS_THREADS                    Number of events recorder threads
#    - SPLIT_SYNC_REDIS_SENTINEL_REPLICATION        Flag to signal that redis sentinel replication will be used
#    - SPLIT_SYNC_REDIS_SENTINEL_MASTER             Name of the master node of sentinel cluster
#    - SPLIT_SYNC_REDIS_SENTINEL_ADDRESSES          Comma-separated list of <HOST:PORT> addresses of redis sentinels
#    - SPLIT_SYNC_REDIS_CLUSTER_MODE                Flag to signal that redis cluster mode will be used
#    - SPLIT_SYNC_REDIS_CLUSTER_NODES               Comma-separated list of <HOST:PORT> nodes of redis cluster
#    - SPLIT_SYNC_REDIS_CLUSTER_KEYHASHTAG          String keyHashTag for redis cluster
#    - SPLIT_SYNC_REDIS_TLS                         Enable TLS Encryption for redis connections
#    - SPLIT_SYNC_REDIS_TLS_SERVER_NAME             Name of the redis server as it appears in the server certificate (defaults to the host)
#    - SPLIT_SYNC_REDIS_TLS_SKIP_NAME_VALIDATION    Don't check the server name in the received certificate
#    - SPLIT_SYNC_REDIS_TLS_CA_ROOT_CERTS           Comma-separated list of CA root certificate file names.
#    - SPLIT_SYNC_REDIS_TLS_CLIENT_KEY              Path to the client's PEM-encoded private key
#    - SPLIT_SYNC_REDIS_TLS_CLIENT_CERTIFICATE      Path to the client's certificate with a signed public key.
#    - SPLIT_SYNC_REDIS_FORCE_CLEANUP               Cleanup redis (DB and prefix only) before starting.


# Accepted values for options
is_true() {
    case $1 in
        TRUE|true|ON|on|YES|yes)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# COMMON PARAMETERS
PARAMETERS="-api-key=${SPLIT_SYNC_API_KEY}"

if [ ! -z ${SPLIT_SYNC_SPLITS_REFRESH_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -split-refresh-rate=${SPLIT_SYNC_SPLITS_REFRESH_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_SEGMENTS_REFRESH_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -segment-refresh-rate=${SPLIT_SYNC_SEGMENTS_REFRESH_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_IMPRESSIONS_POST_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -impressions-post-rate=${SPLIT_SYNC_IMPRESSIONS_POST_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_EVENTS_POST_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -events-post-rate=${SPLIT_SYNC_EVENTS_POST_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_METRICS_POST_RATE+x} ]; then
  PARAMETERS="${PARAMETERS} -metrics-post-rate=${SPLIT_SYNC_METRICS_POST_RATE}"
fi

if [ ! -z ${SPLIT_SYNC_HTTP_TIMEOUT+x} ]; then
  PARAMETERS="${PARAMETERS} -http-timeout=${SPLIT_SYNC_HTTP_TIMEOUT}"
fi

if is_true "$SPLIT_SYNC_LOG_DEBUG"; then
  PARAMETERS="${PARAMETERS} -log-debug"
fi

if is_true "$SPLIT_SYNC_LOG_VERBOSE"; then
  PARAMETERS="${PARAMETERS} -log-verbose"
fi

if is_true "$SPLIT_SYNC_LOG_STDOUT"; then
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
  PARAMETERS="${PARAMETERS} -impression-listener-endpoint=${SPLIT_SYNC_IMPRESSION_LISTENER_ENDPOINT}"
fi

if is_true "$SPLIT_SYNC_IP_ADDRESSES_ENABLED"; then
  PARAMETERS="${PARAMETERS} -ip-addresses-enabled"
fi

if is_true "$SPLIT_SYNC_STREAMING_ENABLED"; then
  PARAMETERS="${PARAMETERS} -streaming-enabled"
fi


# PROXY MODE ON
if is_true "$SPLIT_SYNC_PROXY";
then
  printf "Running in PROXY mode"
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

  if [ ! -z ${SPLIT_SYNC_PROXY_DASHBOARD_TITLE+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-dashboard-title=${SPLIT_SYNC_PROXY_DASHBOARD_TITLE}"
  fi

  if [ ! -z ${SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-impressions-max-size=${SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE}"
  fi

  if [ ! -z ${SPLIT_SYNC_PROXY_EVENTS_MAX_SIZE+x} ]; then
    PARAMETERS="${PARAMETERS} -proxy-events-max-size=${SPLIT_SYNC_PROXY_EVENTS_MAX_SIZE}"
  fi

#PRODUCER MODE ON
else
  printf "Running in PRODUCER mode"

  if is_true "$SPLIT_SYNC_REDIS_DISABLE_LEGACY_IMPRESSIONS"; then
    PARAMETERS="${PARAMETERS} -redis-disable-legacy-impressions"
  fi

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

  # redis sentinel config
  if is_true "$SPLIT_SYNC_REDIS_SENTINEL_REPLICATION"; then
    PARAMETERS="${PARAMETERS} -redis-sentinel-replication"
    if [ ! -z ${SPLIT_SYNC_REDIS_SENTINEL_MASTER+x} ]; then
      PARAMETERS="${PARAMETERS} -redis-sentinel-master=${SPLIT_SYNC_REDIS_SENTINEL_MASTER}"
    fi
    if [ ! -z ${SPLIT_SYNC_REDIS_SENTINEL_ADDRESSES+x} ]; then
      PARAMETERS="${PARAMETERS} -redis-sentinel-addresses=${SPLIT_SYNC_REDIS_SENTINEL_ADDRESSES}"
    fi
  fi

  # redis cluster config
  if is_true "$SPLIT_SYNC_REDIS_CLUSTER_MODE"; then
    PARAMETERS="${PARAMETERS} -redis-cluster-mode"
    if [ ! -z ${SPLIT_SYNC_REDIS_CLUSTER_NODES+x} ]; then
      PARAMETERS="${PARAMETERS} -redis-cluster-nodes=${SPLIT_SYNC_REDIS_CLUSTER_NODES}"
    fi
    if [ ! -z ${SPLIT_SYNC_REDIS_CLUSTER_KEYHASHTAG+x} ]; then
      PARAMETERS="${PARAMETERS} -redis-cluster-key-hashtag=${SPLIT_SYNC_REDIS_CLUSTER_KEYHASHTAG}"
    fi
  fi

  # TLS specific config
  if is_true "$SPLIT_SYNC_REDIS_TLS"; then
    PARAMETERS="${PARAMETERS} -redis-tls"

    if [ ! -z ${SPLIT_SYNC_REDIS_TLS_SERVER_NAME+x} ]; then
        PARAMETERS="${PARAMETERS} -redis-tls-server-name ${SPLIT_SYNC_REDIS_TLS_SERVER_NAME}"
    fi

    if is_true "$SPLIT_SYNC_REDIS_TLS_SKIP_NAME_VALIDATION"; then
        PARAMETERS="${PARAMETERS} -redis-tls-skip-name-validation"
    fi

    if [ ! -z ${SPLIT_SYNC_REDIS_TLS_CA_ROOT_CERTS+x} ]; then
        PARAMETERS="${PARAMETERS} -redis-tls-ca-certs ${SPLIT_SYNC_REDIS_TLS_CA_ROOT_CERTS}"
    fi

    if [ ! -z ${SPLIT_SYNC_REDIS_TLS_CLIENT_KEY+x} ]; then
        PARAMETERS="${PARAMETERS} -redis-tls-client-key ${SPLIT_SYNC_REDIS_TLS_CLIENT_KEY}"
    fi

    if [ ! -z ${SPLIT_SYNC_REDIS_TLS_CLIENT_CERTIFICATE+x} ]; then
        PARAMETERS="${PARAMETERS} -redis-tls-client-certificate ${SPLIT_SYNC_REDIS_TLS_CLIENT_CERTIFICATE}"
    fi
 
    if is_true "$SPLIT_SYNC_REDIS_FORCE_CLEANUP"; then
        PARAMETERS="${PARAMETERS} -force-fresh-startup"
    fi
  fi
    
  

  if [ ! -z ${SPLIT_SYNC_IMPRESSIONS_PER_POST+x} ]; then
    PARAMETERS="${PARAMETERS} -impressions-per-post=${SPLIT_SYNC_IMPRESSIONS_PER_POST}"
  fi

  if [ ! -z ${SPLIT_SYNC_IMPRESSIONS_THREADS+x} ]; then
    PARAMETERS="${PARAMETERS} -impressions-threads=${SPLIT_SYNC_IMPRESSIONS_THREADS}"
  fi

  if [ ! -z ${SPLIT_SYNC_ADMIN_USER+x} ]; then
    PARAMETERS="${PARAMETERS} -sync-admin-username=${SPLIT_SYNC_ADMIN_USER}"
  fi

  if [ ! -z ${SPLIT_SYNC_ADMIN_PASS+x} ]; then
    PARAMETERS="${PARAMETERS} -sync-admin-password=${SPLIT_SYNC_ADMIN_PASS}"
  fi

  if [ ! -z ${SPLIT_SYNC_DASHBOARD_TITLE+x} ]; then
    PARAMETERS="${PARAMETERS} -sync-dashboard-title=${SPLIT_SYNC_DASHBOARD_TITLE}"
  fi

  if [ ! -z ${SPLIT_SYNC_EVENTS_PER_POST+x} ]; then
    PARAMETERS="${PARAMETERS} -events-per-post=${SPLIT_SYNC_EVENTS_PER_POST}"
  fi

  if [ ! -z ${SPLIT_SYNC_EVENTS_THREADS+x} ]; then
    PARAMETERS="${PARAMETERS} -events-threads=${SPLIT_SYNC_EVENTS_THREADS}"
  fi

fi

if [ ! -z ${SPLIT_SYNC_ADVANCED_PARAMETERS+x} ]; then
  PARAMETERS="${PARAMETERS} ${SPLIT_SYNC_ADVANCED_PARAMETERS}"
fi

exec split-sync ${PARAMETERS}
