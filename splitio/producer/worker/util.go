package worker

import (
	"github.com/splitio/go-split-commons/dtos"
	"golang.org/x/exp/errors/fmt"
)

func toImpressionsDTO(impressionsMap map[string][]dtos.ImpressionDTO) ([]dtos.ImpressionsDTO, error) {
	if impressionsMap == nil {
		return nil, fmt.Errorf("Impressions map cannot be null")
	}

	toReturn := make([]dtos.ImpressionsDTO, 0)
	for feature, impressions := range impressionsMap {
		toReturn = append(toReturn, dtos.ImpressionsDTO{
			TestName:       feature,
			KeyImpressions: impressions,
		})
	}
	return toReturn, nil
}

func wrapData(impressions []dtos.Impression, collectedData map[dtos.Metadata]map[string][]dtos.ImpressionDTO, metadata dtos.Metadata) map[dtos.Metadata]map[string][]dtos.ImpressionDTO {
	for _, impression := range impressions { // To prevent errors use range instead of first element
		_, instanceExists := collectedData[metadata]
		if !instanceExists {
			collectedData[metadata] = make(map[string][]dtos.ImpressionDTO)
		}
		_, featureExists := collectedData[metadata][impression.FeatureName]
		if !featureExists {
			collectedData[metadata][impression.FeatureName] = make([]dtos.ImpressionDTO, 0)
		}
		collectedData[metadata][impression.FeatureName] = append(
			collectedData[metadata][impression.FeatureName],
			dtos.ImpressionDTO{
				BucketingKey: impression.BucketingKey,
				ChangeNumber: impression.ChangeNumber,
				KeyName:      impression.KeyName,
				Label:        impression.Label,
				Time:         impression.Time,
				Treatment:    impression.Treatment,
				Pt:           impression.Pt,
			},
		)
	}
	return collectedData
}

type impressionListener struct {
	KeyName      string `json:"keyName"`
	Treatment    string `json:"treatment"`
	Time         int64  `json:"time"`
	ChangeNumber int64  `json:"changeNumber"`
	Label        string `json:"label"`
	BucketingKey string `json:"bucketingKey,omitempty"`
	Pt           int64  `json:"pt,omitempty"`
}

type impressionsListener struct {
	TestName       string               `json:"testName"`
	KeyImpressions []impressionListener `json:"keyImpressions"`
}

func wrapDataForListener(impressions []dtos.Impression, collectedData map[dtos.Metadata]map[string][]impressionListener, metadata dtos.Metadata) map[dtos.Metadata]map[string][]impressionListener {
	for _, impression := range impressions { // To prevent errors use range instead of first element
		_, instanceExists := collectedData[metadata]
		if !instanceExists {
			collectedData[metadata] = make(map[string][]impressionListener)
		}
		_, featureExists := collectedData[metadata][impression.FeatureName]
		if !featureExists {
			collectedData[metadata][impression.FeatureName] = make([]impressionListener, 0)
		}
		collectedData[metadata][impression.FeatureName] = append(
			collectedData[metadata][impression.FeatureName],
			impressionListener{
				BucketingKey: impression.BucketingKey,
				ChangeNumber: impression.ChangeNumber,
				KeyName:      impression.KeyName,
				Label:        impression.Label,
				Time:         impression.Time,
				Treatment:    impression.Treatment,
				Pt:           impression.Pt,
			},
		)
	}
	return collectedData
}

func toListenerDTO(impressionsMap map[string][]impressionListener) ([]impressionsListener, error) {
	if impressionsMap == nil {
		return nil, fmt.Errorf("Impressions map cannot be null")
	}

	toReturn := make([]impressionsListener, 0)
	for feature, impressions := range impressionsMap {
		toReturn = append(toReturn, impressionsListener{
			TestName:       feature,
			KeyImpressions: impressions,
		})
	}
	return toReturn, nil
}

func wrapDTOListener(collectedData map[dtos.Metadata]map[string][]impressionListener) map[dtos.Metadata][]impressionsListener {
	var err error
	impressions := make(map[dtos.Metadata][]impressionsListener)
	for metadata, impsForMetadata := range collectedData {
		impressions[metadata], err = toListenerDTO(impsForMetadata)
		if err != nil {
			continue
		}
	}
	return impressions
}
