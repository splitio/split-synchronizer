// Package redis implements redis storage for split information
package redis

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func makeImpressions(key string, treatment string, changenumber int64, label string, bucketingKey string, count int) []api.ImpressionDTO {
	keyMod := int(float64(count)*0.2) + 1
	imps := make([]api.ImpressionDTO, count)
	for i := 0; i < count; i++ {
		imps[i] = api.ImpressionDTO{
			BucketingKey: bucketingKey,
			ChangeNumber: changenumber,
			KeyName:      fmt.Sprintf("%s_%d", key, i%keyMod),
			Time:         changenumber + int64(i),
			Treatment:    treatment,
		}
	}
	return imps
}

func findImpressionsForFeature(bulk []api.ImpressionsDTO, featureName string) (*api.ImpressionsDTO, error) {
	for _, feature := range bulk {
		if feature.TestName == featureName {
			return &feature, nil
		}
	}
	return nil, fmt.Errorf("Feature %s not found", featureName)
}

func TestImpressionStorageAdapterNoQueueKey(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}
	featureName := "some_feature"

	i1TXT := `{
	    "keyName":"some_key1",
	    "treatment":"off",
	    "time":1234567890,
	    "changeNumber":55555555,
	    "label":"some label",
	    "bucketingKey":"some_bucket_key"
	}`
	i2TXT := `{
	    "keyName":"some_key2",
	    "treatment":"on",
	    "time":1234567999,
	    "changeNumber":577775,
	    "label":"some label no match",
	    "bucketingKey":"some_bucket_key_2"
	}`
	impressionsKey := prefixAdapter.impressionsNamespace(metadata.SdkVersion, metadata.MachineIP, featureName)
	//Adding impressions to retrieve.
	Client.SAdd(impressionsKey, i1TXT, i2TXT)

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions(500, false)
	if err != nil {
		t.Error(err)
	}

	_, ok1 := retrievedImpressions[metadata]
	if !ok1 {
		t.Error("Error retrieving impressions by language and version, machineIp & name")
	}

	impressionDTO := retrievedImpressions[metadata][0]

	if impressionDTO.TestName != featureName {
		t.Error("Error on fetched impressions - Test name")
	}

	if len(impressionDTO.KeyImpressions) != 2 {
		t.Error("Error counting fetched impressions")
	}
}

func TestThatQuotaiIsAppliedNoQueueKey(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	impressionsRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("key", "on", 123456, "some_label", "key", 30),
		"feature2": makeImpressions("key", "on", 123456, "some_label", "key", 70),
		"feature3": makeImpressions("key", "on", 123456, "some_label", "key", 100),
	}

	//Adding impressions to retrieve.
	for feature, impressions := range impressionsRaw {
		for _, impression := range impressions {
			toStore, _ := json.Marshal(impression)
			Client.SAdd(
				prefixAdapter.impressionsNamespace(metadata.SdkVersion, metadata.MachineIP, feature),
				toStore,
			)
		}
	}
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")

	// We have 200 impressions in storage (30 + 70 + 100). And try to retrieve 150 in total,
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions(150, false)
	if err != nil {
		t.Error(err)
	}

	feature1Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature1")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature1Impressions.KeyImpressions) != 30 {
		t.Errorf("We should have 3 impressions for feature1, we have %d", len(feature1Impressions.KeyImpressions))
	}

	feature2Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature2")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature2Impressions.KeyImpressions) != 70 {
		t.Errorf("We should have 3 impressions for feature2, we have %d", len(feature2Impressions.KeyImpressions))
	}

	feature3Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature3")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature3Impressions.KeyImpressions) != 50 {
		t.Errorf("We should have 5 impressions for feature3, we have %d", len(feature3Impressions.KeyImpressions))
	}
}

func TestLuaScriptFailure(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

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
		"Failed to execute LUA script: ERR Error compiling script ",
	) {
		t.Error("Incorrect error cause")
		t.Error(err.Error())
		return
	}

	imps2, err := impressionsStorageAdapter.RetrieveImpressions(500, false)
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
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

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

	imps2, err := impressionsStorageAdapter.RetrieveImpressions(500, true)
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
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

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

	imps2, err := impressionsStorageAdapter.RetrieveImpressions(500, true)
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
		if sdk != "" {
			t.Errorf("Sdk should be empty. Is %s", sdk)
		}
		if ip != "" {
			t.Errorf("Ip should be empty. Is %s", ip)
		}
		if feature != "" {
			t.Errorf("Feature should be empty. Is %s", feature)
		}
	}
}

