package flagsets

import "golang.org/x/exp/slices"

type FlagSetMatcher struct {
	strict bool
	sets   map[string]struct{}
}

func NewMatcher(strict bool, fetched []string) FlagSetMatcher {
	out := FlagSetMatcher{
		strict: strict,
		sets:   make(map[string]struct{}, len(fetched)),
	}

	for idx := range fetched {
		out.sets[fetched[idx]] = struct{}{}
	}

	return out
}

// Sort, Dedupe & Filter input flagsets. Returns a new sanitized slice; the input slice is never modified.
func (f *FlagSetMatcher) Sanitize(input []string) []string {
	if len(input) == 0 {
		return input
	}

	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		if f.strict && !setContains(f.sets, item) {
			continue
		}
		if setContains(seen, item) {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}

	slices.Sort(out)
	return out
}

func setContains(set map[string]struct{}, item string) bool {
	_, ok := set[item]
	return ok
}
