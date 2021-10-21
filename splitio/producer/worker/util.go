package worker

import (
	"fmt"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
)

func toImpressionsDTO(impressionsMap impressionsByFeature) ([]dtos.ImpressionsDTO, error) {
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

func wrapData(
	impressions []dtos.Impression,
	collectedData impressionsByMetadataByFeature,
	metadata dtos.Metadata,
) impressionsByMetadataByFeature {
	for _, impression := range impressions { // To prevent errors use range instead of first element
		_, instanceExists := collectedData[metadata]
		if !instanceExists {
			collectedData[metadata] = make(impressionsByFeature)
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

func wrapDataForListener(
	impressions []dtos.Impression,
	collectedData listenerImpressionsByMetadataByFeature, metadata dtos.Metadata,
) listenerImpressionsByMetadataByFeature {
	for _, impression := range impressions { // To prevent errors use range instead of first element
		_, instanceExists := collectedData[metadata]
		if !instanceExists {
			collectedData[metadata] = make(listenerImpressionsByFeature)
		}
		_, featureExists := collectedData[metadata][impression.FeatureName]
		if !featureExists {
			collectedData[metadata][impression.FeatureName] = make([]common.ImpressionForListener, 0)
		}
		collectedData[metadata][impression.FeatureName] = append(
			collectedData[metadata][impression.FeatureName],
			common.ImpressionForListener{
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

func toListenerDTO(impressionsMap listenerImpressionsByFeature) ([]common.ImpressionsForListener, error) {
	if impressionsMap == nil {
		return nil, fmt.Errorf("Impressions map cannot be null")
	}

	toReturn := make([]common.ImpressionsForListener, 0)
	for feature, impressions := range impressionsMap {
		toReturn = append(toReturn, common.ImpressionsForListener{
			TestName:       feature,
			KeyImpressions: impressions,
		})
	}
	return toReturn, nil
}

func wrapDTOListener(collectedData listenerImpressionsByMetadataByFeature) listenerImpressionsByMetadata {
	var err error
	impressions := make(map[dtos.Metadata][]common.ImpressionsForListener)
	for metadata, impsForMetadata := range collectedData {
		impressions[metadata], err = toListenerDTO(impsForMetadata)
		if err != nil {
			continue
		}
	}
	return impressions
}
