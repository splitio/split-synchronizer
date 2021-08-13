package proxy

import (
	"fmt"
	"github.com/splitio/go-toolkit/v4/backoff"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/snapshot"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v3/service"
	"github.com/splitio/go-split-commons/v3/service/api"
	"github.com/splitio/go-split-commons/v3/synchronizer"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/metric"
	"github.com/splitio/go-split-commons/v3/tasks"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/fetcher"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/interfaces"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v4/splitio/recorder"
	"github.com/splitio/split-synchronizer/v4/splitio/task"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
)

func gracefulShutdownProxy(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup, syncManager synchronizer.Manager) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	// Events - Emit task stop signal
	fmt.Println(" -> Sending STOP to impression posting goroutine")
	controllers.StopEventsRecording()

	// Impressions - Emit task stop signal
	fmt.Println(" -> Sending STOP to event posting goroutine")
	controllers.StopImpressionsRecording()

	// Healthcheck - Emit task stop signal
	fmt.Println(" -> Sending STOP to healthcheck goroutine")
	task.StopHealtcheck()

	// Stopping Sync Manager in charge of PeriodicFetchers and PeriodicRecorders as well as Streaming
	fmt.Println(" -> Sending STOP to Synchronizer")
	syncManager.Stop()

	fmt.Println(" * Waiting goroutines stop")
	gracefulShutdownWaitingGroup.Wait()

	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

// Start initialize in proxy mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	// Initialization of DB
	var startedWithSnapshot = false
	var dbpath = boltdb.InMemoryMode
	if conf.Data.Proxy.Snapshot != "" {
		// Backward compatible. Provided file is not an snapshot, it is a boltdb
		if conf.Data.Proxy.PersistMemoryPath ==  conf.Data.Proxy.Snapshot {
			// if Snapshot is empty, this one is overwritten with conf.Data.Proxy.PersistMemoryPath
			// see the file conf/validator.go
			dbpath = conf.Data.Proxy.Snapshot
			log.Instance.Debug("Database created from boltdb file at", dbpath)
		} else {
			snap, err := snapshot.DecodeFromFile(conf.Data.Proxy.Snapshot)
			if err != nil {
				panic(err)
			}

			dbpath, err = snap.WriteDataToTmpFile()
			if err != nil {
				panic(err)
			}

			log.Instance.Debug("Database created from snapshot at", dbpath)
		}
		startedWithSnapshot = true
	}
	boltdb.Initialize(dbpath, nil)

	// Getting initial config data
	advanced := conf.ParseAdvancedOptions()
	metadata := util.GetMetadata()

	// Initialization common
	interfaces.Initialize()

	// Setup fetchers & recorders
	splitAPI := service.NewSplitAPI(
		conf.Data.APIKey,
		advanced,
		log.Instance,
		metadata,
	)

	// Instantiating storages
	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
	splitStorage := storage.NewSplitStorage(splitCollection)
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segmentStorage := storage.NewSegmentStorage(segmentCollection)

	// Creating Workers and Tasks
	workers := synchronizer.Workers{
		SplitFetcher:      fetcher.NewSplitFetcher(splitCollection, splitAPI.SplitFetcher, interfaces.ProxyTelemetryWrapper, log.Instance),
		SegmentFetcher:    fetcher.NewSegmentFetcher(segmentCollection, splitCollection, splitAPI.SegmentFetcher, interfaces.ProxyTelemetryWrapper, log.Instance),
		TelemetryRecorder: metric.NewRecorderSingle(interfaces.TelemetryStorage, splitAPI.MetricRecorder, metadata),
	}
	splitTasks := synchronizer.SplitTasks{
		SplitSyncTask:     tasks.NewFetchSplitsTask(workers.SplitFetcher, conf.Data.SplitsFetchRate, log.Instance),
		SegmentSyncTask:   tasks.NewFetchSegmentsTask(workers.SegmentFetcher, conf.Data.SegmentFetchRate, advanced.SegmentWorkers, advanced.SegmentQueueSize, log.Instance),
		TelemetrySyncTask: tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, conf.Data.MetricsPostRate, log.Instance),
	}

	// Creating Synchronizer for tasks
	syncImpl := synchronizer.NewSynchronizer(
		advanced,
		splitTasks,
		workers,
		log.Instance,
		nil,
	)

	managerStatus := make(chan int, 1)
	syncManager, err := synchronizer.NewSynchronizerManager(
		syncImpl,
		log.Instance,
		advanced,
		splitAPI.AuthClient,
		splitStorage,
		managerStatus,
	)
	if err != nil {
		panic(err)
	}

	// Proxy mode - graceful shutdown
	go gracefulShutdownProxy(sigs, gracefulShutdownWaitingGroup, syncManager)

	// Run Sync Manager
	go syncManager.Start()
	select {
	case status := <-managerStatus:
		switch status {
		case synchronizer.Ready:
			if !startedWithSnapshot {
				log.Instance.Info("Synchronizer tasks started")
			} else {
				log.Instance.Info("Synchronizer tasks started from snapshot")
			}
		case synchronizer.Error:
			if !startedWithSnapshot {
				os.Exit(splitio.ExitTaskInitialization)
			}
			log.Instance.Warning("Starting from a Snapshot and the Synchronizer tasks cannot be started")
			fmt.Println("* Starting from a Snapshot and the Synchronizer tasks cannot be started")
			// trying to start sync with backoff
			go func() {
				b := backoff.New()
				toSleep := time.Second
				for {
					syncManager.Start()

					status := <-managerStatus
					switch status {
					case synchronizer.Ready:
						log.Instance.Info("Synchronizer tasks started after many attempts")
						return
					}

					time.Sleep(toSleep)
					if toSleep < time.Minute { //if 1 minute backoff is reached do not increment it.
						toSleep = b.Next()
					}
				}
			}()

		}
	}

	if conf.Data.ImpressionListener.Endpoint != "" {
		go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
			Endpoint: conf.Data.ImpressionListener.Endpoint,
		})
	}

	// Initialization routes
	controllers.InitializeImpressionWorkers(
		conf.Data.Proxy.ImpressionsMaxSize,
		int64(conf.Data.ImpressionsPostRate),
		gracefulShutdownWaitingGroup,
	)
	controllers.InitializeEventWorkers(
		conf.Data.Proxy.EventsMaxSize,
		int64(conf.Data.EventsPostRate),
		gracefulShutdownWaitingGroup,
	)
	controllers.InitializeImpressionsCountRecorder()

	httpClients := common.HTTPClients{
		SdkClient:    api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.SdkURL, log.Instance, metadata),
		EventsClient: api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.EventsURL, log.Instance, metadata),
		AuthClient:   api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.AuthServiceURL, log.Instance, metadata),
	}
	proxyOptions := &Options{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   conf.Data.Proxy.Auth.APIKeys,
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
		httpClients:               httpClients,
		splitStorage:              splitStorage,
		segmentStorage:            segmentStorage,
	}

	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, splitStorage, httpClients)

	// Run webserver loop
	Run(proxyOptions)
}
