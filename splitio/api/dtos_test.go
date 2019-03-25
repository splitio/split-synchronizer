// Package api contains all functions and dtos Split APIs
package api

import (
	"encoding/json"
	"fmt"
	"testing"
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
  "configurations": {"on":"{\"size\":15}"},
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

var segmentMock = `
{
  "name": "employees",
  "added": [
    "user_for_testing_do_no_erase"
  ],
  "removed": [],
  "since": -1,
  "till": 1489542661161
}`

func TestSplitDTO(t *testing.T) {
	mockedData := fmt.Sprintf(splitsMock, splitMock)

	var splitChangesDtoFromMock SplitChangesDTO
	var splitChangesDtoFromMarshal SplitChangesDTO

	err := json.Unmarshal([]byte(mockedData), &splitChangesDtoFromMock)
	if err != nil {
		t.Error("Error parsing split changes JSON ", err)
	}

	if dataSerialize, err := splitChangesDtoFromMock.Splits[0].MarshalBinary(); err != nil {
		t.Error(err)
	} else {
		marshalData := fmt.Sprintf(splitsMock, dataSerialize)
		err2 := json.Unmarshal([]byte(marshalData), &splitChangesDtoFromMarshal)
		if err2 != nil {
			t.Error("Error parsing split changes JSON ", err)
		}

		if splitChangesDtoFromMarshal.Splits[0].ChangeNumber !=
			splitChangesDtoFromMock.Splits[0].ChangeNumber {
			t.Error("Marshal struct mal formed [ChangeNumber]")
		}

		if splitChangesDtoFromMarshal.Splits[0].Name !=
			splitChangesDtoFromMock.Splits[0].Name {
			t.Error("Marshal struct mal formed [Name]")
		}

		if splitChangesDtoFromMarshal.Splits[0].Killed !=
			splitChangesDtoFromMock.Splits[0].Killed {
			t.Error("Marshal struct mal formed [Killed]")
		}

	}
}

func TestSplitDTOWithConfigs(t *testing.T) {
	mockedData := fmt.Sprintf(splitsMock, splitMock)

	var splitChangesDtoFromMock SplitChangesDTO
	var splitChangesDtoFromMarshal SplitChangesDTO

	err := json.Unmarshal([]byte(mockedData), &splitChangesDtoFromMock)
	if err != nil {
		t.Error("Error parsing split changes JSON ", err)
	}

	if dataSerialize, err := splitChangesDtoFromMock.Splits[0].MarshalBinary(); err != nil {
		t.Error(err)
	} else {
		marshalData := fmt.Sprintf(splitsMock, dataSerialize)
		err2 := json.Unmarshal([]byte(marshalData), &splitChangesDtoFromMarshal)
		if err2 != nil {
			t.Error("Error parsing split changes JSON ", err)
		}

		if splitChangesDtoFromMarshal.Splits[0].ChangeNumber !=
			splitChangesDtoFromMock.Splits[0].ChangeNumber {
			t.Error("Marshal struct mal formed [ChangeNumber]")
		}

		if splitChangesDtoFromMarshal.Splits[0].Name !=
			splitChangesDtoFromMock.Splits[0].Name {
			t.Error("Marshal struct mal formed [Name]")
		}

		if splitChangesDtoFromMarshal.Splits[0].Killed !=
			splitChangesDtoFromMock.Splits[0].Killed {
			t.Error("Marshal struct mal formed [Killed]")
		}

		if string(splitChangesDtoFromMarshal.Splits[0].Configurations["on"]) !=
			string(splitChangesDtoFromMock.Splits[0].Configurations["on"]) {
			t.Error("Marshal struct mal formed [Configurations]")
		}
	}
}

func TestImpressionDTO(t *testing.T) {
	impressionTXT := `{"keyName":"some_key","treatment":"off","time":1234567890,"changeNumber":55555555,"label":"some label","bucketingKey":"some_bucket_key"}`
	impressionDTO := &ImpressionDTO{
		KeyName:      "some_key",
		Treatment:    "off",
		Time:         1234567890,
		ChangeNumber: 55555555,
		Label:        "some label",
		BucketingKey: "some_bucket_key"}

	marshalImpression, err := impressionDTO.MarshalBinary()
	if err != nil {
		t.Error(err)
	}

	if string(marshalImpression) != impressionTXT {
		t.Error("Error marshaling impression")
	}

}
