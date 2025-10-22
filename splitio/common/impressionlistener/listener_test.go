package impressionlistener

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/go-split-commons/v8/dtos"
)

func TestImpressionListener(t *testing.T) {

	reqsDone := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { reqsDone <- struct{}{} }()
		if r.URL.Path != "/someUrl" && r.Method != "POST" {
			t.Error("Invalid request. Should be POST to /someUrl")
		}

		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Error("Error reading body")
			return
		}

		var all impressionListenerPostBody
		err = json.Unmarshal(body, &all)
		if err != nil {
			t.Errorf("Error parsing json: %s", err)
			return
		}

		if all.SdkVersion != "go-1.1.1" || all.MachineIP != "1.2.3.4" || all.MachineName != "ip-1-2-3-4" {
			t.Error("invalid metadata")
		}

		imps := all.Impressions
		if len(imps) != 2 {
			t.Error("invalid number of impression groups received")
			return
		}

		if imps[0].TestName != "t1" || len(imps[0].KeyImpressions) != 2 {
			t.Errorf("invalid ipmressions for t1")
			return
		}

		if imps[1].TestName != "t2" || len(imps[1].KeyImpressions) != 2 {
			t.Errorf("invalid ipmressions for t2")
			return
		}
	}))
	defer ts.Close()

	listener, err := NewImpressionBulkListener(ts.URL, 10, nil)
	if err != nil {
		t.Error("error cannot be nil: ", err)
	}

	if err = listener.Start(); err != nil {
		t.Error("start() should not fail. Got: ", err)
	}
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
