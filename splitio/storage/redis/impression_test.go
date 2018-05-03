// Package redis implements redis storage for split information
package redis

import (
	"encoding/json"
	"fmt"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"io/ioutil"
	"testing"
)

func impressionsForFeature(impsByFeature []api.ImpressionsDTO, featureName string) []api.ImpressionDTO {
	if impsByFeature == nil {
		return nil
	}

	for _, forFeature := range impsByFeature {
		if forFeature.TestName == featureName {
			return forFeature.KeyImpressions
		}
	}

	return nil
}

func TestImpressionStorageAdapterInLegacyMode(t *testing.T) {
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

	// Make sure to remove impressionKey from redis, so that we fallback and use legacy mode
	Client.Del(prefixAdapter.impressionKeysNamespace())

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

func TestImpressionStorageAdapterWithPerformanceBoost(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	prefixAdapter := &prefixAdapter{prefix: ""}

	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	impressionsKey := prefixAdapter.impressionKeysNamespace()

	sdkVersion := "python-5.2.1"
	instanceID := "10.0.4.23"

	// Add impression keys to the set
	Client.SAdd(impressionsKey, fmt.Sprintf("%s/%s/impressions.feature%d", sdkVersion, instanceID, 1))
	Client.SAdd(impressionsKey, fmt.Sprintf("%s/%s/impressions.feature%d", sdkVersion, instanceID, 2))
	Client.SAdd(impressionsKey, fmt.Sprintf("%s/%s/impressions.feature%d", sdkVersion, instanceID, 3))

	// Add actual impressions
	imp1JSON, _ := json.Marshal(api.ImpressionDTO{
		BucketingKey: "buck",
		ChangeNumber: 123,
		KeyName:      "key1",
		Label:        "label1",
		Time:         123,
		Treatment:    "treatment1",
	})

	imp2JSON, _ := json.Marshal(api.ImpressionDTO{
		BucketingKey: "buck",
		ChangeNumber: 123,
		KeyName:      "key2",
		Label:        "label1",
		Time:         123,
		Treatment:    "treatment1",
	})

	imp3JSON, _ := json.Marshal(api.ImpressionDTO{
		BucketingKey: "buck",
		ChangeNumber: 123,
		KeyName:      "key3",
		Label:        "label1",
		Time:         123,
		Treatment:    "treatment1",
	})

	Client.SAdd(prefixAdapter.impressionsNamespace(sdkVersion, instanceID, "feature1"), imp1JSON)
	Client.SAdd(prefixAdapter.impressionsNamespace(sdkVersion, instanceID, "feature1"), imp2JSON)
	Client.SAdd(prefixAdapter.impressionsNamespace(sdkVersion, instanceID, "feature1"), imp3JSON)
	Client.SAdd(prefixAdapter.impressionsNamespace(sdkVersion, instanceID, "feature2"), imp1JSON)
	Client.SAdd(prefixAdapter.impressionsNamespace(sdkVersion, instanceID, "feature2"), imp2JSON)
	Client.SAdd(prefixAdapter.impressionsNamespace(sdkVersion, instanceID, "feature3"), imp3JSON)

	impressions, err := impressionsStorageAdapter.RetrieveImpressions()
	if err != nil {
		t.Error("Error retrieving impressions")
		t.Error(err.Error())
		return
	}

	if len(impressions) == 0 {
		t.Error("No impressions returned")
		return
	}

	bySdkVersion, ok := impressions[sdkVersion]
	if !ok {
		t.Errorf("Impressions not found for sdk version %s", sdkVersion)
		return
	}

	byMachineName, ok := bySdkVersion[instanceID]
	if !ok {
		t.Errorf("Impressions not found for instance id %s", instanceID)
		return
	}

	forFeature1 := impressionsForFeature(byMachineName, "feature1")
	if len(forFeature1) != 3 {
		t.Error("Feature1 should have 3 impressions")
	}

	forFeature2 := impressionsForFeature(byMachineName, "feature2")
	if len(forFeature2) != 2 {
		t.Error("Feature2 should have 2 impressions")
	}

	forFeature3 := impressionsForFeature(byMachineName, "feature3")
	if !ok || len(forFeature3) != 1 {
		t.Error("Feature3 should have 1 impression")
	}

	impressionsAfter, err := impressionsStorageAdapter.RetrieveImpressions()
	if len(impressionsAfter) > 0 {
		t.Error("No impressions should have been returned.")
	}
}
