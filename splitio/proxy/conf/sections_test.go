package conf

import (
	"reflect"
	"testing"
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
