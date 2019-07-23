package redis

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

var splitsMock = `{
  "splits": [%s],
  "since": -1,
  "till": 1491244291288
}`

var splitMock = `{
  "trafficTypeName": "user",
  "name": "DEMO_MURMUR2",
  "trafficAllocation": 100,
  "trafficAllocationSeed": 1314112417,
  "seed": -2059033614,
  "status": "ACTIVE",
  "killed": false,
  "defaultTreatment": "of",
  "changeNumber": 1491244291288,
  "algo": 2,
  "conditions": [
    {
      "conditionType": "ROLLOUT",
      "matcherGroup": {
        "combiner": "AND",
        "matchers": [
          {
            "keySelector": {
              "trafficType": "user",
              "attribute": null
            },
            "matcherType": "ALL_KEYS",
            "negate": false,
            "userDefinedSegmentMatcherData": null,
            "whitelistMatcherData": null,
            "unaryNumericMatcherData": null,
            "betweenMatcherData": null
          }
        ]
      },
      "partitions": [
        {
          "treatment": "on",
          "size": 0
        },
        {
          "treatment": "of",
          "size": 100
        }
      ],
      "label": "in segment all"
    }
  ]
}`

func TestSplitStorageAdapter(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	config := conf.NewInitializedConfigData()
	Initialize(config.Redis)

	mockedData := fmt.Sprintf(splitsMock, splitMock)

	var splitChangesDtoFromMock api.SplitChangesDTO
	err := json.Unmarshal([]byte(mockedData), &splitChangesDtoFromMock)
	if err != nil {
		t.Error("Error parsing split changes JSON ", err)
		return
	}

	redisStorageAdapter := NewSplitStorageAdapter(Client, "")

	err = redisStorageAdapter.Save([]byte(splitMock))
	if err != nil {
		t.Error(err)
		return
	}

	exist := redisStorageAdapter.getSplit("DEMO_MURMUR2")
	if exist == nil {
		t.Error("It should exist")
	}

	notExist := redisStorageAdapter.getSplit("DEMO_MURMUR2_")
	if notExist != nil {
		t.Error("It should not exist")
	}

	err = redisStorageAdapter.Remove([]byte(splitMock))
	if err != nil {
		t.Error(err)
		return
	}

	err = redisStorageAdapter.Save([]byte("invalid split"))
	if err == nil {
		t.Error(err)
		return
	}

	err = redisStorageAdapter.Remove([]byte("invalid split"))
	if err == nil {
		t.Error(err)
		return
	}

	err = redisStorageAdapter.SetChangeNumber(splitChangesDtoFromMock.Till)
	if err != nil {
		t.Error(err)
		return
	}

	changeNumber, err2 := redisStorageAdapter.ChangeNumber()
	if err2 != nil {
		t.Error(err2)
		return
	}
	if changeNumber != splitChangesDtoFromMock.Till {
		t.Error("Change number, mismatch")
	}

	err = redisStorageAdapter.RegisterSegment("some_segment")
	if err != nil {
		t.Error(err)
		return
	}
}
