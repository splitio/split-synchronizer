package storage

import "github.com/splitio/split-synchronizer/splitio/api"

// ImpressionsByMachineIP maps a list of impressions using as map key the machine IP
type ImpressionsByMachineIP map[string][]api.ImpressionsDTO

// ImpressionsBySDKVersionAndMachineIP maps the above list of impressions (grouped by IP) under <sdk_name-version> key
type ImpressionsBySDKVersionAndMachineIP map[string]ImpressionsByMachineIP
