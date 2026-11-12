package util

import (
	"bufio"
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/splitio/go-toolkit/v5/hasher"
)

func TestConfigFingerprint(t *testing.T) {
	inline := func(apikey, spec string, flagSets []string) string {
		return strconv.Itoa(int(HashAPIKey(apikey + spec + strings.Join(flagSets, "::"))))
	}

	fp := ConfigFingerprint("key-1", "1.3", []string{"set_a", "set_b"})
	if fp != inline("key-1", "1.3", []string{"set_a", "set_b"}) {
		t.Errorf("fingerprint must equal the inline expression, got %s", fp)
	}
	if fp == ConfigFingerprint("key-2", "1.3", []string{"set_a", "set_b"}) {
		t.Error("fingerprint must differ when the api key differs")
	}
	if fp == ConfigFingerprint("key-1", "1.2", []string{"set_a", "set_b"}) {
		t.Error("fingerprint must differ when the flag spec version differs")
	}
	if fp == ConfigFingerprint("key-1", "1.3", []string{"set_a"}) {
		t.Error("fingerprint must differ when the flag sets differ")
	}
	if ConfigFingerprint("", "", nil) != inline("", "", nil) {
		t.Error("empty inputs must still match the inline expression")
	}
}

func TestMurmurHashOnAlphanumericData(t *testing.T) {
	inFile, _ := os.Open("../../test/murmur/murmur3-sample-data-v2.csv")
	defer func() {
		if err := inFile.Close(); err != nil {
			t.Error(err)
		}
	}()

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
	defer func() {
		if err := inFile.Close(); err != nil {
			t.Error(err)
		}
	}()

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
