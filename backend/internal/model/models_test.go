package model

import (
	"testing"
	"time"
)

func TestUrgency(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		before time.Duration
		want   string
	}{{14 * 24 * time.Hour, "green"}, {5 * 24 * time.Hour, "yellow"}, {2 * 24 * time.Hour, "red"}, {12 * time.Hour, "critical"}, {-2 * time.Hour, "critical"}} {
		due := now.Add(tc.before)
		if got := Urgency(Task{DueAt: &due}, now); got != tc.want {
			t.Errorf("%v: got %s, expected %s", tc.before, got, tc.want)
		}
	}
	due := now.Add(time.Hour)
	if got := Urgency(Task{DueAt: &due, UrgencyOverride: "green"}, now); got != "green" {
		t.Fatalf("manual priority ignored: %s", got)
	}
	if got := Urgency(Task{DueAt: &due, Status: "done"}, now); got != "green" {
		t.Fatalf("completed task flashes: %s", got)
	}
}