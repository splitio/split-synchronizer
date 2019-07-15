package util

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	result := ParseTime(time.Now())

	if result != "0d 0h 0m 0s" {
		t.Error("Error parsing time")
	}

	result = ParseTime(time.Now().Add(time.Duration(-30) * time.Second))
	if result != "0d 0h 0m 30s" {
		t.Error("Error parsing time")
	}

	result = ParseTime(time.Now().Add(time.Duration(-30) * time.Minute))
	if result != "0d 0h 30m 0s" {
		t.Error("Error parsing time")
	}

	result = ParseTime(time.Now().Add(time.Duration(-3) * time.Hour))
	if result != "0d 3h 0m 0s" {
		t.Error("Error parsing time")
	}

	result = ParseTime(time.Now().AddDate(0, 0, -3))
	if result != "3d 0h 0m 0s" {
		t.Error("Error parsing time")
	}

	passedTime := time.Now().Add(time.Duration(-30) * time.Second)
	passedTime = passedTime.Add(time.Duration(-30) * time.Minute)
	passedTime = passedTime.Add(time.Duration(-3) * time.Hour)
	passedTime = passedTime.AddDate(0, 0, -3)
	result = ParseTime(passedTime)
	if result != "3d 3h 30m 30s" {
		t.Error("Error parsing time")
	}
}