func TestImpressionsSingleQueue(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	impressionsRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("key", "on", 123456, "some_label", "key", 30),
		"feature2": makeImpressions("key", "on", 123456, "some_label", "key", 70),
		"feature3": makeImpressions("key", "on", 123456, "some_label", "key", 100),
	}

	//Adding impressions to retrieve.
	for feature, impressions := range impressionsRaw {
		for _, impression := range impressions {
			toStore, err := json.Marshal(ImpressionDTO{
				Data: ImpressionObject{
					BucketingKey:      impression.BucketingKey,
					FeatureName:       feature,
					KeyName:           impression.KeyName,
					Rule:              impression.Label,
					SplitChangeNumber: impression.ChangeNumber,
					Timestamp:         impression.Time,
					Treatment:         impression.Treatment,
				},
				Metadata: ImpressionMetadata{
					InstanceIP:   metadata.MachineIP,
					InstanceName: metadata.MachineName,
					SdkVersion:   metadata.SdkVersion,
				},
			})
			if err != nil {
				t.Error(err.Error())
				return
			}

			Client.LPush(
				prefixAdapter.impressionsQueueNamespace(),
				toStore,
			)
		}
	}
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")

	// We have 200 impressions in storage (30 + 70 + 100). And try to retrieve 150 in total,
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions(200, false)
	if err != nil {
		t.Error(err.Error())
		return
	}

	if len(retrievedImpressions[metadata]) != 3 {
		t.Error("Should have 3 elements. Had ", len(retrievedImpressions[metadata]))
	}

	feature1Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature1")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature1Impressions.KeyImpressions) != 30 {
		t.Error("Should have 30 elements. Had, ", len(feature1Impressions.KeyImpressions))
	}

	feature2Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature2")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature2Impressions.KeyImpressions) != 70 {
		t.Error("Should have 70 elements. Had, ", len(feature2Impressions.KeyImpressions))
	}

	feature3Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature3")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature3Impressions.KeyImpressions) != 100 {
		t.Error("Should have 100 elements. Had, ", len(feature3Impressions.KeyImpressions))
	}
}

func TestImpressionsSingleQueueAndLegacy(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	impressionsSQRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("key", "on", 123456, "some_label", "key", 30),
		"feature2": makeImpressions("key", "on", 123456, "some_label", "key", 20),
		"feature3": makeImpressions("key", "on", 123456, "some_label", "key", 5),
	}

	//Adding impressions to single queue.
	for feature, impressions := range impressionsSQRaw {
		for _, impression := range impressions {
			toStore, err := json.Marshal(ImpressionDTO{
				Data: ImpressionObject{
					BucketingKey:      impression.BucketingKey,
					FeatureName:       feature,
					KeyName:           impression.KeyName,
					Rule:              impression.Label,
					SplitChangeNumber: impression.ChangeNumber,
					Timestamp:         impression.Time,
					Treatment:         impression.Treatment,
				},
				Metadata: ImpressionMetadata{
					InstanceIP:   metadata.MachineIP,
					InstanceName: metadata.MachineName,
					SdkVersion:   metadata.SdkVersion,
				},
			})
			if err != nil {
				t.Error(err.Error())
				return
			}

			Client.LPush(
				prefixAdapter.impressionsQueueNamespace(),
				toStore,
			)
		}
	}

	impressionsLegacyRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("keykey", "on", 123456, "some_label", "key", 10),
		"feature2": makeImpressions("keykey", "on", 123456, "some_label", "key", 10),
		"feature3": makeImpressions("keykey", "on", 123456, "some_label", "key", 10),
	}

	//Adding impressions to retrieve.
	for feature, impressions := range impressionsLegacyRaw {
		for _, impression := range impressions {
			toStore, _ := json.Marshal(impression)
			Client.SAdd(
				prefixAdapter.impressionsNamespace(metadata.SdkVersion, metadata.MachineIP, feature),
				toStore,
			)
		}
	}

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")

	// We have 200 impressions in storage (30 + 70 + 100). And try to retrieve 150 in total,
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions(200, false)
	if err != nil {
		t.Error(err.Error())
		return
	}

	if len(retrievedImpressions[metadata]) != 3 {
		t.Error("Should have 3 elements. Had ", len(retrievedImpressions[metadata]))
	}

	feature1Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature1")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature1Impressions.KeyImpressions) != 40 {
		t.Error("Should have 30 elements. Had, ", len(feature1Impressions.KeyImpressions))
	}

	feature2Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature2")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature2Impressions.KeyImpressions) != 30 {
		t.Error("Should have 70 elements. Had, ", len(feature2Impressions.KeyImpressions))
	}

	feature3Impressions, err := findImpressionsForFeature(retrievedImpressions[metadata], "feature3")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if len(feature3Impressions.KeyImpressions) != 15 {
		t.Error("Should have 100 elements. Had, ", len(feature3Impressions.KeyImpressions))
	}
}

