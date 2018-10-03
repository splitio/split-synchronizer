// Package redis implements redis storage for split information
package redis

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func findImpressionsForFeature(
	bulk map[string]map[string][]api.ImpressionsDTO,
	sdk string,
	instanceID string,
	featureName string,
) (*api.ImpressionsDTO, error) {
	for _, feature := range bulk[sdk][instanceID] {
		if feature.TestName == featureName {
			return &feature, nil
		}
	}
	return nil, fmt.Errorf("Feature %s not found", featureName)
}

func TestImpressionStorageAdapter(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	languageAndVersion := "test-2.0"
	instanceID := "127.0.0.1"
	featureName := "some_feature"
	impressionTXT1 := `{"keyName":"some_key1","treatment":"off","time":1234567890,"changeNumber":55555555,"label":"some label","bucketingKey":"some_bucket_key"}`
	impressionTXT2 := `{"keyName":"some_key2","treatment":"on","time":1234567999,"changeNumber":577775,"label":"some label no match","bucketingKey":"some_bucket_key_2"}`

	prefixAdapter := &prefixAdapter{prefix: ""}
	impressionsKey := prefixAdapter.impressionsNamespace(languageAndVersion, instanceID, featureName)
	//Adding impressions to retrieve.
	Client.SAdd(impressionsKey, impressionTXT1)
	Client.SAdd(impressionsKey, impressionTXT2)

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions()

	if err != nil {
		t.Error(err)
	}

	_, ok1 := retrievedImpressions[languageAndVersion]
	if !ok1 {
		t.Error("Error retrieving impressions by language and version")
	}
	_, ok2 := retrievedImpressions[languageAndVersion][instanceID]
	if !ok2 {
		t.Error("Error retrieving impressions by instance ID ")
	}

	impressionDTO := retrievedImpressions[languageAndVersion][instanceID][0]

	if impressionDTO.TestName != featureName {
		t.Error("Error on fetched impressions - Test name")
	}

	if len(impressionDTO.KeyImpressions) != 2 {
		t.Error("Error counting fetched impressions")
	}
}

func TestThatQuotaiIsApplied(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	languageAndVersion := "test-2.0"
	instanceID := "127.0.0.1"

	impressionsRaw := map[string][]api.ImpressionDTO{
		"feature1": {
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key1",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment1",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key2",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment2",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key3",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment3",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key4",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment4",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key5",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment5",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key6",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment6",
			},
		},
		"feature2": {
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key1",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment1",
			},
		},
		"feature3": {
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key1",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment1",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key2",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment2",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key3",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment3",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key4",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment4",
			},
			{
				BucketingKey: "",
				ChangeNumber: 0,
				KeyName:      "key5",
				Label:        "label1",
				Time:         123,
				Treatment:    "treatment5",
			},
		},
	}

	prefixAdapter := &prefixAdapter{prefix: ""}
	//Adding impressions to retrieve.
	for feature, impressions := range impressionsRaw {
		for _, impression := range impressions {
			toStore, _ := json.Marshal(impression)
			Client.SAdd(
				prefixAdapter.impressionsNamespace(languageAndVersion, instanceID, feature),
				toStore,
			)
		}
	}
	conf.Data.ImpressionsPerPost = 9
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions()

	if err != nil {
		t.Error(err)
	}

	// We set an impressionsPerPost value of 6. Which means that when distributed evenly, we should get 3 impressions
	// per feature. Since feature 2 has only one, we should get 3 for feature1, 1 for feature2 and 5 for feature3.
	feature1Impressions, err := findImpressionsForFeature(retrievedImpressions, languageAndVersion, instanceID, "feature1")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature1Impressions.KeyImpressions) != 3 {
		t.Errorf("We should have 3 impressions for feature1, we have %d", len(feature1Impressions.KeyImpressions))
	}

	feature2Impressions, _ := findImpressionsForFeature(retrievedImpressions, languageAndVersion, instanceID, "feature2")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature2Impressions.KeyImpressions) != 1 {
		t.Errorf("We should have 3 impressions for feature2, we have %d", len(feature2Impressions.KeyImpressions))
	}

	feature3Impressions, _ := findImpressionsForFeature(retrievedImpressions, languageAndVersion, instanceID, "feature3")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature3Impressions.KeyImpressions) != 5 {
		t.Errorf("We should have 5 impressions for feature3, we have %d", len(feature3Impressions.KeyImpressions))
	}
}

