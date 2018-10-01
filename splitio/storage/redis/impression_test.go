// Package redis implements redis storage for split information
package redis

import (
	"errors"
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func findImpressionsForFeature(
	bulk map[string]map[string][]api.ImpressionsDTO,
	featureName string,
	instanceID string,
	sdk string,
) (*api.ImpressionsDTO, error) {
	for _, feature := range bulk[sdk][instanceID] {
		if feature.TestName == featureName {
			return &feature, nil
		}
	}
	return nil, errors.New("featre not found")
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
	featureName := "some_feature"

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
			Client.SAdd(
				prefixAdapter.impressionsNamespace(languageAndVersion, instanceID, featureName),
				impression,
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
	feature1Impressions, _ := findImpressionsForFeature(retrievedImpressions, languageAndVersion, instanceID, "feature1")
	if len(feature1Impressions.KeyImpressions) != 3 {
		t.Error("We should have 3 impressions for feature 1")
	}

	feature2Impressions, _ := findImpressionsForFeature(retrievedImpressions, languageAndVersion, instanceID, "feature2")
	if len(feature2Impressions.KeyImpressions) != 2 {
		t.Error("We should have 1 impressions for feature 2")
	}

	feature3Impressions, _ := findImpressionsForFeature(retrievedImpressions, languageAndVersion, instanceID, "feature3")
	if len(feature3Impressions.KeyImpressions) != 5 {
		t.Error("We should have 5 impressions for feature 3")
	}

}
