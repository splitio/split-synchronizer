package proxy

import "testing"

func TestTelemetryPeriodSeconds(t *testing.T) {
	tests := []struct {
		name   string
		rateMs int64
		want   int
	}{
		{name: "default one hour", rateMs: 3_600_000, want: 3600},
		{name: "one minute", rateMs: 60_000, want: 60},
		{name: "exactly one second", rateMs: 1000, want: 1},
		{name: "just under one second clamps to one", rateMs: 999, want: 1},
		{name: "one millisecond clamps to one", rateMs: 1, want: 1},
		{name: "zero clamps to one", rateMs: 0, want: 1},
		{name: "negative clamps to one", rateMs: -5000, want: 1},
		{name: "integer division truncates", rateMs: 3_599_999, want: 3599},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := telemetryPeriodSeconds(tc.rateMs); got != tc.want {
				t.Errorf("telemetryPeriodSeconds(%d) = %d, want %d", tc.rateMs, got, tc.want)
			}
		})
	}
}
