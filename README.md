# Split Synchronizer
[![Build Status](https://api.travis-ci.com/splitio/split-synchronizer.svg?branch=master)](https://api.travis-ci.com/splitio/split-synchronizer)

## Overview
By default, Split’s SDKs keep segment and split data synchronized as users navigate across disparate systems, treatments, and conditions. However, some languages, do not have a native capability to keep a shared local cache of this data to properly serve treatments. For these cases, we built the Split Synchronizer.

[![Twitter Follow](https://img.shields.io/twitter/follow/splitsoftware.svg?style=social&label=Follow&maxAge=1529000)](https://twitter.com/intent/follow?screen_name=splitsoftware)

## Compatibility
Split Synchronizer supports Go version 1.18 or higher.

## Getting started
Below is a simple example that describes the instantiation of Split Synchronizer:

### Usage via Go
1. Install dependencies via `go build` || `go mod vendor`
2. Then, execute `go run main.go -apikey "<YOUR_SDK_TYPE_API_KEY>" -redis-host "<YOUR_REDIS_HOST>" -redis-port <YOUR_REDIS_PORT> -redis-prefix "<YOUR_PREFIX>"`

### Docker
1. You can pull the Docker image from [Docker Hub](https://hub.docker.com/r/splitsoftware/split-synchronizer) and run it into your container environment.

```shell
docker pull splitsoftware/split-synchronizer:latest
```

2. Run the image:

```shell
docker run --rm --name split-synchronizer \
 -p 3010:3010 \
 -e SPLIT_SYNC_APIKEY=<YOUR_SDK_TYPE_API_KEY> \
 -e SPLIT_SYNC_REDIS_HOST=<YOUR_REDIS_HOST> \
 -e SPLIT_SYNC_REDIS_PORT=<YOUR_REDIS_PORT> \
 -e SPLIT_SYNC_REDIS_PREFIX=<YOUR_PREFIX> \
 splitsoftware/split-synchronizer
```

Please refer to [our official docs](https://help.split.io/hc/en-us/articles/360019686092-Split-Synchronizer-Proxy) to learn about all the functionality provided by Split Synchronizer and the configuration options available for tailoring it to your current application setup.

## Submitting issues
The Split team monitors all issues submitted to this [issue tracker](https://github.com/splitio/split-synchronizer/issues). We encourage you to use this issue tracker to submit any bug reports, feedback, and feature enhancements. We'll do our best to respond in a timely manner.

## Contributing
Please see [Contributors Guide](CONTRIBUTORS-GUIDE.md) to find all you need to submit a Pull Request (PR).

## License
Licensed under the Apache License, Version 2.0. See: [Apache License](http://www.apache.org/licenses/).

## About Split

Split is the leading Feature Delivery Platform for engineering teams that want to confidently deploy features as fast as they can develop them. Split’s fine-grained management, real-time monitoring, and data-driven experimentation ensure that new features will improve the customer experience without breaking or degrading performance. Companies like Twilio, Salesforce, GoDaddy and WePay trust Split to power their feature delivery.

To learn more about Split, contact hello@split.io, or get started with feature flags for free at https://www.split.io/signup.

Split has built and maintains SDKs for:

* Java [Github](https://github.com/splitio/java-client) [Docs](https://help.split.io/hc/en-us/articles/360020405151-Java-SDK)
* Javascript [Github](https://github.com/splitio/javascript-client) [Docs](https://help.split.io/hc/en-us/articles/360020448791-JavaScript-SDK)
* Node [Github](https://github.com/splitio/javascript-client) [Docs](https://help.split.io/hc/en-us/articles/360020564931-Node-js-SDK)
* .NET [Github](https://github.com/splitio/.net-core-client) [Docs](https://help.split.io/hc/en-us/articles/360020240172--NET-SDK)
* Ruby [Github](https://github.com/splitio/ruby-client) [Docs](https://help.split.io/hc/en-us/articles/360020673251-Ruby-SDK)
* PHP [Github](https://github.com/splitio/php-client) [Docs](https://help.split.io/hc/en-us/articles/360020350372-PHP-SDK)
* Python [Github](https://github.com/splitio/python-client) [Docs](https://help.split.io/hc/en-us/articles/360020359652-Python-SDK)
* GO [Github](https://github.com/splitio/go-client) [Docs](https://help.split.io/hc/en-us/articles/360020093652-Go-SDK)
* Android [Github](https://github.com/splitio/android-client) [Docs](https://help.split.io/hc/en-us/articles/360020343291-Android-SDK)
* IOS [Github](https://github.com/splitio/ios-client) [Docs](https://help.split.io/hc/en-us/articles/360020401491-iOS-SDK)

For a comprehensive list of opensource projects visit our [Github page](https://github.com/splitio?utf8=%E2%9C%93&query=%20only%3Apublic%20).

**Learn more about Split:**

Visit [split.io/product](https://www.split.io/product) for an overview of Split, or visit our documentation at [docs.split.io](https://help.split.io/hc/en-us) for more detailed information.
