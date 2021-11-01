package worker

import (
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
)

type beImpressionsByFeature = map[string]dtos.ImpressionsDTO
type beImpressionsByMetadataAndFeature = map[dtos.Metadata]beImpressionsByFeature
type listenerImpressionsByFeature = map[string]impressionlistener.ImpressionsForListener
type listenerImpressionsByMetadataAndFeature = map[dtos.Metadata]listenerImpressionsByFeature

type bePayloadBuilder struct {
	accum beImpressionsByMetadataAndFeature
}

func makeBePayloadBuilder() *bePayloadBuilder {
	return &bePayloadBuilder{accum: make(beImpressionsByMetadataAndFeature)}
}

func (b *bePayloadBuilder) add(imp *dtos.Impression, metadata *dtos.Metadata) {
	forMetadata := b.accum[*metadata]
	if forMetadata == nil {
		forMetadata = make(map[string]dtos.ImpressionsDTO)
	}

	curr := forMetadata[imp.FeatureName]
	curr.TestName = imp.FeatureName
	curr.KeyImpressions = append(curr.KeyImpressions, dtos.ImpressionDTO{
		KeyName:      imp.KeyName,
		Treatment:    imp.Treatment,
		Time:         imp.Time,
		ChangeNumber: imp.ChangeNumber,
		Label:        imp.Label,
		BucketingKey: imp.BucketingKey,
		Pt:           imp.Pt,
	})
	forMetadata[imp.FeatureName] = curr
	b.accum[*metadata] = forMetadata
}

type listenerPayloadBuilder struct {
	accum listenerImpressionsByMetadataAndFeature
}

func makeListenerPayloadBuilder() *listenerPayloadBuilder {
	return &listenerPayloadBuilder{accum: make(listenerImpressionsByMetadataAndFeature)}
}

func (l *listenerPayloadBuilder) add(imp *dtos.Impression, metadata *dtos.Metadata) {
	forMetadata := l.accum[*metadata]
	if forMetadata == nil {
		forMetadata = make(map[string]impressionlistener.ImpressionsForListener)
	}

	curr := forMetadata[imp.FeatureName]
	curr.TestName = imp.FeatureName
	curr.KeyImpressions = append(curr.KeyImpressions, impressionlistener.ImpressionForListener{
		KeyName:      imp.KeyName,
		Treatment:    imp.Treatment,
		Time:         imp.Time,
		ChangeNumber: imp.ChangeNumber,
		Label:        imp.Label,
		BucketingKey: imp.BucketingKey,
		Pt:           imp.Pt,
	})
	forMetadata[imp.FeatureName] = curr
	l.accum[*metadata] = forMetadata
}

func toTestImpressionsSlice(imps beImpressionsByFeature) []dtos.ImpressionsDTO {
	slc := make([]dtos.ImpressionsDTO, 0, len(imps))
	for _, testImpressions := range imps {
		slc = append(slc, testImpressions)
	}
	return slc
}

func toListenerImpressionsSlice(imps listenerImpressionsByFeature) []impressionlistener.ImpressionsForListener {
	slc := make([]impressionlistener.ImpressionsForListener, 0, len(imps))
	for _, testImpressions := range imps {
		slc = append(slc, testImpressions)
	}
	return slc
}
