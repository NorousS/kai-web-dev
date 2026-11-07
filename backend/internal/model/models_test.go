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
func TestEffectivePriority(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		base string
		until time.Duration
		override string
		status string
		want string
	}{
		{"far away low", "low", 14 * 24 * time.Hour, "auto", "todo", "low"},
		{"one week", "low", 5 * 24 * time.Hour, "auto", "in_progress", "medium"},
		{"three days", "low", 2 * 24 * time.Hour, "auto", "todo", "high"},
		{"one day", "low", 12 * time.Hour, "auto", "in_review", "critical"},
		{"overdue", "low", -time.Hour, "auto", "todo", "critical"},
		{"higher manual priority kept", "high", 5 * 24 * time.Hour, "auto", "todo", "high"},
		{"manual critical", "low", 30 * 24 * time.Hour, "critical", "todo", "critical"},
		{"manual green", "low", 12 * time.Hour, "green", "todo", "low"},
		{"done returns original", "low", -time.Hour, "auto", "done", "low"},
		{"done ignores override", "low", -time.Hour, "critical", "done", "low"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			due := now.Add(tc.until)
			task := Task{Priority: tc.base, UrgencyOverride: tc.override, DueAt: &due, Status: tc.status}
			if got := EffectivePriority(task, now); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
			if task.Priority != tc.base {
				t.Fatal("calculation must not modify persisted priority")
			}
		})
	}
}
