package dashboard

import "testing"

func TestFormatNumber(t *testing.T) {

	var hundred = int64(999)
	var thousand = int64(999546)
	var million = int64(999456832)
	var billion = int64(999678123432)
	var trillion = int64(999321543123987)
	var quadrillion = int64(999453678765987234)

	var result string

	result = FormatNumber(hundred)
	if result != "999" {
		t.Error("Hundred mal-formed! - expected: 999 ... result: " + result)
	}

	result = FormatNumber(thousand)
	if result != "999.55 k" {
		t.Error("Thousand mal-formed! - expected: 999.55 k ... result: " + result)
	}

	result = FormatNumber(million)
	if result != "999.46 M" {
		t.Error("Million mal-formed! - expected: 999.46 M ... result: " + result)
	}

	result = FormatNumber(billion)
	if result != "999.68 G" {
		t.Error("Billion mal-formed! - expected: 999.68 G ... result: " + result)
	}

	result = FormatNumber(trillion)
	if result != "999.32 T" {
		t.Error("Trillion mal-formed! - expected: 999.32 T ... result: " + result)
	}

	result = FormatNumber(quadrillion)
	if result != "999.45 P" {
		t.Error("Quadrillion mal-formed! - expected: 999.45 P ... result: " + result)
	}

}
