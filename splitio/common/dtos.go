package common

// ImpressionListener struct for payload
type ImpressionListener struct {
	KeyName      string `json:"keyName"`
	Treatment    string `json:"treatment"`
	Time         int64  `json:"time"`
	ChangeNumber int64  `json:"changeNumber"`
	Label        string `json:"label"`
	BucketingKey string `json:"bucketingKey,omitempty"`
	Pt           int64  `json:"pt,omitempty"`
}

// ImpressionsListener struct for payload
type ImpressionsListener struct {
	TestName       string               `json:"testName"`
	KeyImpressions []ImpressionListener `json:"keyImpressions"`
}
