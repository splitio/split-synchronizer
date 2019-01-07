// Package task contains all agent tasks
package task

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

/* ImpressionStorage for testing */
type testImpressionStorage struct{}

func (r testImpressionStorage) RetrieveImpressions(count int64, legacyDisabled bool) (map[api.SdkMetadata][]api.ImpressionsDTO, error) {
	imp1 := api.ImpressionDTO{KeyName: "some_key_1", Treatment: "on", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_1", BucketingKey: "some_bucket_key_1"}
	imp2 := api.ImpressionDTO{KeyName: "some_key_2", Treatment: "off", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_2", BucketingKey: "some_bucket_key_2"}

	keyImpressions := make([]api.ImpressionDTO, 0)
	keyImpressions = append(keyImpressions, imp1, imp2)
	impressionsTest := api.ImpressionsDTO{TestName: "some_test", KeyImpressions: keyImpressions}
	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	return map[api.SdkMetadata][]api.ImpressionsDTO{metadata: {impressionsTest}}, nil
}

func (r testImpressionStorage) Size() int64 {
	return 0
}

/* ImpressionsRecorder for testing */
type testImpressionsRecorder struct{}

func (r testImpressionsRecorder) Post(impressions []api.ImpressionsDTO, metadata api.SdkMetadata) error {
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
		taskPostImpressions(tid, impressionsRecorderAdapter, impressionStorageAdapter, conf.Data.ImpressionsPerPost, true, false)
	}()
}
