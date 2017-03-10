// Package task contains all agent tasks
package task

import (
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
)

// PostImpressions post impressions to Split Events server
func PostImpressions(impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage,
	impressionsRefreshRate int) {
	for {
		impressionsToSend, err := impressionStorageAdapter.RetrieveImpressions()
		if err != nil {
			log.Error.Println("Error Retrieving ")
		}
		log.Debug.Println(impressionsToSend)

		for sdkVersion, impressionsByMachineIP := range impressionsToSend {
			for machineIP, impressions := range impressionsByMachineIP {
				log.Debug.Println("Posting impressions from ", sdkVersion, machineIP)
				err := impressionsRecorderAdapter.Post(impressions, sdkVersion, machineIP)
				if err != nil {
					log.Error.Println("Error posting impressions", err.Error())
					continue
				}
				log.Debug.Println("Impressions sent")
			}
		}
		time.Sleep(time.Duration(impressionsRefreshRate) * time.Second)
	}

}

/*
SMEMBERS SPLITIO.impressions.martin_redolatti_test
1) "{\"keyName\":\"sarrubia2\",\"treatment\":\"off\",\"time\":1488929278980,\"changeNumber\":1488844876698,\"label\":\"no rule matched\",\"bucketingKey\":57}"
2) "{\"keyName\":\"sarrubia2\",\"treatment\":\"off\",\"time\":1488929275391,\"changeNumber\":1488844876698,\"label\":\"no rule matched\",\"bucketingKey\":885}"
3) "{\"keyName\":\"sarrubia2\",\"treatment\":\"off\",\"time\":1488929280238,\"changeNumber\":1488844876698,\"label\":\"no rule matched\",\"bucketingKey\":430}"


SADD SPLITIO/php-3.3.3/127.0.0.1/impressions.martin_redolatti_test "{\"keyName\":\"sarrubia113\",\"treatment\":\"off\",\"time\":1489100343757,\"changeNumber\":1488844876698,\"label\":\"no rule matched\",\"bucketingKey\":\"bucket123\"}"
SADD SPLITIO/php-3.3.3/127.0.0.1/impressions.martin_redolatti_test "{\"keyName\":\"tincho113\",\"treatment\":\"on\",\"time\":1489100343757,\"changeNumber\":1488844876698,\"label\":\"no rule matched\"}"

SADD SPLITIO/php-5.1.0/127.0.0.1/impressions.martin_redolatti_test "{\"keyName\":\"sarrubia115\",\"treatment\":\"off\",\"time\":1489100360011,\"changeNumber\":1488844876698,\"label\":\"no rule matched\",\"bucketingKey\":\"bucket123\"}"
SADD SPLITIO/php-5.1.0/127.0.0.1/impressions.martin_redolatti_test "{\"keyName\":\"tincho115\",\"treatment\":\"on\",\"time\":1489100360011,\"changeNumber\":1488844876698,\"label\":\"no rule matched\"}"





SMEMBERS nodejs.SPLITIO/nodejs-8.0.0-canary.18/10.0.4.215/impressions.NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch

 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572913112,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572918130,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572917125,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572915120,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572912105,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572916123,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572914116,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572920135,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
 "{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572911100,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"
"{\"feature\":\"NODEJS_REDIS_isOnDateTimeWithAttributeValueThatDoesNotMatch\",\"keyName\":\"littlespoon\",\"treatment\":\"INITIALIZATION_STEP\",\"time\":1488572919132,\"label\":\"in segment all\",\"changeNumber\":1488572887006}"


SADD SPLITIO/nodejs-8.0.0-canary.18/10.0.4.215/impressions.martin_redolatti_test "{\"feature\":\"martin_redolatti_test\",\"keyName\":\"tincho\",\"treatment\":\"on\",\"time\":1488926089326,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"

SMEMBERS nodejs.SPLITIO/nodejs-8.0.0-canary.18/10.0.4.215/impressions.NODEJS_REDIS_severalCalls

"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER86\",\"treatment\":\"V3\",\"time\":1488573382237,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER2\",\"treatment\":\"V3\",\"time\":1488573381841,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER174\",\"treatment\":\"V3\",\"time\":1488573382690,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER195\",\"treatment\":\"V3\",\"time\":1488573382808,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"user_for_testing_do_no_erase\",\"treatment\":\"V2\",\"time\":1488573382351,\"label\":\"in segment employees\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER25\",\"treatment\":\"V3\",\"time\":1488573381952,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"


*/