func TestImpressionsFromSingleQueueAreRemovedAfterFetched(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	impressionsRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("key", "on", 123456, "some_label", "key", 30),
		"feature2": makeImpressions("key", "on", 123456, "some_label", "key", 70),
		"feature3": makeImpressions("key", "on", 123456, "some_label", "key", 100),
	}

	//Adding impressions to retrieve.
	for feature, impressions := range impressionsRaw {
		for _, impression := range impressions {
			toStore, err := json.Marshal(ImpressionDTO{
				Data: ImpressionObject{
					BucketingKey:      impression.BucketingKey,
					FeatureName:       feature,
					KeyName:           impression.KeyName,
					Rule:              impression.Label,
					SplitChangeNumber: impression.ChangeNumber,
					Timestamp:         impression.Time,
					Treatment:         impression.Treatment,
				},
				Metadata: ImpressionMetadata{
					InstanceIP:   metadata.MachineIP,
					InstanceName: metadata.MachineName,
					SdkVersion:   metadata.SdkVersion,
				},
			})
			if err != nil {
				t.Error(err.Error())
				return
			}

			Client.LPush(
				prefixAdapter.impressionsQueueNamespace(),
				toStore,
			)
		}
	}
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")

	// We have 200 impressions in storage (30 + 70 + 100). And try to retrieve 150 in total,
	retrievedImpressions, err := impressionsStorageAdapter.RetrieveImpressions(200, false)
	if err != nil {
		t.Error(err.Error())
		return
	}

	if len(retrievedImpressions[metadata]) != 3 {
		t.Error("Should have impressions for 3 features")
		return
	}

	retrievedImpressions, err = impressionsStorageAdapter.RetrieveImpressions(200, false)
	if err != nil {
		t.Error(err.Error())
		return
	}

	if len(retrievedImpressions[metadata]) != 0 {
		t.Error("No impressions should have been returned")
		t.Errorf("%+v", retrievedImpressions)
		return
	}
}

func TestTTLIsSet(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	imps := makeImpressions("key", "on", 123456, "some_label", "key", 500)

	//Adding impressions to retrieve.
	for _, impression := range imps {
		toStore, err := json.Marshal(ImpressionDTO{
			Data: ImpressionObject{
				BucketingKey:      impression.BucketingKey,
				FeatureName:       "some_feature",
				KeyName:           impression.KeyName,
				Rule:              impression.Label,
				SplitChangeNumber: impression.ChangeNumber,
				Timestamp:         impression.Time,
				Treatment:         impression.Treatment,
			},
			Metadata: ImpressionMetadata{
				InstanceIP:   metadata.MachineIP,
				InstanceName: metadata.MachineName,
				SdkVersion:   metadata.SdkVersion,
			},
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		Client.LPush(
			prefixAdapter.impressionsQueueNamespace(),
			toStore,
		)
	}
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")

	// We have 200 impressions in storage (30 + 70 + 100). And try to retrieve 150 in total,
	_, err := impressionsStorageAdapter.RetrieveImpressions(200, false)
	if err != nil {
		t.Error(err.Error())
		return
	}

	ttl, _ := Client.TTL(prefixAdapter.impressionsQueueNamespace()).Result()

	if ttl > time.Duration(3600)*time.Second || ttl < time.Duration(3590)*time.Second {
		t.Error("TTL should have been set and be near 3600 seconds")
	}

}

func TestImpressionsSize(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	impressionsRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("key", "on", 123456, "some_label", "key", 30),
		"feature2": makeImpressions("key", "on", 123456, "some_label", "key", 70),
		"feature3": makeImpressions("key", "on", 123456, "some_label", "key", 100),
	}

	//Adding impressions to retrieve.
	for feature, impressions := range impressionsRaw {
		for _, impression := range impressions {
			toStore, err := json.Marshal(ImpressionDTO{
				Data: ImpressionObject{
					BucketingKey:      impression.BucketingKey,
					FeatureName:       feature,
					KeyName:           impression.KeyName,
					Rule:              impression.Label,
					SplitChangeNumber: impression.ChangeNumber,
					Timestamp:         impression.Time,
					Treatment:         impression.Treatment,
				},
				Metadata: ImpressionMetadata{
					InstanceIP:   metadata.MachineIP,
					InstanceName: metadata.MachineName,
					SdkVersion:   metadata.SdkVersion,
				},
			})
			if err != nil {
				t.Error(err.Error())
				return
			}

			Client.LPush(
				prefixAdapter.impressionsQueueNamespace(),
				toStore,
			)
		}
	}
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	size := impressionsStorageAdapter.Size(prefixAdapter.impressionsQueueNamespace())
	if size != 200 {
		t.Error("Size is not the expected one. Expected 200. Actual", size)
	}
	Client.Del(prefixAdapter.impressionsQueueNamespace())
}
