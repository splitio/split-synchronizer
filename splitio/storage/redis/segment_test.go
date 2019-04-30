package redis

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

func TestSegmentStorageAdapter(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	config := conf.NewInitializedConfigData()
	config.Redis.Prefix = "segmenttests"
	Initialize(config.Redis)

	segmentName := "some_segment"
	var changeNumber int64 = 123123435345

	segmentStorageadapter := NewSegmentStorageAdapter(Client, "segmenttests")

	err := segmentStorageadapter.AddToSegment(segmentName, []string{})
	if err != nil {
		t.Error(err)
	}

	err = segmentStorageadapter.AddToSegment(segmentName, []string{"key1", "key2", "key3"})
	if err != nil {
		t.Error(err)
	}

	err = segmentStorageadapter.RemoveFromSegment(segmentName, []string{})
	if err != nil {
		t.Error(err)
	}

	err = segmentStorageadapter.RemoveFromSegment(segmentName, []string{"key3"})
	if err != nil {
		t.Error(err)
	}

	err = segmentStorageadapter.SetChangeNumber(segmentName, changeNumber)
	if err != nil {
		t.Error(err)
	}

	cn, errCn := segmentStorageadapter.ChangeNumber(segmentName)
	if errCn != nil {
		t.Error(errCn)
	}
	if cn != changeNumber {
		t.Error("Change number, mismatch")
	}

	_, errRs := segmentStorageadapter.RegisteredSegmentNames()
	if errRs != nil {
		t.Error(errRs)
	}

	segmentStorageadapter.client.Del("segmenttests.SPLITIO.segment.some_segment.till")
	segmentStorageadapter.client.Del("segmenttests.SPLITIO.segment.some_segment")
}
