// Package storage implements different kind of storages for split information
package storage

import "github.com/splitio/go-agent/splitio/api"

type ImpressionsByMachineIP map[string][]api.ImpressionsDTO
type ImpressionsBySDKVersionAndMachineIP map[string]ImpressionsByMachineIP
