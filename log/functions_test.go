package log

import "testing"

func TestObfuscateAPIKey(t *testing.T) {

	var testSet = map[string]string{
		"hd4mgav7kam3ebh94t615pcm9ths1r3n64a3": "hd4m...64a3",
		"hd4m9e": "h...e",
		"":       "..."}

	for key, expected := range testSet {
		gotKey := ObfuscateAPIKey(key)
		if gotKey != expected {
			t.Error("Expected:", expected, "Returned:", gotKey, "Key:", key)
			return
		}
	}

}
