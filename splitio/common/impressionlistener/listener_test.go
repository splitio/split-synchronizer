package impressionlistener

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/stretchr/testify/assert"
)

func TestImpressionListener(t *testing.T) {

	reqsDone := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { reqsDone <- struct{}{} }()
		// Verify request method and path
		assert.False(t, r.URL.Path != "/someUrl" && r.Method != "POST", "Invalid request. Should be POST to /someUrl")

		// Read and parse request body
		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		assert.False(t, err != nil, "Error reading body")
		if err != nil {
			return
		}

		var all impressionListenerPostBody
		err = json.Unmarshal(body, &all)
		assert.False(t, err != nil, "Error parsing json: %v", err)
		if err != nil {
			return
		}

		// Verify metadata
		assert.False(t, all.SdkVersion != "go-1.1.1" || all.MachineIP != "1.2.3.4" || all.MachineName != "ip-1-2-3-4", "invalid metadata")

		// Verify impressions
		imps := all.Impressions
		assert.False(t, len(imps) != 2, "invalid number of impression groups received")
		if len(imps) != 2 {
			return
		}

		// Verify first impression group (t1)
		assert.False(t, imps[0].TestName != "t1" || len(imps[0].KeyImpressions) != 2, "invalid ipmressions for t1")
		if imps[0].TestName != "t1" || len(imps[0].KeyImpressions) != 2 {
			return
		}

		// Verify first impression of t1
		assert.Equal(t, "k1", imps[0].KeyImpressions[0].KeyName, "t1 first impression should have correct key name")
		assert.Equal(t, "on", imps[0].KeyImpressions[0].Treatment, "t1 first impression should have correct treatment")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[0].Time, "t1 first impression should have correct time")
		assert.Equal(t, int64(2), imps[0].KeyImpressions[0].ChangeNumber, "t1 first impression should have correct change number")
		assert.Equal(t, "l1", imps[0].KeyImpressions[0].Label, "t1 first impression should have correct label")
		assert.Equal(t, "b1", imps[0].KeyImpressions[0].BucketingKey, "t1 first impression should have correct bucketing key")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[0].Pt, "t1 first impression should have correct pt")

		// Verify second impression of t1
		assert.Equal(t, "k2", imps[0].KeyImpressions[1].KeyName, "t1 second impression should have correct key name")
		assert.Equal(t, "on", imps[0].KeyImpressions[1].Treatment, "t1 second impression should have correct treatment")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[1].Time, "t1 second impression should have correct time")
		assert.Equal(t, int64(2), imps[0].KeyImpressions[1].ChangeNumber, "t1 second impression should have correct change number")
		assert.Equal(t, "l1", imps[0].KeyImpressions[1].Label, "t1 second impression should have correct label")
		assert.Equal(t, "b1", imps[0].KeyImpressions[1].BucketingKey, "t1 second impression should have correct bucketing key")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[1].Pt, "t1 second impression should have correct pt")

		// Verify second impression group (t2)
		assert.False(t, imps[1].TestName != "t2" || len(imps[1].KeyImpressions) != 2, "invalid ipmressions for t2")
		if imps[1].TestName != "t2" || len(imps[1].KeyImpressions) != 2 {
			return
		}

		// Verify first impression of t2
		assert.Equal(t, "k1", imps[1].KeyImpressions[0].KeyName, "t2 first impression should have correct key name")
		assert.Equal(t, "off", imps[1].KeyImpressions[0].Treatment, "t2 first impression should have correct treatment")
		assert.Equal(t, int64(2), imps[1].KeyImpressions[0].Time, "t2 first impression should have correct time")
		assert.Equal(t, int64(3), imps[1].KeyImpressions[0].ChangeNumber, "t2 first impression should have correct change number")
		assert.Equal(t, "l2", imps[1].KeyImpressions[0].Label, "t2 first impression should have correct label")
		assert.Equal(t, "b2", imps[1].KeyImpressions[0].BucketingKey, "t2 first impression should have correct bucketing key")
		assert.Equal(t, int64(2), imps[1].KeyImpressions[0].Pt, "t2 first impression should have correct pt")

		// Verify second impression of t2
		assert.Equal(t, "k2", imps[1].KeyImpressions[1].KeyName, "t2 second impression should have correct key name")
		assert.Equal(t, "off", imps[1].KeyImpressions[1].Treatment, "t2 second impression should have correct treatment")
		assert.Equal(t, int64(2), imps[1].KeyImpressions[1].Time, "t2 second impression should have correct time")
		assert.Equal(t, int64(3), imps[1].KeyImpressions[1].ChangeNumber, "t2 second impression should have correct change number")
		assert.Equal(t, "l2", imps[1].KeyImpressions[1].Label, "t2 second impression should have correct label")
		assert.Equal(t, "b2", imps[1].KeyImpressions[1].BucketingKey, "t2 second impression should have correct bucketing key")
		assert.Equal(t, int64(3), imps[1].KeyImpressions[1].Pt, "t2 second impression should have correct pt")
	}))
	defer ts.Close()

	listener, err := NewImpressionBulkListener(ts.URL, 10, nil)
	assert.False(t, err != nil, "error cannot be nil: %v", err)

	err = listener.Start()
	assert.False(t, err != nil, "start() should not fail. Got: %v", err)
	defer listener.Stop(true)

	listener.Submit([]ImpressionsForListener{
		ImpressionsForListener{
			TestName: "t1",
			KeyImpressions: []ImpressionForListener{
				ImpressionForListener{
					KeyName:      "k1",
					Treatment:    "on",
					Time:         1,
					ChangeNumber: 2,
					Label:        "l1",
					BucketingKey: "b1",
					Pt:           1,
				},
				ImpressionForListener{
					KeyName:      "k2",
					Treatment:    "on",
					Time:         1,
					ChangeNumber: 2,
					Label:        "l1",
					BucketingKey: "b1",
					Pt:           1,
				},
			},
		},
		ImpressionsForListener{
			TestName: "t2",
			KeyImpressions: []ImpressionForListener{
				ImpressionForListener{
					KeyName:      "k1",
					Treatment:    "off",
					Time:         2,
					ChangeNumber: 3,
					Label:        "l2",
					BucketingKey: "b2",
					Pt:           2,
				},
				ImpressionForListener{
					KeyName:      "k2",
					Treatment:    "off",
					Time:         2,
					ChangeNumber: 3,
					Label:        "l2",
					BucketingKey: "b2",
					Pt:           3,
				},
			},
		},
	}, &dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"})

	<-reqsDone
}

