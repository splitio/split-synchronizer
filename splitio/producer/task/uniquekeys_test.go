package task

import (
	"testing"

	"github.com/splitio/go-split-commons/v4/dtos"
)

func TestFilter(t *testing.T) {
	slice := getUniqueMocks()
	filter := make(map[string]map[string]bool)

	for _, u := range slice {
		addUniqueToFilter(u, filter)
	}

	for _, u := range slice {
		addUniqueToFilter(u, filter)
	}

	uniques := buildUniquesObj(filter)

	if len(uniques.Keys) != 3 {
		t.Error("Keys len should be 3")
	}

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

func getUniqueMocks() []dtos.Uniques {
	one := dtos.Uniques{
		Keys: []dtos.Key{
			{
				Feature: "feature-1",
				Keys:    []string{"key-1", "key-2"},
			},
			{
				Feature: "feature-2",
				Keys:    []string{"key-10", "key-20"},
			},
		},
	}

	two := dtos.Uniques{
		Keys: []dtos.Key{
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
		},
	}

	three := dtos.Uniques{
		Keys: []dtos.Key{
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
		},
	}

	return []dtos.Uniques{one, two, three}
}
