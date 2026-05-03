package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildTrainStationReliabilityMetrics(t *testing.T) {
	metrics := buildTrainStationReliabilityMetrics(trainStationReliabilityAggregate{
		OnTimeRate:      84.24,
		DelayRate:       12.26,
		SevereDelayRate: 3.5,
		Count:           37,
	})

	assertMetric(t, metrics, "on_time_rate", "準點率", 84.2, "%")
	assertMetric(t, metrics, "delay_rate", "誤點率", 12.3, "%")
	assertMetric(t, metrics, "severe_delay_rate", "嚴重誤點率", 3.5, "%")
	assertMetric(t, metrics, "sample_count", "樣本數", 37, "筆")
}

func TestBuildYouBikeAvailabilityMetrics(t *testing.T) {
	rentMetrics := buildYouBikeAvailabilityMetrics("rent", 5.24, 19)
	assertMetric(t, rentMetrics, "avg_available_rent_bikes", "平均可借車輛", 5.2, "輛")
	assertMetric(t, rentMetrics, "rent_sample_count", "可借樣本數", 19, "筆")

	returnMetrics := buildYouBikeAvailabilityMetrics("return", 8.26, 23)
	assertMetric(t, returnMetrics, "avg_available_return_bikes", "平均可還空位", 8.3, "格")
	assertMetric(t, returnMetrics, "return_sample_count", "可還樣本數", 23, "筆")
}

func TestBuildBusArrivalReliabilityMetrics(t *testing.T) {
	metrics := buildBusArrivalReliabilityMetrics(busArrivalReliabilityAggregate{
		AvgAbsArrivalErrorMinutes: 6.26,
		OnTimeCount:               15,
		LateCount:                 8,
		EarlyOver5Count:           2,
		Late5To10Count:            3,
		LateOver10Count:           5,
		SampleCount:               23,
	})

	assertMetric(t, metrics, "avg_abs_arrival_error_minutes", "平均到站誤差", 6.3, "分鐘")
	assertMetric(t, metrics, "on_time_count", "準時班次", 15, "筆")
	assertMetric(t, metrics, "late_count", "未準時班次", 8, "筆")
	assertMetric(t, metrics, "early_over_5_count", "提前 5 分以上", 2, "筆")
	assertMetric(t, metrics, "late_5_to_10_count", "誤點 5-10 分", 3, "筆")
	assertMetric(t, metrics, "late_over_10_count", "誤點 10 分以上", 5, "筆")
	assertMetric(t, metrics, "sample_count", "樣本數", 23, "筆")
}

func TestAnalyzeRouteLegReliabilityUnsupportedMode(t *testing.T) {
	got := analyzeRouteLegReliability(
		RouteReliabilityLegRequest{ID: "mrt-1", Mode: "SUBWAY"},
		timeForReliabilityTest(t),
	)
	if got.Available {
		t.Fatal("expected unsupported subway mode to be unavailable")
	}
	if got.Mode != "SUBWAY" {
		t.Fatalf("expected normalized mode SUBWAY, got %s", got.Mode)
	}
	if got.MatchQuality != "unavailable" {
		t.Fatalf("expected unavailable match quality, got %s", got.MatchQuality)
	}
	if len(got.Metrics) != 0 {
		t.Fatalf("expected no metrics for unsupported mode, got %d", len(got.Metrics))
	}
}

func TestRouteReliabilityJSONOmitsLegacyStatusFields(t *testing.T) {
	payload, err := json.Marshal(RouteReliabilityOutput{
		Legs: []RouteLegReliability{
			{
				ID:        "rail-1",
				Mode:      "RAIL",
				Available: true,
				Metrics:   []RouteReliabilityMetric{},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected marshal to succeed, got %v", err)
	}

	body := string(payload)
	for _, legacyField := range []string{"route_status", "status", "label"} {
		if strings.Contains(body, legacyField) {
			t.Fatalf("expected response JSON to omit %q, got %s", legacyField, body)
		}
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

	if _, err := parseReliabilityTime("bad-time"); err == nil {
		t.Fatal("expected invalid time error")
	}
}

func TestTrainStationNameCandidates(t *testing.T) {
	candidates := trainStationNameCandidates("台北車站")
	if !containsString(candidates, "臺北") {
		t.Fatalf("expected candidates to include 臺北, got %#v", candidates)
	}
}

func assertMetric(t *testing.T, metrics []RouteReliabilityMetric, key string, label string, value float64, unit string) {
	t.Helper()
	for _, metric := range metrics {
		if metric.Key != key {
			continue
		}
		if metric.Label != label || metric.Value != value || metric.Unit != unit {
			t.Fatalf("unexpected metric for %s: %#v", key, metric)
		}
		return
	}
	t.Fatalf("metric %s not found in %#v", key, metrics)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func timeForReliabilityTest(t *testing.T) time.Time {
	t.Helper()
	parsed, err := parseReliabilityTime("2026-05-03T08:30:00+08:00")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	return parsed
}