func TestImpressionListenerWithProperties(t *testing.T) {

	reqsDone := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { reqsDone <- struct{}{} }()
		// Verify request method and path
		assert.False(t, r.URL.Path != "/someUrl" && r.Method != "POST", "Invalid request. Should be POST to /someUrl")

		// Read and parse request body
		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		assert.False(t, err != nil, "Error reading body")
		if err != nil {
			return
		}

		var all impressionListenerPostBody
		err = json.Unmarshal(body, &all)
		assert.False(t, err != nil, "Error parsing json: %v", err)
		if err != nil {
			return
		}

		// Verify metadata
		assert.False(t, all.SdkVersion != "go-1.1.1" || all.MachineIP != "1.2.3.4" || all.MachineName != "ip-1-2-3-4", "invalid metadata")

		// Verify impressions
		imps := all.Impressions
		assert.False(t, len(imps) != 2, "invalid number of impression groups received")
		if len(imps) != 2 {
			return
		}

		// Verify first impression group (t1)
		assert.False(t, imps[0].TestName != "t1" || len(imps[0].KeyImpressions) != 2, "invalid ipmressions for t1")
		if imps[0].TestName != "t1" || len(imps[0].KeyImpressions) != 2 {
			return
		}

		// Verify first impression of t1
		assert.Equal(t, "k1", imps[0].KeyImpressions[0].KeyName, "t1 first impression should have correct key name")
		assert.Equal(t, "on", imps[0].KeyImpressions[0].Treatment, "t1 first impression should have correct treatment")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[0].Time, "t1 first impression should have correct time")
		assert.Equal(t, int64(2), imps[0].KeyImpressions[0].ChangeNumber, "t1 first impression should have correct change number")
		assert.Equal(t, "l1", imps[0].KeyImpressions[0].Label, "t1 first impression should have correct label")
		assert.Equal(t, "b1", imps[0].KeyImpressions[0].BucketingKey, "t1 first impression should have correct bucketing key")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[0].Pt, "t1 first impression should have correct pt")
		assert.Equal(t, "{'prop':'val'}", imps[0].KeyImpressions[0].Properties, "First impression of t1 should have properties")

		// Verify second impression of t1
		assert.Equal(t, "k2", imps[0].KeyImpressions[1].KeyName, "t1 second impression should have correct key name")
		assert.Equal(t, "on", imps[0].KeyImpressions[1].Treatment, "t1 second impression should have correct treatment")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[1].Time, "t1 second impression should have correct time")
		assert.Equal(t, int64(2), imps[0].KeyImpressions[1].ChangeNumber, "t1 second impression should have correct change number")
		assert.Equal(t, "l1", imps[0].KeyImpressions[1].Label, "t1 second impression should have correct label")
		assert.Equal(t, "b1", imps[0].KeyImpressions[1].BucketingKey, "t1 second impression should have correct bucketing key")
		assert.Equal(t, int64(1), imps[0].KeyImpressions[1].Pt, "t1 second impression should have correct pt")
		// Second impression should not have properties
		assert.Empty(t, imps[0].KeyImpressions[1].Properties, "Second impression of t1 should not have properties")

		// Verify second impression group (t2)
		assert.False(t, imps[1].TestName != "t2" || len(imps[1].KeyImpressions) != 2, "invalid ipmressions for t2")
		if imps[1].TestName != "t2" || len(imps[1].KeyImpressions) != 2 {
			return
		}

		// Verify first impression of t2
		assert.Equal(t, "k1", imps[1].KeyImpressions[0].KeyName, "t2 first impression should have correct key name")
		assert.Equal(t, "off", imps[1].KeyImpressions[0].Treatment, "t2 first impression should have correct treatment")
		assert.Equal(t, int64(2), imps[1].KeyImpressions[0].Time, "t2 first impression should have correct time")
		assert.Equal(t, int64(3), imps[1].KeyImpressions[0].ChangeNumber, "t2 first impression should have correct change number")
		assert.Equal(t, "l2", imps[1].KeyImpressions[0].Label, "t2 first impression should have correct label")
		assert.Equal(t, "b2", imps[1].KeyImpressions[0].BucketingKey, "t2 first impression should have correct bucketing key")
		assert.Equal(t, int64(2), imps[1].KeyImpressions[0].Pt, "t2 first impression should have correct pt")

		// Verify second impression of t2
		assert.Equal(t, "k2", imps[1].KeyImpressions[1].KeyName, "t2 second impression should have correct key name")
		assert.Equal(t, "off", imps[1].KeyImpressions[1].Treatment, "t2 second impression should have correct treatment")
		assert.Equal(t, int64(2), imps[1].KeyImpressions[1].Time, "t2 second impression should have correct time")
		assert.Equal(t, int64(3), imps[1].KeyImpressions[1].ChangeNumber, "t2 second impression should have correct change number")
		assert.Equal(t, "l2", imps[1].KeyImpressions[1].Label, "t2 second impression should have correct label")
		assert.Equal(t, "b2", imps[1].KeyImpressions[1].BucketingKey, "t2 second impression should have correct bucketing key")
		assert.Equal(t, int64(3), imps[1].KeyImpressions[1].Pt, "t2 second impression should have correct pt")
	}))
	defer ts.Close()

	listener, err := NewImpressionBulkListener(ts.URL, 10, nil)
	assert.False(t, err != nil, "error cannot be nil: %v", err)

	err = listener.Start()
	assert.False(t, err != nil, "start() should not fail. Got: %v", err)
	defer listener.Stop(true)

	listener.Submit([]ImpressionsForListener{
		ImpressionsForListener{
			TestName: "t1",
			KeyImpressions: []ImpressionForListener{
				ImpressionForListener{
					KeyName:      "k1",
					Treatment:    "on",
					Time:         1,
					ChangeNumber: 2,
					Label:        "l1",
					BucketingKey: "b1",
					Pt:           1,
					Properties:   "{'prop':'val'}",
				},
				ImpressionForListener{
					KeyName:      "k2",
					Treatment:    "on",
					Time:         1,
					ChangeNumber: 2,
					Label:        "l1",
					BucketingKey: "b1",
					Pt:           1,
				},
			},
		},
		ImpressionsForListener{
			TestName: "t2",
			KeyImpressions: []ImpressionForListener{
				ImpressionForListener{
					KeyName:      "k1",
					Treatment:    "off",
					Time:         2,
					ChangeNumber: 3,
					Label:        "l2",
					BucketingKey: "b2",
					Pt:           2,
				},
				ImpressionForListener{
					KeyName:      "k2",
					Treatment:    "off",
					Time:         2,
					ChangeNumber: 3,
					Label:        "l2",
					BucketingKey: "b2",
					Pt:           3,
				},
			},
		},
	}, &dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"})

	<-reqsDone
}
