package models

import (
	"database/sql"
	"errors"
	"testing"
)

func TestBuildChartQueryKeepsLegacyTimePlaceholders(t *testing.T) {
	query, args, err := buildChartQuery(
		"SELECT * FROM table WHERE data_time BETWEEN '%s' AND '%s'",
		ChartQueryParams{TimeFrom: "2026-01-01T00:00:00+08:00", TimeTo: "2026-01-02T00:00:00+08:00"},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if query != "SELECT * FROM table WHERE data_time BETWEEN '2026-01-01T00:00:00+08:00' AND '2026-01-02T00:00:00+08:00'" {
		t.Fatalf("unexpected query: %s", query)
	}
	if len(args) != 0 {
		t.Fatalf("expected no named args for legacy query, got %d", len(args))
	}
}

func TestBuildChartQueryNormalizesNamedParams(t *testing.T) {
	query, args, err := buildChartQuery(
		"SELECT * FROM table WHERE data_time >= :timefrom::timestamptz AND route = :selector_1 AND segment = @selector_2",
		ChartQueryParams{
			TimeFrom:  "2026-01-01T00:00:00+08:00",
			TimeTo:    "2026-01-02T00:00:00+08:00",
			Selector1: "route-a",
			Selector2: "segment-b",
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if query != "SELECT * FROM table WHERE data_time >= (@timefrom)::timestamptz AND route = @selector_1 AND segment = @selector_2" {
		t.Fatalf("unexpected query: %s", query)
	}

	expected := map[string]any{
		"timefrom":   "2026-01-01T00:00:00+08:00",
		"selector_1": "route-a",
		"selector_2": "segment-b",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d named args, got %d", len(expected), len(args))
	}
	for _, arg := range args {
		namedArg, ok := arg.(sql.NamedArg)
		if !ok {
			t.Fatalf("expected sql.NamedArg, got %T", arg)
		}
		if expected[namedArg.Name] != namedArg.Value {
			t.Fatalf("unexpected value for %s: %v", namedArg.Name, namedArg.Value)
		}
		delete(expected, namedArg.Name)
	}
	if len(expected) != 0 {
		t.Fatalf("missing named args: %v", expected)
	}
}

func TestBuildChartQueryRequiresUsedSelector(t *testing.T) {
	_, _, err := buildChartQuery(
		"SELECT * FROM table WHERE route = :selector_1",
		ChartQueryParams{},
	)
	if !errors.Is(err, ErrMissingChartSelector) {
		t.Fatalf("expected ErrMissingChartSelector, got %v", err)
	}
}

func TestBuildChartQueryRequiresSelectorsWhenConfigured(t *testing.T) {
	_, _, err := buildChartQuery(
		"SELECT * FROM table",
		ChartQueryParams{RequireSelectors: true},
	)
	if !errors.Is(err, ErrMissingChartSelector) {
		t.Fatalf("expected ErrMissingChartSelector, got %v", err)
	}

	_, _, err = buildChartQuery(
		"SELECT * FROM table",
		ChartQueryParams{RequireSelectors: true, Selector1: "route-a", Selector2: "segment-b"},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestBuildChartQueryDoesNotMatchPlaceholderPrefixes(t *testing.T) {
	query, args, err := buildChartQuery(
		"SELECT :selector_10 AS unsupported_selector",
		ChartQueryParams{},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if query != "SELECT :selector_10 AS unsupported_selector" {
		t.Fatalf("unexpected query: %s", query)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %d", len(args))
	}
}
