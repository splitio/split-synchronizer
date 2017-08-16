Split Synchronizer [ ![Codeship Status for splitio/go-agent](https://app.codeship.com/projects/ce54acf0-1c95-0135-d754-16467d9e760e/status?branch=staging)](https://app.codeship.com/projects/220048)
===
 > **split-sync** A background service to synchronize Split information with your SDK

Split synchronizer is able to run in 2 different modes.
 - **Producer mode** (default): coordinates the sending and receiving of data to a **remote datastore** that all of your processes can share to pull data for the evaluation of treatments.
 - **Proxy mode**: keep synchronized SDKs connecting they with split-sync proxy to reduce connection latencies and letting the proxy receive information and post impressions to Split servers.

 For further information check the official documentation at: [https://docs.split.io/docs/split-synchronizer](https://docs.split.io/docs/split-synchronizer)

## Docker
#### Creating the image
 The follow command creates the Docker image tagged with the branch build version
 ```
 docker build -t splitsoftware/split-sync:$(tail -n 1 ./splitio/version.go | awk '{print $4}' | tr -d '"') .
 ```
#### Running the container
The container can be run on both modes (producer and proxy). To run it, different environment variables are available to be tuned.
```
 Environment vars:

   Common vars:
    - SPLIT_SYNC_API_KEY                     Split service API-KEY grabbed from webconsole
    - SPLIT_SYNC_SPLITS_REFRESH_RATE         Refresh rate of splits fetcher
    - SPLIT_SYNC_SEGMENTS_REFRESH_RATE       Refresh rate of segments fetcher
    - SPLIT_SYNC_IMPRESSIONS_REFRESH_RATE    Refresh rate of impressions recorder
    - SPLIT_SYNC_METRICS_REFRESH_RATE        Refresh rate of metrics recorder
    - SPLIT_SYNC_HTTP_TIMEOUT                Timeout specifies a time limit for requests
    - SPLIT_SYNC_LOG_DEBUG                   Enable debug mode: Set as 'on'
    - SPLIT_SYNC_LOG_VERBOSE                 Enable verbose mode: Set as 'on'
    - SPLIT_SYNC_LOG_STDOUT                  Enable standard output: Set as 'on'
    - SPLIT_SYNC_LOG_FILE                    Set the log file
    - SPLIT_SYNC_LOG_FILE_MAX_SIZE           Max file log size in bytes
    - SPLIT_SYNC_LOG_BACKUP_COUNT            Number of last log files to keep in filesystem
    - SPLIT_SYNC_LOG_SLACK_CHANNEL           Set the Slack channel or user
    - SPLIT_SYNC_LOG_SLACK_WEBHOOK           Set the Slack webhook url

    - SPLIT_SYNC_ADVANCED_PARAMETERS         Set custom parameters that are not configured via provided Env vars.
                                             Sample:
                                               SPLIT_SYNC_ADVANCED_PARAMETERS="-redis-read-timeout=20 -redis-max-retries=10"

   Proxy vars:
    - SPLIT_SYNC_PROXY                       Enables the proxy mode: Set as 'on'
    - SPLIT_SYNC_PROXY_SDK_APIKEYS           List of custom API-KEYs for your SDKs (Comma separated string)
    - SPLIT_SYNC_PROXY_ADMIN_USER            HTTP basic auth username for admin endpoints
    - SPLIT_SYNC_PROXY_ADMIN_PASS            HTTP basic auth password for admin endpoints
    - SPLIT_SYNC_PROXY_IMPRESSIONS_MAX_SIZE  Max size, in bytes, to send impressions in proxy mode

   Producer vars:
    - SPLIT_SYNC_REDIS_HOST                  Redis server hostname
    - SPLIT_SYNC_REDIS_PORT                  Redis Server port
    - SPLIT_SYNC_REDIS_DB                    Redis DB number
    - SPLIT_SYNC_REDIS_PASS                  Redis password
    - SPLIT_SYNC_REDIS_PREFIX                Redis key prefix
    - SPLIT_SYNC_IMPRESSIONS_PER_POST        Number of impressions to send in a POST request
    - SPLIT_SYNC_IMPRESSIONS_THREADS         Number of impressions recorder threads

```
