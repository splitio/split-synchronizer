package worker

import (
	"errors"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/recorder"
)

var impressionListenerStream = make(chan *ImpressionBulk, recorder.ImpressionListenerMainQueueSize)

// QueueImpressionsForListener Impression Listener for Synchronizer
func QueueImpressionsForListener(impressions *ImpressionBulk) error {
	select {
	case impressionListenerStream <- impressions:
		return nil
	default:
		return errors.New("Impression listener queue is full. Last bulk not added")
	}
}

func queueFailedImpressions(failedQueue chan *ImpressionBulk, msg *ImpressionBulk) {
	select {
	case failedQueue <- msg:
	default:
		log.Error.Println("Impression listener failed queue is full. " +
			"Impressions will be dropped until the listener enpoint is restored.")
	}
}

func taskPostImpressionsToListener(ilSubmitter recorder.ImpressionListenerSubmitter, failedQueue chan *ImpressionBulk) {
	failedImpressions := true
	for failedImpressions {
		select {
		case msg := <-failedQueue:
			err := ilSubmitter.Post(msg.Data, msg.SdkVersion, msg.MachineIP, msg.MachineName)
			if err != nil {
				msg.attempt++
				if msg.attempt < 3 {
					queueFailedImpressions(failedQueue, msg)
				}
				time.Sleep(time.Millisecond * 100)
			}
		default:
			failedImpressions = false
		}
	}
	msg := <-impressionListenerStream
	err := ilSubmitter.Post(msg.Data, msg.SdkVersion, msg.MachineIP, msg.MachineName)
	if err != nil {
		queueFailedImpressions(failedQueue, msg)
	}
}

// PostImpressionsToListener Add Impressions to Listener
func PostImpressionsToListener(ilSubmitter recorder.ImpressionListenerSubmitter) {
	var failedQueue = make(chan *ImpressionBulk, recorder.ImpressionListenerFailedQueueSize)
	for {
		taskPostImpressionsToListener(ilSubmitter, failedQueue)
		time.Sleep(time.Duration(100) * time.Millisecond)
	}
}