func TestLuaScriptFailure(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	impressionKeysWithCardinalityScriptTemplate = `local impkeys = redis.call('KEYS', '{KEY_NAME`

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	imps, err := impressionsStorageAdapter.getImpressionsWithCardinality()
	if imps != nil {
		t.Error("Resulting impressions should be nil")
		return
	}

	if err == nil {
		t.Error("Should have failed")
		return
	}

	if !strings.Contains(
		err.Error(),
		"Failed to execute LUA script: ERR Error compiling script (new function): user_script:1: unfinished string near ''{KEY_NAME'",
	) {
		t.Error("Incorrect error cause")
		t.Error(err.Error())
		return
	}

	imps2, err := impressionsStorageAdapter.RetrieveImpressions()
	if imps2 == nil {
		t.Error("Impressions should not be null, fallback should work correctly")
		return
	}

	if err != nil {
		t.Error("No error should have been generated. fallback should work correctly")
		return
	}
}

func TestLuaScriptReturnsIncorrectType(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	impressionKeysWithCardinalityScriptTemplate = `return 1`

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	imps, err := impressionsStorageAdapter.getImpressionsWithCardinality()
	if imps != nil {
		t.Error("Resulting impressions should be nil")
		return
	}

	if err == nil {
		t.Error("Should have failed")
		return
	}

	if !strings.Contains(
		err.Error(),
		"Failed to type-assert script's output. []interface {}",
	) {
		t.Error("Incorrect error cause")
		t.Error(err.Error())
		return
	}

	imps2, err := impressionsStorageAdapter.RetrieveImpressions()
	if imps2 == nil {
		t.Error("Impressions should not be null, fallback should work correctly")
		return
	}

	if err != nil {
		t.Error("No error should have been generated. fallback should work correctly")
		return
	}
}

func TestLuaScriptReturnsIncorrectSlice(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	impressionKeysWithCardinalityScriptTemplate = `return {1, 2, 3}`

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	imps, err := impressionsStorageAdapter.getImpressionsWithCardinality()
	if imps != nil {
		t.Error("Resulting impressions should be nil")
		return
	}

	if err == nil {
		t.Error("Should have failed")
		return
	}

	if !strings.Contains(
		err.Error(),
		"Failed to type-assert returned structure: Error casting 1 to string, it's int64",
	) {
		t.Error("Incorrect error cause")
		t.Error(err.Error())
		return
	}

	imps2, err := impressionsStorageAdapter.RetrieveImpressions()
	if imps2 == nil {
		t.Error("Impressions should not be null, fallback should work correctly")
		return
	}

	if err != nil {
		t.Error("No error should have been generated. fallback should work correctly")
		return
	}
}

func TestThatMalformedImpressionKeysDoNotPanic(t *testing.T) {
	wrongKeys := []string{
		"SPLITIO/php-5.3.1//impressions.mono_signal_unique_payers_for_payee",
		"SPLITIO/php-5.3.1///impressions.mono_signal_unique_payers_for_payee",
		"SPLITIO//ip-123-123-123-123/impressions.mono_signal_unique_payers_for_payee",
		"SPLITIO///impressions.mono_signal_unique_payers_for_payee",
		"SPLITIO///ip-123-123-123-123//php-5.3.1//impressions.mono_signal_unique_payers_for_payee",
	}

	for _, key := range wrongKeys {
		sdk, ip, feature, err := parseImpressionKey(key)
		if err == nil {
			t.Error("An error should have been returned.")
		}
		if sdk != nil {
			t.Errorf("Sdk should be nil. Is %s", *sdk)
		}
		if ip != nil {
			t.Errorf("Ip should be nil. Is %s", *ip)
		}
		if feature != nil {
			t.Errorf("Feature should be nil. Is %s", *feature)
		}
	}
}
