// Package redis implements redis storage for split information
package redis

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/log"
)

func TestImpressionStorageAdapter(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

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
