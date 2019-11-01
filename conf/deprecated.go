package conf

func ProcessDeprecatedOptions() []string {
	deprecatedMessages := make([]string, 0)

	if Data.ImpressionsConsumerThreads > 0 {
		deprecatedMessages = append(deprecatedMessages, "The cli parameter 'impressions-consumer-threads' will be deprecated soon in favor of 'impressions-threads'. Mapping to replacement: 'impressions-threads'.")
		if Data.ImpressionsThreads == 1 {
			Data.ImpressionsThreads = Data.ImpressionsConsumerThreads
		}
	}

	if Data.EventsConsumerReadSize > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'eventsConsumerReadSize' and 'events-consumer-read-size' will be deprecated soon in favor of 'eventsPerPost' or 'events-per-post'. Mapping to replacement: 'eventsPerPost'/'events-per-post'.")
		if Data.EventsPerPost == 10000 {
			Data.EventsPerPost = Data.EventsConsumerReadSize
		}
	}

	if Data.EventsPushRate > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'eventsPushRate' and 'events-push-rate' will be deprecated soon in favor of 'eventsPostRate' or 'events-post-rate'. Mapping to replacement: 'eventsPostRate'/'events-post-rate'.")
		if Data.EventsPostRate == 60 {
			Data.EventsPostRate = Data.EventsPushRate
		}
	}

	if Data.ImpressionsRefreshRate > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'impressionsRefreshRate' will be deprecated soon in favor of 'impressionsPostRate'. Mapping to replacement: 'impressionsPostRate'.")
		if Data.ImpressionsPostRate == 20 {
			Data.ImpressionsPostRate = Data.ImpressionsRefreshRate
		}
	}

	if Data.EventsConsumerThreads > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'eventsConsumerThreads' and 'events-consumer-threads' will be deprecated soon in favor of 'eventsThreads' or 'events-threads'. Mapping to replacement 'eventsThreads'/'events-threads'.")
		if Data.EventsThreads == 1 {
			Data.EventsThreads = Data.EventsConsumerThreads
		}
	}

	return deprecatedMessages
}
