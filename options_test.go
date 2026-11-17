package goheos

import (
	"testing"
	"time"
)

func TestWithHeartbeat(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantOn   bool
	}{
		{name: "positive interval", interval: time.Second, wantOn: true},
		{name: "zero interval", interval: 0, wantOn: false},
		{name: "negative interval", interval: -time.Second, wantOn: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := defaultOptions()
			WithHeartbeat(tt.interval)(&o)

			if o.heartBeat != tt.wantOn {
				t.Errorf("heartbeat enabled = %v, want %v", o.heartBeat, tt.wantOn)
			}
			if o.heartBeatInterval != tt.interval {
				t.Errorf("heartbeat interval = %s, want %s", o.heartBeatInterval, tt.interval)
			}
		})
	}
}
