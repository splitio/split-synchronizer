package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	producer "github.com/splitio/split-synchronizer/v5/splitio/producer/conf"
	proxy "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
)

func main() {

	target := flag.String("target", "", "synchronizer|proxy")
	envPrefix := flag.String("env-prefix", "", "SPLIT_SYNC_ | SPLIT_PROXY_ | ...")
	output := flag.String("output", "{cli}\n", "string containing one or more of `cli,env,json,desc,type,default` in braces")
	flag.Parse()

	var config interface{}
	switch *target {
	case "synchronizer":
		config = producer.Main{}
	case "proxy":
		config = proxy.Main{}
	case "":
		fmt.Println("-target is required.")
		os.Exit(1)
	default:
		fmt.Println("invalid target config: ", *target)
		os.Exit(1)
	}

	parsedOutput := parseSpecialChars(*output)

	var collector OptionCollector
	VisitConfig(config, collector.Collect)

	for _, collected := range collector.collected {
		replacer := strings.NewReplacer(
			"{cli}", collected.CliArg,
			"{env}", *envPrefix + collected.Env,
			"{json}", collected.JSON,
			"{desc}", collected.Description,
			"{type}", collected.Type,
			"{default}", collected.Default,
		)
		fmt.Print(replacer.Replace(parsedOutput))
	}
}

type ConfigOption struct {
	CliArg      string
	JSON        string
	Env         string
	Description string
	Type        string
	Default     string
}

type OptionCollector struct {
	collected []ConfigOption
}

func (o *OptionCollector) Collect(stack Stack, current reflect.StructField, value interface{}) bool {

	var cliOpt string
	stack.Each(func(f reflect.StructField) bool {
		if prefix, ok := f.Tag.Lookup("s-cli-prefix"); ok {
			cliOpt += prefix + "-"
		}
		return true
	})
	cliOpt += current.Tag.Get("s-cli")

	jsonOpt := current.Name
	if j, ok := current.Tag.Lookup("json"); ok {
		jsonOpt = strings.Split(j, ",")[0] // remove `omitempty` and other stuff
	}

	o.collected = append(o.collected, ConfigOption{
		CliArg:      cliOpt,
		JSON:        jsonOpt,
		Env:         cliToEnv(cliOpt),
		Description: current.Tag.Get("s-desc"),
		Type:        current.Type.Name(),
		Default:     current.Tag.Get("s-def"),
	})
	return true
}

func cliToEnv(cli string) string {
	return strings.ToUpper(strings.ReplaceAll(cli, "-", "_"))
}

func parseSpecialChars(s string) string {
	return strings.NewReplacer("\\n", "\n", "\\t", "\t").Replace(s)
}

type ConfigVisitor func(stack Stack, current reflect.StructField, value interface{}) (keepGoing bool)

type Stack []reflect.StructField

func (s Stack) Each(callback func(f reflect.StructField) bool) {
	for idx := 0; idx < len(s) && callback(s[idx]); idx++ {
	}
}

func VisitConfig(sp interface{}, visitor ConfigVisitor) {
	visitConfig(reflect.TypeOf(sp), reflect.ValueOf(sp), nil, visitor)
}

func visitConfig(rtype reflect.Type, val reflect.Value, stack Stack, visitor ConfigVisitor) {
	for _, field := range reflect.VisibleFields(rtype) {
		if field.Type.Kind() == reflect.Struct {
			stack = append(stack, field)
			visitConfig(field.Type, val.FieldByName(field.Name), stack, visitor)
			stack = stack[:len(stack)-1]
		} else {
			if !visitor(stack, field, val.Interface()) {
				return
			}
		}
	}
}
