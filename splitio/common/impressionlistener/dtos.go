package impressionlistener

// ImpressionForListener struct for payload
type ImpressionForListener struct {
	KeyName      string `json:"keyName"`
	Treatment    string `json:"treatment"`
	Time         int64  `json:"time"`
	ChangeNumber int64  `json:"changeNumber"`
	Label        string `json:"label"`
	BucketingKey string `json:"bucketingKey,omitempty"`
	Pt           int64  `json:"pt,omitempty"`
	Properties   string `json:"properties,omitempty"`
}

// ImpressionsForListener struct for payload
type ImpressionsForListener struct {
	TestName       string                  `json:"testName"`
	KeyImpressions []ImpressionForListener `json:"keyImpressions"`
}
