package task

import (
	"encoding/json"
	"testing"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/provisional/strategy"
	"github.com/splitio/go-split-commons/v8/storage/mocks"
	"github.com/splitio/go-toolkit/v5/logging"
)

func makeSerializedUniquesDto(key dtos.Key) [][]byte {
	result, _ := json.Marshal(key)
	return [][]byte{result}
}

func makeSerializedUniquesArray(slice [][]dtos.Key) [][]byte {
	result := func(r []byte, _ error) []byte { return r }
	uqs := make([][]byte, 0)

	for _, unique := range slice {
		uqs = append(uqs, result(json.Marshal(unique)))
	}

	return uqs
}

func getUniqueMocks() [][]dtos.Key {
	one := []dtos.Key{
		{
			Feature: "feature-1",
			Keys:    []string{"key-1", "key-2"},
		},
		{
			Feature: "feature-2",
			Keys:    []string{"key-10", "key-20"},
		},
	}

	two := []dtos.Key{
		{
			Feature: "feature-1",
			Keys:    []string{"key-1", "key-2", "key-3"},
		},
		{
			Feature: "feature-2",
			Keys:    []string{"key-10", "key-20"},
		},
		{
			Feature: "feature-3",
			Keys:    []string{"key-10", "key-20"},
		},
	}

	three := []dtos.Key{
		{
			Feature: "feature-1",
			Keys:    []string{"key-1", "key-2", "key-3"},
		},
		{
			Feature: "feature-2",
			Keys:    []string{"key-10", "key-20", "key-30", "key-55"},
		},
		{
			Feature: "feature-3",
			Keys:    []string{"key-10", "key-20", "key-40", "key-100", "key-300", "key-10", "key-20", "key-40", "key-100", "key-300"},
		},
	}

	return [][]dtos.Key{one, two, three}
}

func TestUniquesMemoryIsProperlyReturnedDto(t *testing.T) {
	filter := mocks.MockFilter{
		ContainsCall: func(data string) bool { return false },
		AddCall:      func(data string) {},
		ClearCall:    func() {},
	}
	tracker := strategy.NewUniqueKeysTracker(filter)
	worker := NewUniqueKeysWorker(&UniqueWorkerConfig{
		Logger:            logging.NewLogger(nil),
		Storage:           mocks.MockUniqueKeysStorage{},
		UniqueKeysTracker: tracker,
		URL:               "http://test",
		Apikey:            "someApikey",
		FetchSize:         100,
		Metadata: dtos.Metadata{
			SDKVersion:  "sdk-version-test",
			MachineIP:   "ip-test",
			MachineName: "name-test",
		},
	})

	sinker := make(chan interface{}, 100)
	key := dtos.Key{
		Feature: "feature-1",
		Keys:    []string{"key-1", "key-2"},
	}
	dataRaw := makeSerializedUniquesDto(key)
	worker.Process(dataRaw, sinker)

	if len(sinker) != 1 {
		t.Error("there should be 1 bulk ready for submission")
	}
	data := <-sinker
	req, err := worker.BuildRequest(data)

	if req == nil || err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	uniques, _ := data.(dtos.Uniques)
	for _, uk := range uniques.Keys {
		switch uk.Feature {
		case "feature-1":
			if len(uk.Keys) != 2 {
				t.Error("Len should be 2")
			}
		default:
			t.Errorf("Incorrect feature name, %s", uk.Feature)
		}
	}
}

func TestUniquesMemoryIsProperlyReturnedArray(t *testing.T) {
	filter := mocks.MockFilter{
		ContainsCall: func(data string) bool { return false },
		AddCall:      func(data string) {},
		ClearCall:    func() {},
	}
	tracker := strategy.NewUniqueKeysTracker(filter)
	worker := NewUniqueKeysWorker(&UniqueWorkerConfig{
		Logger:            logging.NewLogger(nil),
		Storage:           mocks.MockUniqueKeysStorage{},
		UniqueKeysTracker: tracker,
		URL:               "http://test",
		Apikey:            "someApikey",
		FetchSize:         100,
		Metadata: dtos.Metadata{
			SDKVersion:  "sdk-version-test",
			MachineIP:   "ip-test",
			MachineName: "name-test",
		},
	})

	sinker := make(chan interface{}, 100)
	slice := getUniqueMocks()
	dataRaw := makeSerializedUniquesArray(slice)
	worker.Process(dataRaw, sinker)

	if len(sinker) != 1 {
		t.Error("there should be 1 bulk ready for submission")
	}
	data := <-sinker
	req, err := worker.BuildRequest(data)

	if req == nil || err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	uniques, _ := data.(dtos.Uniques)
	for _, uk := range uniques.Keys {
		switch uk.Feature {
		case "feature-1":
			if len(uk.Keys) != 3 {
				t.Error("Len should be 3")
			}
		case "feature-2":
			if len(uk.Keys) != 4 {
				t.Error("Len should be 4")
			}
		case "feature-3":
			if len(uk.Keys) != 5 {
				t.Error("Len should be 5")
			}
		default:
			t.Errorf("Incorrect feature name, %s", uk.Feature)
		}
	}
}
