package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v2/dtos"
	"github.com/splitio/go-split-commons/v2/util"
	"github.com/splitio/go-toolkit/v3/logging"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
)

func TestPostImpressionsCount(t *testing.T) {
	call := 0
	conf.Initialize()
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")
		sdkMachineName := r.Header.Get("SplitSDKMachineName")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		if sdkMachineName != "SOME_MACHINE_NAME" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsCountInPost dtos.ImpressionsCountDTO
		err := json.Unmarshal(rBody, &impressionsCountInPost)
		if err != nil {
			t.Error(err)
			return
		}

		for _, feature := range impressionsCountInPost.PerFeature {
			switch feature.FeatureName {
			case "some_feature":
				if feature.RawCount != 100 {
					t.Error("Unexpected body")
				}
			case "another_feature":
				if feature.RawCount != 150 {
					t.Error("Unexpected body")
				}
			default:
				t.Error("Unexpected feature")
			}
		}

		call = 1
		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	impCount := []dtos.ImpressionsInTimeFrameDTO{
		{
			FeatureName: "some_feature",
			RawCount:    100,
			TimeFrame:   util.TruncateTimeFrame(time.Now().UTC().UnixNano()),
		},
		{
			FeatureName: "another_feature",
			RawCount:    150,
			TimeFrame:   util.TruncateTimeFrame(time.Now().UTC().UnixNano()),
		},
	}
	imp := dtos.ImpressionsCountDTO{PerFeature: impCount}

	data, err := json.Marshal(imp)
	if err != nil {
		t.Error(err)
		return
	}

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// Init Impressions controller.
	InitializeImpressionsCountRecorder()
	PostImpressionsCount("test-1.0.0", "127.0.0.1", "SOME_MACHINE_NAME", data)

	time.Sleep(300 * time.Millisecond)
	if call != 1 {
		t.Error("It should call post")
	}
}
