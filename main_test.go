package main

import (
	"testing"

	"github.com/splitio/split-synchronizer/conf"
)

func TestInitializationWithProperParameters(t *testing.T) {
	c := "test/dataset/test.conf.warning1.json"
	configFile = &c

	loadConfiguration()

	if len(checkDeprecatedConfigParameters()) > 0 {
		t.Error("It should not be messages to inform")
	}

	if conf.Data.EventsConsumerReadSize > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.EventsPushRate > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.ImpressionsRefreshRate > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.EventsConsumerThreads > 0 {
		t.Error("It should be 0")
	}
}

func TestInitializationWithPassingEeventsConsumerReadSize(t *testing.T) {
	c := "test/dataset/test.conf.warning2.json"
	configFile = &c

	loadConfiguration()

	messages := checkDeprecatedConfigParameters()

	if len(messages) == 0 {
		t.Error("It should be messages to inform")
	}

	expected := "The parameter 'eventsConsumerReadSize' and 'events-consumer-read-size' will be deprecated soon in favor of 'eventsPerPost' or 'events-per-post'. Mapping to replacement: 'eventsPerPost'/'events-per-post'."
	if messages[0] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[0])
		t.Error("Expected -> ", expected)
	}

	if conf.Data.EventsPerPost != 5 {
		t.Error("Wrong value for event per post")
	}
	if conf.Data.EventsPushRate > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.ImpressionsRefreshRate > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.EventsConsumerThreads > 0 {
		t.Error("It should be 0")
	}
}

func TestInitializationWithPassingEventsPushRate(t *testing.T) {
	c := "test/dataset/test.conf.warning3.json"
	configFile = &c

	loadConfiguration()

	messages := checkDeprecatedConfigParameters()

	if len(messages) == 0 {
		t.Error("It should be messages to inform")
	}

	expected := "The parameter 'eventsPushRate' and 'events-push-rate' will be deprecated soon in favor of 'eventsPostRate' or 'events-post-rate'. Mapping to replacement: 'eventsPostRate'/'events-post-rate'."
	if messages[0] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[0])
		t.Error("Expected -> ", expected)
	}

	if conf.Data.EventsConsumerReadSize > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.EventsPostRate != 30 {
		t.Error("It should be 30")
	}
	if conf.Data.ImpressionsRefreshRate > 0 {
		t.Error("It should be 0")
	}
	if conf.Data.EventsConsumerThreads > 0 {
		t.Error("It should be 0")
	}
}
func TestInitializationWithPassingDeprecatedProperties(t *testing.T) {
	c := "test/dataset/test.conf.warning4.json"
	configFile = &c

	loadConfiguration()

	messages := checkDeprecatedConfigParameters()

	if len(messages) == 0 {
		t.Error("It should be messages to inform")
	}

	expected := "The parameter 'eventsConsumerReadSize' and 'events-consumer-read-size' will be deprecated soon in favor of 'eventsPerPost' or 'events-per-post'. Mapping to replacement: 'eventsPerPost'/'events-per-post'."
	if messages[0] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[0])
		t.Error("Expected -> ", expected)
	}

	expected = "The parameter 'eventsPushRate' and 'events-push-rate' will be deprecated soon in favor of 'eventsPostRate' or 'events-post-rate'. Mapping to replacement: 'eventsPostRate'/'events-post-rate'."
	if messages[1] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[1])
		t.Error("Expected -> ", expected)
	}

	expected = "The parameter 'impressionsRefreshRate' will be deprecated soon in favor of 'impressionsPostRate'. Mapping to replacement: 'impressionsPostRate'."
	if messages[2] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[2])
		t.Error("Expected -> ", expected)
	}

	expected = "The parameter 'eventsConsumerThreads' and 'events-consumer-threads' will be deprecated soon in favor of 'eventsThreads' or 'events-threads'. Mapping to replacement 'eventsThreads'/'events-threads'."
	if messages[3] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[3])
		t.Error("Expected -> ", expected)
	}

	if conf.Data.EventsPerPost != 5 {
		t.Error("It should be 5")
	}
	if conf.Data.EventsPostRate != 30 {
		t.Error("It should be 30")
	}
	if conf.Data.ImpressionsPostRate != 2 {
		t.Error("It should be 2")
	}
	if conf.Data.EventsThreads != 2 {
		t.Error("It should be 2")
	}
}

func TestInitializationWithPassingDeprecatedPropertiesAndNonDeprecatedProperties(t *testing.T) {
	c := "test/dataset/test.conf.warning5.json"
	configFile = &c

	loadConfiguration()

	messages := checkDeprecatedConfigParameters()

	if len(messages) == 0 {
		t.Error("It should be messages to inform")
	}

	expected := "The parameter 'eventsConsumerReadSize' and 'events-consumer-read-size' will be deprecated soon in favor of 'eventsPerPost' or 'events-per-post'. Mapping to replacement: 'eventsPerPost'/'events-per-post'."
	if messages[0] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[0])
		t.Error("Expected -> ", expected)
	}

	expected = "The parameter 'eventsPushRate' and 'events-push-rate' will be deprecated soon in favor of 'eventsPostRate' or 'events-post-rate'. Mapping to replacement: 'eventsPostRate'/'events-post-rate'."
	if messages[1] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[1])
		t.Error("Expected -> ", expected)
	}

	expected = "The parameter 'impressionsRefreshRate' will be deprecated soon in favor of 'impressionsPostRate'. Mapping to replacement: 'impressionsPostRate'."
	if messages[2] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[2])
		t.Error("Expected -> ", expected)
	}

	expected = "The parameter 'eventsConsumerThreads' and 'events-consumer-threads' will be deprecated soon in favor of 'eventsThreads' or 'events-threads'. Mapping to replacement 'eventsThreads'/'events-threads'."
	if messages[3] != expected {
		t.Error("Error is distinct from the expected one")
		t.Error("Actual -> ", messages[3])
		t.Error("Expected -> ", expected)
	}

	if conf.Data.EventsPerPost != 5 {
		t.Error("It should be 5")
	}
	if conf.Data.EventsPostRate != 30 {
		t.Error("It should be 30")
	}
	if conf.Data.ImpressionsPostRate != 5 {
		t.Error("It should be 5")
	}
	if conf.Data.EventsThreads != 4 {
		t.Error("It should be 4")
	}
}
