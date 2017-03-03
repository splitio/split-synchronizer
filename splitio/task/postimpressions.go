// Package task contains all agent tasks
package task

import (
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage"
)

// PostImpressions post impressions to Split Events server
func PostImpressions(impressionStorageAdapter storage.ImpressionStorage) {
	impressionsToSend, err := impressionStorageAdapter.RetrieveImpressions()
	if err != nil {
		log.Error.Println("Error Retrieving ")
	}
	log.Debug.Println(impressionsToSend)
}

/*

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


SADD nodejs.SPLITIO/nodejs-8.0.0-canary.18/10.0.4.215/impressions.NODEJS_REDIS_severalCalls "{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER86\",\"treatment\":\"V3\",\"time\":1488573382237,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"

SMEMBERS nodejs.SPLITIO/nodejs-8.0.0-canary.18/10.0.4.215/impressions.NODEJS_REDIS_severalCalls

"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER86\",\"treatment\":\"V3\",\"time\":1488573382237,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER2\",\"treatment\":\"V3\",\"time\":1488573381841,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER174\",\"treatment\":\"V3\",\"time\":1488573382690,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER195\",\"treatment\":\"V3\",\"time\":1488573382808,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"user_for_testing_do_no_erase\",\"treatment\":\"V2\",\"time\":1488573382351,\"label\":\"in segment employees\",\"changeNumber\":1488573357392}"
"{\"feature\":\"NODEJS_REDIS_severalCalls\",\"keyName\":\"USER25\",\"treatment\":\"V3\",\"time\":1488573381952,\"label\":\"in segment all\",\"changeNumber\":1488573357392}"


*/
