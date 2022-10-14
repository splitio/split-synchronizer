package conf

import (
	"flag"
	"os"
	"testing"

	"github.com/splitio/go-toolkit/v5/common"
)

type nestedConf struct {
	F1 string `s-cli:"ff1" s-def:"CHAU"`
}

type someConf struct {
	F0 int        `s-cli:"f0" s-def:"42"`
	F1 int64      `s-cli:"f1" s-def:"123"`
	F2 string     `s-cli:"f2" s-def:"HOLA"`
	F3 bool       `s-cli:"f3" s-def:"false"`
	F4 []string   `s-cli:"f4" s-def:"e1,e2"`
	F5 nestedConf `s-nested:"true"`
}

func TestArgMap(t *testing.T) {
	m := make(ArgMap)
	m["e0"] = common.IntRef(42)
	m["e1"] = common.Int64Ref(123)
	m["e2"] = common.StringRef("HOLA")
	m["e3"] = common.StringRef("e1,e2")
	m["e4"] = boolRef(true)

	if e, ok := m.getInt64("e1"); !ok || e != 123 {
		t.Error("error parsing e1. Got: ", ok, e)
	}

	if e, ok := m.getString("e2"); !ok || e != "HOLA" {
		t.Error("error parsing e2. Got: ", ok, e)
	}

	if e, ok := m.getStringSlice("e3"); !ok || len(e) != 2 || e[0] != "e1" || e[1] != "e2" {
		t.Error("error parsing e3. Got: ", ok, e)
	}

	if e, ok := m.getBool("e4"); !ok || !e {
		t.Error("error parsing e4. Got: ", ok, e)
	}
}

func TestParsingDefaultValues(t *testing.T) {
	target := someConf{}
	PopulateDefaults(&target)
	if e := target.F1; e != 123 {
		t.Error("expected F1 == 123. Got: ", e)
	}

	if e := target.F2; e != "HOLA" {
		t.Error("expected F2 == HOLA. Got: ", e)
	}

	if e := target.F3; e {
		t.Error("expected F3 == false. Got: ", e)
	}

	if e := target.F4; len(e) != 2 || e[0] != "e1" || e[1] != "e2" {
		t.Error("expected F4 == [e1,e2]. Got: ", e)
	}

	if e := target.F5.F1; e != "CHAU" {
		t.Error("expected F5 == CHAU. Got: ", e)
	}
}

func TestPopulateFromArgMap(t *testing.T) {
	target := &someConf{}
	argMap := make(ArgMap)
	argMap["f1"] = common.Int64Ref(456)
	argMap["f2"] = common.StringRef("HOLA2")
	argMap["f3"] = boolRef(true)
	argMap["f4"] = common.StringRef("e3,e4")
	argMap["ff1"] = common.StringRef("CHAU2")

	PopulateFromArguments(target, argMap)
	if e := target.F1; e != 456 {
		t.Error("expected F1 == 456. Got: ", e)
	}

	if e := target.F2; e != "HOLA2" {
		t.Error("expected F2 == HOLA2. Got: ", e)
	}

	if e := target.F3; !e {
		t.Error("expected F3 == true. Got: ", e)
	}

	if e := target.F4; len(e) != 2 || e[0] != "e3" || e[1] != "e4" {
		t.Error("expected F4 == [e1,e2]. Got: ", e)
	}

	if e := target.F5.F1; e != "CHAU2" {
		t.Error("expected F5 == CHAU2. Got: ", e)
	}
}

func TestBuildArgumentMapFromStruct(t *testing.T) {
	target := &someConf{}
	m := MakeCliArgMapFor(target)
	os.Args = []string{"programName", "-f1=456", "-f2=HOLA2", "-f3", "-f4=e3,e4", "-ff1=CHAU2"}
	flag.Parse()

	for _, param := range []string{"f1", "f2", "f3", "f4", "ff1"} {
		if _, ok := m[param]; !ok {
			t.Error(param, " not present in map")
		}
	}

	PopulateFromArguments(target, m)
	if e := target.F1; e != 456 {
		t.Error("expected F1 == 456. Got: ", e)
	}

	if e := target.F2; e != "HOLA2" {
		t.Error("expected F2 == HOLA2. Got: ", e)
	}

	if e := target.F3; !e {
		t.Error("expected F3 == true. Got: ", e)
	}

	if e := target.F4; len(e) != 2 || e[0] != "e3" || e[1] != "e4" {
		t.Error("expected F4 == [e1,e2]. Got: ", e)
	}

	if e := target.F5.F1; e != "CHAU2" {
		t.Error("expected F5 == CHAU2. Got: ", e)
	}

}

func boolRef(b bool) *bool {
	return &b
}
