// Package task contains all agent tasks
package task

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

/* ImpressionStorage for testing */
type testImpressionStorage struct{}

func (r testImpressionStorage) RetrieveImpressions() (map[string]map[string][]api.ImpressionsDTO, error) {
	imp1 := api.ImpressionDTO{KeyName: "some_key_1", Treatment: "on", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_1", BucketingKey: "some_bucket_key_1"}
	imp2 := api.ImpressionDTO{KeyName: "some_key_2", Treatment: "off", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_2", BucketingKey: "some_bucket_key_2"}

	keyImpressions := make([]api.ImpressionDTO, 0)
	keyImpressions = append(keyImpressions, imp1, imp2)
	impressionsTest := api.ImpressionsDTO{TestName: "some_test", KeyImpressions: keyImpressions}

	impressions := make([]api.ImpressionsDTO, 0)
	impressions = append(impressions, impressionsTest)

	toReturn := make(map[string]map[string][]api.ImpressionsDTO, 0)
	toReturn["test-2.0"] = make(map[string][]api.ImpressionsDTO, 0)
	toReturn["test-2.0"]["127.0.0.1"] = impressions
	return toReturn, nil
}

/* ImpressionsRecorder for testing */
type testImpressionsRecorder struct{}

func (r testImpressionsRecorder) Post(impressions []api.ImpressionsDTO, sdkVersion string, machineIP string) error {
	return nil
}

func TestTaskPostImpressions(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	tid := 1
	impressionsRecorderAdapter := testImpressionsRecorder{}
	impressionStorageAdapter := testImpressionStorage{}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskPostImpressions(tid, impressionsRecorderAdapter, impressionStorageAdapter)
	}()
}
