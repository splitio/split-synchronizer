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

// Sort, Dedupe & Filter input flagsets. returns sanitized list and a boolean indicating whether a sort was necessary
func (f *FlagSetMatcher) Sanitize(input []string) []string {
	if len(input) == 0 {
		return input
	}

	seen := map[string]struct{}{}
	for idx := 0; idx < len(input); idx++ { // cant use range because we're srhinking the slice inside the loop
		item := input[idx]
		if (f.strict && !setContains(f.sets, item)) || setContains(seen, item) {
			if idx+1 < len(input) {
				input[idx] = input[len(input)-1]
			}
			input = input[:len(input)-1]
		}
		seen[item] = struct{}{}
	}

	slices.Sort(input)
	return input
}

func setContains(set map[string]struct{}, item string) bool {
	_, ok := set[item]
	return ok
}
