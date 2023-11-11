package flagsets

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

func (f *FlagSetMatcher) Sanitize(input []string) []string {
	if !f.strict || len(input) == 0 {
		return input
	}

	for idx := range input {
		if _, ok := f.sets[input[idx]]; !ok {
			if idx+1 < len(input) {
				input[idx] = input[len(input)-1]
			}
			input = input[:len(input)-1]
		}
	}
	return input
}
