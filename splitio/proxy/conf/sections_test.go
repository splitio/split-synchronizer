package conf

import (
	"reflect"
	"strings"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
)

// walkStructFields visits every struct field reachable from t, recursing into
// nested structs and slice/pointer element types.
func walkStructFields(t reflect.Type, visit func(reflect.StructField)) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		visit(f)
		walkStructFields(f.Type, visit)
	}
}

func TestNoMisspelledDescriptionTags(t *testing.T) {
	walkStructFields(reflect.TypeOf(Main{}), func(f reflect.StructField) {
		if _, ok := f.Tag.Lookup("s-dec"); ok {
			t.Errorf("field %s uses misspelled tag s-dec; use s-desc", f.Name)
		}
	})
}

func TestBufferOptionsHaveDescriptions(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{name: "ImpressionsBuffer", field: "ImpressionsBuffer"},
		{name: "EventsBuffer", field: "EventsBuffer"},
		{name: "TelemetryBuffer", field: "TelemetryBuffer"},
	}

	syncField, ok := reflect.TypeOf(Main{}).FieldByName("Sync")
	if !ok {
		t.Fatal("Main has no Sync field")
	}
	advanced, ok := syncField.Type.FieldByName("Advanced")
	if !ok {
		t.Fatal("Sync has no Advanced field")
	}
	advancedType := advanced.Type

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := advancedType.FieldByName(tc.field)
			if !ok {
				t.Fatalf("field %s not found", tc.field)
			}
			if f.Tag.Get("s-desc") == "" {
				t.Errorf("field %s has no s-desc tag", tc.field)
			}
		})
	}
}

func TestProxyRedisStateDefaults(t *testing.T) {
	var m Main
	if err := conf.PopulateDefaults(&m); err != nil {
		t.Fatalf("PopulateDefaults returned error: %v", err)
	}

	rs := m.Storage.Redis
	if rs.Enabled {
		t.Errorf("Enabled: expected false, got true")
	}
	if rs.Namespace != "" {
		t.Errorf("Namespace: expected empty, got %q", rs.Namespace)
	}
	if rs.RetentionHours != 168 {
		t.Errorf("RetentionHours: expected 168, got %d", rs.RetentionHours)
	}
	if rs.WriteMaxAttempts != 3 {
		t.Errorf("WriteMaxAttempts: expected 3, got %d", rs.WriteMaxAttempts)
	}
	if rs.WriteBackoffMaxMs != 60000 {
		t.Errorf("WriteBackoffMaxMs: expected 60000, got %d", rs.WriteBackoffMaxMs)
	}
	if rs.MaxPendingChanges != 3000000 {
		t.Errorf("MaxPendingChanges: expected 3000000, got %d", rs.MaxPendingChanges)
	}
	if rs.ChunkSize != 1000 {
		t.Errorf("ChunkSize: expected 1000, got %d", rs.ChunkSize)
	}
	if rs.PurgeBatch != 500 {
		t.Errorf("PurgeBatch: expected 500, got %d", rs.PurgeBatch)
	}
	if rs.StopTimeoutMs != 10000 {
		t.Errorf("StopTimeoutMs: expected 10000, got %d", rs.StopTimeoutMs)
	}
}

func TestProxyRedisStateHelpMatchesBehavior(t *testing.T) {
	rt := reflect.TypeOf(RedisState{})

	mpc, ok := rt.FieldByName("MaxPendingChanges")
	if !ok {
		t.Fatal("MaxPendingChanges field not found")
	}
	if desc := mpc.Tag.Get("s-desc"); strings.Contains(desc, "rewrites its whole state") {
		t.Errorf("MaxPendingChanges s-desc is stale: %q", desc)
	}

	wma, ok := rt.FieldByName("WriteMaxAttempts")
	if !ok {
		t.Fatal("WriteMaxAttempts field not found")
	}
	if desc := wma.Tag.Get("s-desc"); strings.Contains(desc, "rounds repeat forever") || !strings.Contains(desc, "dropped") || !strings.Contains(desc, "next resync repairs") {
		t.Errorf("WriteMaxAttempts s-desc must say the change is dropped and the next resync repairs it: %q", desc)
	}

	for _, name := range []string{"ChunkSize", "PurgeBatch", "StopTimeoutMs"} {
		f, ok := rt.FieldByName(name)
		if !ok {
			t.Errorf("field %s not found", name)
			continue
		}
		if f.Tag.Get("s-desc") == "" {
			t.Errorf("field %s has empty s-desc", name)
		}
	}

	bounds := map[string]string{"ChunkSize": "10000", "PurgeBatch": "5000", "StopTimeoutMs": "60000"}
	for name, bound := range bounds {
		f, _ := rt.FieldByName(name)
		if !strings.Contains(f.Tag.Get("s-desc"), bound) {
			t.Errorf("field %s s-desc does not state its upper bound %s", name, bound)
		}
	}
}

func TestProxyRedisStateFlagNames(t *testing.T) {
	argMap := conf.MakeCliArgMapFor(&Main{})

	expected := []string{
		"redis-state-enabled",
		"redis-state-namespace",
		"redis-state-retention-hours",
		"redis-state-write-max-attempts",
		"redis-state-write-backoff-max-ms",
		"redis-state-max-pending-changes",
		"redis-state-chunk-size",
		"redis-state-purge-batch",
		"redis-state-stop-timeout-ms",
	}
	for _, name := range expected {
		if _, ok := argMap[name]; !ok {
			t.Errorf("expected CLI flag %q to be registered", name)
		}
	}

	if _, ok := argMap["storage-redis-state-enabled"]; ok {
		t.Errorf("unexpected prefixed flag storage-redis-state-enabled")
	}
}
