package api

import (
	"testing"
	"time"
)

func TestCapDue(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.Local)
	day := 24 * time.Hour

	tests := []struct {
		name    string
		fsrsDue time.Time
		capTime time.Time
		wantDue time.Time
		capped  bool
	}{
		{
			name:    "due before cap stays",
			fsrsDue: now.Add(2 * day),
			capTime: now.Add(10 * day),
			wantDue: now.Add(2 * day),
			capped:  false,
		},
		{
			name:    "overshooting due squeezed to 90% of remaining",
			fsrsDue: now.Add(30 * day),
			capTime: now.Add(10 * day),
			wantDue: now.Add(9 * day), // 0.9 * 10d
			capped:  true,
		},
		{
			name:    "final day hard-caps at capTime",
			fsrsDue: now.Add(5 * day),
			capTime: now.Add(12 * time.Hour), // 0.9*12h < 24h → hard cap
			wantDue: now.Add(12 * time.Hour),
			capped:  true,
		},
		{
			name:    "on deadline day nothing is capped",
			fsrsDue: now.Add(30 * day),
			capTime: now, // now >= capTime
			wantDue: now.Add(30 * day),
			capped:  false,
		},
		{
			name:    "past deadline nothing is capped",
			fsrsDue: now.Add(30 * day),
			capTime: now.Add(-2 * day),
			wantDue: now.Add(30 * day),
			capped:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, capped := capDue(tt.fsrsDue, now, tt.capTime)
			if capped != tt.capped {
				t.Fatalf("capped = %v, want %v", capped, tt.capped)
			}
			if !got.Equal(tt.wantDue) {
				t.Fatalf("due = %v, want %v", got, tt.wantDue)
			}
		})
	}
}

func TestDaysLeft(t *testing.T) {
	now := time.Date(2026, 7, 10, 15, 30, 0, 0, time.Local)

	tests := []struct {
		deadline string
		want     int
	}{
		{"2026-07-10", 1}, // deadline today: today is the last study day
		{"2026-07-11", 2}, // today + deadline day
		{"2026-07-19", 10},
		{"2026-07-09", 0},  // passed yesterday
		{"2026-07-01", -8}, // long passed
	}
	for _, tt := range tests {
		got, err := daysLeft(tt.deadline, now)
		if err != nil {
			t.Fatalf("daysLeft(%q): %v", tt.deadline, err)
		}
		if got != tt.want {
			t.Errorf("daysLeft(%q) = %d, want %d", tt.deadline, got, tt.want)
		}
	}

	if _, err := daysLeft("not-a-date", now); err == nil {
		t.Error("want error for malformed deadline")
	}
}

func TestTargetNewToday(t *testing.T) {
	tests := []struct {
		name                                       string
		remaining, introduced, daysLeft, newPerDay int
		want                                       int
	}{
		{"even split", 30, 0, 3, 10, 10},
		{"ceil rounds up", 31, 0, 3, 10, 11},
		{"deadline today takes all", 25, 0, 1, 10, 25},
		{"stable across refetches", 20, 5, 5, 3, 5}, // (20+5)/5, not 20/5+5
		{"never below configured limit", 2, 0, 5, 10, 10},
		{"passed deadline reverts to limit", 40, 0, 0, 10, 10},
		{"nothing new left", 0, 4, 3, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := targetNewToday(tt.remaining, tt.introduced, tt.daysLeft, tt.newPerDay)
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}
