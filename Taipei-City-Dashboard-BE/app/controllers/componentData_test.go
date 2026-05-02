package controllers

import "testing"

func TestResolveChartQueryType(t *testing.T) {
	tests := []struct {
		name             string
		queryType        string
		wantBaseType     string
		wantSelectors    bool
		wantValidMapping bool
	}{
		{
			name:             "existing query type",
			queryType:        "three_d",
			wantBaseType:     "three_d",
			wantSelectors:    false,
			wantValidMapping: true,
		},
		{
			name:             "default two selector type",
			queryType:        "two_selector",
			wantBaseType:     "three_d",
			wantSelectors:    true,
			wantValidMapping: true,
		},
		{
			name:             "explicit two selector two dimensional type",
			queryType:        "two_selector_two_d",
			wantBaseType:     "two_d",
			wantSelectors:    true,
			wantValidMapping: true,
		},
		{
			name:             "unknown type",
			queryType:        "not_real",
			wantBaseType:     "",
			wantSelectors:    false,
			wantValidMapping: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBaseType, gotSelectors, gotValidMapping := resolveChartQueryType(tt.queryType)
			if gotBaseType != tt.wantBaseType {
				t.Fatalf("expected base type %q, got %q", tt.wantBaseType, gotBaseType)
			}
			if gotSelectors != tt.wantSelectors {
				t.Fatalf("expected require selectors %v, got %v", tt.wantSelectors, gotSelectors)
			}
			if gotValidMapping != tt.wantValidMapping {
				t.Fatalf("expected valid mapping %v, got %v", tt.wantValidMapping, gotValidMapping)
			}
		})
	}
}
