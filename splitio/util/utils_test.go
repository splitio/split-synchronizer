package util

import (
	"bufio"
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/splitio/go-toolkit/hasher"
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

func TestMurmurHashOnAlphanumericData(t *testing.T) {
	inFile, _ := os.Open("../../test/murmur/murmur3-sample-data-v2.csv")
	defer inFile.Close()

	reader := csv.NewReader(bufio.NewReader(inFile))

	var arr []string
	var err error
	line := 0
	for err != io.EOF {
		line++
		arr, err = reader.Read()
		if len(arr) < 4 {
			continue // Skip empty lines
		}
		seed, _ := strconv.ParseInt(arr[0], 10, 32)
		str := arr[1]
		digest, _ := strconv.ParseUint(arr[2], 10, 32)

		murmur := hasher.NewMurmur332Hasher(uint32(seed))
		calculated := murmur.Hash([]byte(str))
		if calculated != uint32(digest) {
			t.Errorf("%d: Murmur hash calculation failed for string %s. Should be %d and was %d", line, str, digest, calculated)
			break
		}
	}
}

func TestMurmurHashOnNonAlphanumericData(t *testing.T) {
	inFile, _ := os.Open("../../test/murmur/murmur3-sample-data-non-alpha-numeric-v2.csv")
	defer inFile.Close()

	reader := csv.NewReader(bufio.NewReader(inFile))

	var arr []string
	var err error
	line := 0
	for err != io.EOF {
		line++
		arr, err = reader.Read()
		if len(arr) < 4 {
			continue // Skip empty lines
		}
		seed, _ := strconv.ParseInt(arr[0], 10, 32)
		str := arr[1]
		digest, _ := strconv.ParseUint(arr[2], 10, 32)

		murmur := hasher.NewMurmur332Hasher(uint32(seed))
		calculated := murmur.Hash([]byte(str))
		if calculated != uint32(digest) {
			t.Errorf("%d: Murmur hash calculation failed for string %s. Should be %d and was %d", line, str, digest, calculated)
			break
		}
	}
}
