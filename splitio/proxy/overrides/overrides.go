// Package overrides loads the treatment overrides file of the Split Proxy and applies it to flags.
package overrides

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"gopkg.in/yaml.v3"
)

const (
	fieldTreatment = "treatment"
	fieldConfig    = "config"
	fieldKeys      = "keys"
)

// Entry is the override of a single flag.
type Entry struct {
	Flag      string
	Treatment string
	// Config replaces the config of Treatment. Nil keeps the flag's existing config.
	Config *string
}

// Overrides is the content of a loaded overrides file.
type Overrides struct {
	Path    string
	SHA256  string
	Entries map[string]Entry

	order []string
}

// Flags returns the overridden flag names in file order.
func (o *Overrides) Flags() []string {
	return append([]string(nil), o.order...)
}

// Load reads and parses an overrides file. An empty file, or one with only comments or `{}`, has no entries.
func Load(path string) (*Overrides, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("overrides file %s: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	o := &Overrides{Path: path, SHA256: hex.EncodeToString(sum[:]), Entries: map[string]Entry{}}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("overrides file %s: malformed YAML: %w", path, err)
	}
	if doc.Kind == 0 { // no content at all
		return o, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("overrides file %s: line %d: the root must be a map from flag key to override", path, root.Line)
	}

	lines := map[string]int{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode, valNode := root.Content[i], root.Content[i+1]
		flag := keyNode.Value
		if first, dup := lines[flag]; dup {
			return nil, fmt.Errorf("overrides file %s: flag %q: duplicate key (lines %d and %d)", path, flag, first, keyNode.Line)
		}
		lines[flag] = keyNode.Line

		entry, err := parseEntry(path, flag, valNode)
		if err != nil {
			return nil, err
		}
		o.Entries[flag] = entry
		o.order = append(o.order, flag)
	}
	return o, nil
}

func entryErr(path, flag, field, format string, args ...any) error {
	if field == "" {
		return fmt.Errorf("overrides file %s: flag %q: %s", path, flag, fmt.Sprintf(format, args...))
	}
	return fmt.Errorf("overrides file %s: flag %q: field %q: %s", path, flag, field, fmt.Sprintf(format, args...))
}

func parseEntry(path, flag string, node *yaml.Node) (Entry, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		t, err := treatmentFrom(path, flag, node)
		if err != nil {
			return Entry{}, err
		}
		return Entry{Flag: flag, Treatment: t}, nil
	case yaml.MappingNode:
		return parseLongForm(path, flag, node)
	default:
		return Entry{}, entryErr(path, flag, "", "line %d: expected a treatment string or a map with treatment and config", node.Line)
	}
}

func parseLongForm(path, flag string, node *yaml.Node) (Entry, error) {
	entry := Entry{Flag: flag}
	var haveTreatment bool
	for i := 0; i+1 < len(node.Content); i += 2 {
		field, val := node.Content[i].Value, node.Content[i+1]
		switch field {
		case fieldTreatment:
			t, err := treatmentFrom(path, flag, val)
			if err != nil {
				return Entry{}, err
			}
			entry.Treatment, haveTreatment = t, true
		case fieldConfig:
			if val.Kind != yaml.ScalarNode || val.ShortTag() != "!!str" {
				return Entry{}, entryErr(path, flag, fieldConfig, "line %d: config must be a string containing JSON", val.Line)
			}
			if !json.Valid([]byte(val.Value)) {
				return Entry{}, entryErr(path, flag, fieldConfig, "line %d: config is not valid JSON", val.Line)
			}
			cfg := val.Value
			entry.Config = &cfg
		case fieldKeys:
			return Entry{}, entryErr(path, flag, fieldKeys, "line %d: per-key targeting is not supported", node.Content[i].Line)
		default:
			return Entry{}, entryErr(path, flag, field, "line %d: unknown field, only %q and %q are allowed", node.Content[i].Line, fieldTreatment, fieldConfig)
		}
	}
	if !haveTreatment {
		return Entry{}, entryErr(path, flag, fieldTreatment, "line %d: treatment is required", node.Line)
	}
	return entry, nil
}

func treatmentFrom(path, flag string, node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!str" {
		return "", entryErr(path, flag, fieldTreatment, "line %d: treatment must be a string; quote it (e.g. \"%s\"), because values like on/off/true/false can be read as booleans", node.Line, node.Value)
	}
	return node.Value, nil
}

// Validate checks the entries against the flags the proxy serves. A treatment that the flag does not have is an
// error. An entry for a flag that is not in flags is dropped with a warning.
func Validate(o *Overrides, flags []dtos.SplitDTO, logger logging.LoggerInterface) error {
	treatments := make(map[string]map[string]struct{}, len(flags))
	for i := range flags {
		set := map[string]struct{}{flags[i].DefaultTreatment: {}}
		for _, cond := range flags[i].Conditions {
			for _, part := range cond.Partitions {
				set[part.Treatment] = struct{}{}
			}
		}
		treatments[flags[i].Name] = set
	}

	kept := o.order[:0:0]
	for _, name := range o.order {
		set, present := treatments[name]
		if !present {
			logger.Warning(fmt.Sprintf("Override for '%s' skipped: flag not present in proxy data", name))
			delete(o.Entries, name)
			continue
		}
		if _, ok := set[o.Entries[name].Treatment]; !ok {
			return entryErr(o.Path, name, fieldTreatment, "%q is not one of the flag's treatments", o.Entries[name].Treatment)
		}
		kept = append(kept, name)
	}
	o.order = kept
	return nil
}
