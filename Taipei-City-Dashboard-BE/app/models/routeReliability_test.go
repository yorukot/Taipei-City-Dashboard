package models

import (
	"testing"
	"time"
)

func TestClassifyBusArrivalError(t *testing.T) {
	tests := []struct {
		minutes float64
		want    string
	}{
		{minutes: 5, want: RouteReliabilityGreen},
		{minutes: 5.1, want: RouteReliabilityYellow},
		{minutes: 10, want: RouteReliabilityYellow},
		{minutes: 10.1, want: RouteReliabilityRed},
	}

	for _, tt := range tests {
		if got := classifyBusArrivalError(tt.minutes); got != tt.want {
			t.Fatalf("expected %s for %.1f minutes, got %s", tt.want, tt.minutes, got)
		}
	}
}

func TestClassifyRailDelay(t *testing.T) {
	tests := []struct {
		severe float64
		avg    float64
		want   string
	}{
		{severe: 9.9, avg: 4.9, want: RouteReliabilityGreen},
		{severe: 10, avg: 4.9, want: RouteReliabilityYellow},
		{severe: 9.9, avg: 5, want: RouteReliabilityYellow},
		{severe: 25, avg: 4.9, want: RouteReliabilityRed},
		{severe: 9.9, avg: 10, want: RouteReliabilityRed},
	}

	for _, tt := range tests {
		if got := classifyRailDelay(tt.severe, tt.avg); got != tt.want {
			t.Fatalf("expected %s for severe %.1f avg %.1f, got %s", tt.want, tt.severe, tt.avg, got)
		}
	}
}

func TestClassifyYouBikeSuccessRate(t *testing.T) {
	tests := []struct {
		rate float64
		want string
	}{
		{rate: 70, want: RouteReliabilityGreen},
		{rate: 69.9, want: RouteReliabilityYellow},
		{rate: 40, want: RouteReliabilityYellow},
		{rate: 39.9, want: RouteReliabilityRed},
	}

	for _, tt := range tests {
		if got := classifyYouBikeSuccessRate(tt.rate); got != tt.want {
			t.Fatalf("expected %s for %.1f%%, got %s", tt.want, tt.rate, got)
		}
	}
}

func TestAggregateRouteReliabilityStatus(t *testing.T) {
	tests := []struct {
		name string
		legs []RouteLegReliability
		want string
	}{
		{
			name: "red wins",
			legs: []RouteLegReliability{
				{Status: RouteReliabilityGreen},
				{Status: RouteReliabilityRed},
			},
			want: RouteReliabilityRed,
		},
		{
			name: "yellow before green",
			legs: []RouteLegReliability{
				{Status: RouteReliabilityGreen},
				{Status: RouteReliabilityYellow},
			},
			want: RouteReliabilityYellow,
		},
		{
			name: "only green",
			legs: []RouteLegReliability{
				{Status: RouteReliabilityGreen},
			},
			want: RouteReliabilityGreen,
		},
		{
			name: "only unknown",
			legs: []RouteLegReliability{
				{Status: RouteReliabilityUnknown},
			},
			want: RouteReliabilityUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregateRouteReliabilityStatus(tt.legs); got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestParseReliabilityTime(t *testing.T) {
	parsed, err := parseReliabilityTime("2026-05-03T08:30:00+08:00")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := taipeiHourBucket(parsed); got != "08:00" {
		t.Fatalf("expected hour bucket 08:00, got %s", got)
	}
	if got := taipeiWeekdayName(parsed); got != "星期日" {
		t.Fatalf("expected 星期日, got %s", got)
	}

	if _, err := parseReliabilityTime("bad-time"); err == nil {
		t.Fatal("expected invalid time error")
	}
}

func TestAnalyzeRouteLegReliabilityTreatsSubwayAsGreen(t *testing.T) {
	got := analyzeRouteLegReliability(
		RouteReliabilityLegRequest{ID: "mrt-1", Mode: "SUBWAY"},
		timeForReliabilityTest(t),
	)
	if got.Status != RouteReliabilityGreen {
		t.Fatalf("expected subway to be green, got %s", got.Status)
	}
	if got.MatchQuality != "default" {
		t.Fatalf("expected default match quality, got %s", got.MatchQuality)
	}
}

func timeForReliabilityTest(t *testing.T) time.Time {
	t.Helper()
	parsed, err := parseReliabilityTime("2026-05-03T08:30:00+08:00")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	return parsed
}
