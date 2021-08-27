package worker

import (
	"fmt"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
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

func wrapDataForListener(impressions []dtos.Impression, collectedData map[dtos.Metadata]map[string][]common.ImpressionListener, metadata dtos.Metadata) map[dtos.Metadata]map[string][]common.ImpressionListener {
	for _, impression := range impressions { // To prevent errors use range instead of first element
		_, instanceExists := collectedData[metadata]
		if !instanceExists {
			collectedData[metadata] = make(map[string][]common.ImpressionListener)
		}
		_, featureExists := collectedData[metadata][impression.FeatureName]
		if !featureExists {
			collectedData[metadata][impression.FeatureName] = make([]common.ImpressionListener, 0)
		}
		collectedData[metadata][impression.FeatureName] = append(
			collectedData[metadata][impression.FeatureName],
			common.ImpressionListener{
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

func toListenerDTO(impressionsMap map[string][]common.ImpressionListener) ([]common.ImpressionsListener, error) {
	if impressionsMap == nil {
		return nil, fmt.Errorf("Impressions map cannot be null")
	}

	toReturn := make([]common.ImpressionsListener, 0)
	for feature, impressions := range impressionsMap {
		toReturn = append(toReturn, common.ImpressionsListener{
			TestName:       feature,
			KeyImpressions: impressions,
		})
	}
	return toReturn, nil
}

func wrapDTOListener(collectedData map[dtos.Metadata]map[string][]common.ImpressionListener) map[dtos.Metadata][]common.ImpressionsListener {
	var err error
	impressions := make(map[dtos.Metadata][]common.ImpressionsListener)
	for metadata, impsForMetadata := range collectedData {
		impressions[metadata], err = toListenerDTO(impsForMetadata)
		if err != nil {
			continue
		}
	}
	return impressions
}
