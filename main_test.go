package main

import (
	"testing"
)

func TestWrongConfigs(t *testing.T) {
	c := "test/dataset/test.conf.error1.json"
	err := loadConfiguration(&c, nil)
	if err.Error() != "\"redisError\" is not a valid property in configuration" {
		t.Error("Unexpected error msg")
	}
}

func TestWrongConfigsChild(t *testing.T) {
	c := "test/dataset/test.conf.error2.json"
	err := loadConfiguration(&c, nil)
	if err.Error() != "\"redis.hostError\" is not a valid property in configuration" {
		t.Error("Unexpected error msg")
	}
}

func TestWrongConfigsMetric(t *testing.T) {
	c := "test/dataset/test.conf.error3.json"
	err := loadConfiguration(&c, nil)
	if err.Error() != "\"metricsError\" is not a valid property in configuration" {
		t.Error("Unexpected error msg")
	}
}

func TestConfigsOk(t *testing.T) {
	c := "test/dataset/test.conf.json"
	err := loadConfiguration(&c, nil)
	if err != nil {
		t.Error("Unexpected error msg")
	}
}
