package models

import (
	"fmt"
	"strings"
	"time"
)

var taipeiRouteReliabilityLocation = time.FixedZone("Asia/Taipei", 8*60*60)

type RouteReliabilityRequest struct {
	DepartureTime string                       `json:"departure_time"`
	Legs          []RouteReliabilityLegRequest `json:"legs"`
}

type RouteReliabilityLegRequest struct {
	ID                string  `json:"id"`
	Mode              string  `json:"mode"`
	RouteName         string  `json:"route_name"`
	SelectorKey       string  `json:"selector_key"`
	FromName          string  `json:"from_name"`
	ToName            string  `json:"to_name"`
	StopName          string  `json:"stop_name"`
	Direction         *int    `json:"direction"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	DurationSeconds   float64 `json:"duration_seconds"`
	StationID         string  `json:"station_id"`
	StationUID        string  `json:"station_uid"`
	PickupStationUID  string  `json:"pickup_station_uid"`
	ReturnStationUID  string  `json:"return_station_uid"`
	TrainNo           string  `json:"train_no"`
	TrainTypeCode     string  `json:"train_type_code"`
	PickupStationName string  `json:"pickup_station_name"`
	ReturnStationName string  `json:"return_station_name"`
}

type RouteReliabilityOutput struct {
	Legs []RouteLegReliability `json:"legs"`
}

type RouteLegReliability struct {
	ID           string                   `json:"id"`
	Mode         string                   `json:"mode"`
	Available    bool                     `json:"available"`
	Reason       string                   `json:"reason"`
	Metrics      []RouteReliabilityMetric `json:"metrics"`
	Source       string                   `json:"source"`
	MatchQuality string                   `json:"match_quality"`
}

type RouteReliabilityMetric struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type trainStationReliabilityAggregate struct {
	OnTimeRate      float64 `gorm:"column:on_time_rate"`
	DelayRate       float64 `gorm:"column:delay_rate"`
	SevereDelayRate float64 `gorm:"column:severe_delay_rate"`
	Count           int64   `gorm:"column:count"`
}

type availabilityAggregate struct {
	Value float64 `gorm:"column:value"`
	Count int64   `gorm:"column:count"`
}

type busArrivalReliabilityAggregate struct {
	AvgAbsArrivalErrorMinutes float64 `gorm:"column:avg_abs_arrival_error_minutes"`
	OnTimeCount               int64   `gorm:"column:on_time_count"`
	LateCount                 int64   `gorm:"column:late_count"`
	EarlyOver5Count           int64   `gorm:"column:early_over_5_count"`
	Late5To10Count            int64   `gorm:"column:late_5_to_10_count"`
	LateOver10Count           int64   `gorm:"column:late_over_10_count"`
	SampleCount               int64   `gorm:"column:sample_count"`
}

func GetRouteReliability(req RouteReliabilityRequest) (RouteReliabilityOutput, error) {
	departureTime, err := parseReliabilityTime(req.DepartureTime)
	if err != nil {
		return RouteReliabilityOutput{}, err
	}

	legs := make([]RouteLegReliability, 0, len(req.Legs))
	for _, leg := range req.Legs {
		legTime := departureTime
		if leg.StartTime != "" {
			if parsed, err := parseReliabilityTime(leg.StartTime); err == nil {
				legTime = parsed
			}
		}

		analysis := analyzeRouteLegReliability(leg, legTime)
		legs = append(legs, analysis)
	}

	return RouteReliabilityOutput{Legs: legs}, nil
}

func analyzeRouteLegReliability(leg RouteReliabilityLegRequest, legTime time.Time) RouteLegReliability {
	mode := normalizeRouteMode(leg.Mode)
	switch mode {
	case "BUS":
		return analyzeBusReliability(leg, mode)
	case "RAIL", "TRAIN", "TRA":
		return analyzeRailReliability(leg, legTime, mode)
	case "BICYCLE_RENTAL", "BIKE_RENTAL", "YOUBIKE":
		return analyzeYouBikeReliability(leg, legTime, mode)
	default:
		return unavailableLegReliability(leg.ID, mode, "此交通模式暫無本次可靠度指標", "")
	}
}

func analyzeBusReliability(leg RouteReliabilityLegRequest, mode string) RouteLegReliability {
	if DBDashboard == nil {
		return unavailableLegReliability(leg.ID, mode, "資料庫尚未連線，無法計算公車到站誤差指標", "mv_bus_arrival_error_detail")
	}

	result, matchQuality, ok := queryBusArrivalReliability(leg)
	if !ok {
		return unavailableLegReliability(leg.ID, mode, "查無近 24 小時公車到站誤差資料", "mv_bus_arrival_error_detail")
	}

	return RouteLegReliability{
		ID:           leg.ID,
		Mode:         mode,
		Available:    true,
		Reason:       fmt.Sprintf("近 24 小時公車到站誤差資料共 %d 筆", result.SampleCount),
		Source:       "mv_bus_arrival_error_detail",
		MatchQuality: matchQuality,
		Metrics:      buildBusArrivalReliabilityMetrics(result),
	}
}

func analyzeRailReliability(leg RouteReliabilityLegRequest, legTime time.Time, mode string) RouteLegReliability {
	if DBDashboard == nil {
		return unavailableLegReliability(leg.ID, mode, "資料庫尚未連線，無法計算台鐵可靠度指標", "train_realtime")
	}

	stationID, matchQuality := resolveTrainStationID(leg)
	if stationID == "" {
		return unavailableLegReliability(leg.ID, mode, "查無台鐵站點，無法計算台鐵可靠度指標", "train_realtime")
	}

	hour := taipeiHourBucket(legTime)
	trainTypeCode := strings.TrimSpace(leg.TrainTypeCode)
	trainTypeNamePattern := trainTypeCode + "%"

	var result trainStationReliabilityAggregate
	DBDashboard.Raw(
		`SELECT
			COALESCE(round(100.0 * avg((r.delay_time < 5)::int), 1), 0)::float AS on_time_rate,
			COALESCE(round(100.0 * avg((r.delay_time >= 5 AND r.delay_time <= 10)::int), 1), 0)::float AS delay_rate,
			COALESCE(round(100.0 * avg((r.delay_time > 10)::int), 1), 0)::float AS severe_delay_rate,
			count(*) AS count
		FROM public.train_realtime r
		WHERE r.station_id = ?
			AND (? = '' OR ? = 'all' OR r.train_type_code = ? OR r.train_type_name_zh = ? OR r.train_type_name_zh ILIKE ?)
			AND to_char(r.update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
		stationID,
		trainTypeCode,
		trainTypeCode,
		trainTypeCode,
		trainTypeCode,
		trainTypeNamePattern,
		hour,
	).Scan(&result)

	if result.Count == 0 {
		return unavailableLegReliability(leg.ID, mode, "查無同小時台鐵站點歷史可靠度資料", "train_realtime")
	}

	return RouteLegReliability{
		ID:           leg.ID,
		Mode:         mode,
		Available:    true,
		Reason:       fmt.Sprintf("同小時台鐵站點歷史資料共 %d 筆", result.Count),
		Source:       "train_realtime",
		MatchQuality: matchQuality,
		Metrics:      buildTrainStationReliabilityMetrics(result),
	}
}

func analyzeYouBikeReliability(leg RouteReliabilityLegRequest, legTime time.Time, mode string) RouteLegReliability {
	if DBDashboard == nil {
		return unavailableLegReliability(leg.ID, mode, "資料庫尚未連線，無法計算 YouBike 可借可還指標", "ubike_availability")
	}

	hour := taipeiHourBucket(legTime)
	pickupUID := firstNonEmpty(leg.PickupStationUID, leg.StationUID)
	returnUID := leg.ReturnStationUID
	if pickupUID == "" {
		pickupUID = resolveYouBikeStationUID(firstNonEmpty(leg.PickupStationName, leg.FromName))
	}
	if returnUID == "" {
		returnUID = resolveYouBikeStationUID(firstNonEmpty(leg.ReturnStationName, leg.ToName))
	}

	metrics := []RouteReliabilityMetric{}
	matchQuality := "exact"
	if pickupUID == "" || returnUID == "" {
		matchQuality = "fallback"
	}

	if pickupUID != "" {
		if value, count, ok := queryYouBikeAverageAvailability(pickupUID, "rent", hour); ok {
			metrics = append(metrics, buildYouBikeAvailabilityMetrics("rent", value, count)...)
		}
	}
	if returnUID != "" {
		if value, count, ok := queryYouBikeAverageAvailability(returnUID, "return", hour); ok {
			metrics = append(metrics, buildYouBikeAvailabilityMetrics("return", value, count)...)
		}
	}

	if len(metrics) == 0 {
		return unavailableLegReliability(leg.ID, mode, "查無同小時 YouBike 可借可還歷史資料", "ubike_availability")
	}

	return RouteLegReliability{
		ID:           leg.ID,
		Mode:         mode,
		Available:    true,
		Reason:       "同小時 YouBike 歷史可借可還平均",
		Source:       "ubike_availability",
		MatchQuality: matchQuality,
		Metrics:      metrics,
	}
}

func buildBusArrivalReliabilityMetrics(result busArrivalReliabilityAggregate) []RouteReliabilityMetric {
	return []RouteReliabilityMetric{
		{Key: "avg_abs_arrival_error_minutes", Label: "平均到站誤差", Value: roundReliabilityValue(result.AvgAbsArrivalErrorMinutes), Unit: "分鐘"},
		{Key: "on_time_count", Label: "準時班次", Value: float64(result.OnTimeCount), Unit: "筆"},
		{Key: "late_count", Label: "未準時班次", Value: float64(result.LateCount), Unit: "筆"},
		{Key: "early_over_5_count", Label: "提前 5 分以上", Value: float64(result.EarlyOver5Count), Unit: "筆"},
		{Key: "late_5_to_10_count", Label: "誤點 5-10 分", Value: float64(result.Late5To10Count), Unit: "筆"},
		{Key: "late_over_10_count", Label: "誤點 10 分以上", Value: float64(result.LateOver10Count), Unit: "筆"},
		{Key: "sample_count", Label: "樣本數", Value: float64(result.SampleCount), Unit: "筆"},
	}
}

func buildTrainStationReliabilityMetrics(result trainStationReliabilityAggregate) []RouteReliabilityMetric {
	return []RouteReliabilityMetric{
		{Key: "on_time_rate", Label: "準點率", Value: roundReliabilityValue(result.OnTimeRate), Unit: "%"},
		{Key: "delay_rate", Label: "誤點率", Value: roundReliabilityValue(result.DelayRate), Unit: "%"},
		{Key: "severe_delay_rate", Label: "嚴重誤點率", Value: roundReliabilityValue(result.SevereDelayRate), Unit: "%"},
		{Key: "sample_count", Label: "樣本數", Value: float64(result.Count), Unit: "筆"},
	}
}

func buildYouBikeAvailabilityMetrics(side string, value float64, count int64) []RouteReliabilityMetric {
	if side == "return" {
		return []RouteReliabilityMetric{
			{Key: "avg_available_return_bikes", Label: "平均可還空位", Value: roundReliabilityValue(value), Unit: "格"},
			{Key: "return_sample_count", Label: "可還樣本數", Value: float64(count), Unit: "筆"},
		}
	}
	return []RouteReliabilityMetric{
		{Key: "avg_available_rent_bikes", Label: "平均可借車輛", Value: roundReliabilityValue(value), Unit: "輛"},
		{Key: "rent_sample_count", Label: "可借樣本數", Value: float64(count), Unit: "筆"},
	}
}

func queryBusArrivalReliability(leg RouteReliabilityLegRequest) (busArrivalReliabilityAggregate, string, bool) {
	selectorKey := strings.TrimSpace(leg.SelectorKey)
	if selectorKey != "" {
		if result, ok := queryBusArrivalReliabilityByFilter(selectorKey, "", "", nil); ok {
			return result, "selector", true
		}
	}

	routeCandidates := busRouteNameCandidates(leg.RouteName)
	stopCandidates := stationNameCandidates(firstNonEmpty(leg.StopName, leg.FromName))
	if len(routeCandidates) == 0 {
		return busArrivalReliabilityAggregate{}, "unavailable", false
	}

	for _, routeName := range routeCandidates {
		for _, stopName := range stopCandidates {
			if leg.Direction != nil {
				if result, ok := queryBusArrivalReliabilityByFilter("", routeName, stopName, leg.Direction); ok {
					return result, "exact", true
				}
			}
			if result, ok := queryBusArrivalReliabilityByFilter("", routeName, stopName, nil); ok {
				return result, "exact", true
			}
		}
	}

	for _, routeName := range routeCandidates {
		if leg.Direction != nil {
			if result, ok := queryBusArrivalReliabilityByFilter("", routeName, "", leg.Direction); ok {
				return result, "fallback", true
			}
		}
		if result, ok := queryBusArrivalReliabilityByFilter("", routeName, "", nil); ok {
			return result, "fallback", true
		}
	}

	return busArrivalReliabilityAggregate{}, "unavailable", false
}

func queryBusArrivalReliabilityByFilter(selectorKey string, routeName string, stopName string, direction *int) (busArrivalReliabilityAggregate, bool) {
	var result busArrivalReliabilityAggregate

	whereClauses := []string{}
	args := []interface{}{}
	if selectorKey != "" {
		whereClauses = append(whereClauses, "selector_key = ?")
		args = append(args, selectorKey)
	}
	if routeName != "" {
		whereClauses = append(whereClauses, "(route_name = ? OR route_name ILIKE ? OR ? ILIKE '%' || route_name || '%')")
		args = append(args, routeName, "%"+routeName+"%", routeName)
	}
	if stopName != "" {
		whereClauses = append(whereClauses, "(stop_name = ? OR stop_name ILIKE ? OR ? ILIKE '%' || stop_name || '%')")
		args = append(args, stopName, "%"+stopName+"%", stopName)
	}
	if direction != nil {
		whereClauses = append(whereClauses, "direction = ?")
		args = append(args, *direction)
	}
	if len(whereClauses) == 0 {
		return result, false
	}

	err := DBDashboard.Raw(
		fmt.Sprintf(
			`WITH matched AS (
			SELECT
				actual_time,
				EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0 AS signed_error_minutes
			FROM public.mv_bus_arrival_error_detail
			WHERE %s
		),
		latest_window AS (
			SELECT max(actual_time) AS max_actual_time FROM matched
		),
		base AS (
			SELECT matched.signed_error_minutes
			FROM matched, latest_window
			WHERE latest_window.max_actual_time IS NOT NULL
				AND matched.actual_time >= latest_window.max_actual_time - INTERVAL '24 hours'
		)
		SELECT
			COALESCE(round(avg(abs(signed_error_minutes))::numeric, 1), 0)::float AS avg_abs_arrival_error_minutes,
			count(*) FILTER (WHERE abs(signed_error_minutes) <= 5) AS on_time_count,
			count(*) FILTER (WHERE abs(signed_error_minutes) > 5) AS late_count,
			count(*) FILTER (WHERE signed_error_minutes < -5) AS early_over_5_count,
			count(*) FILTER (WHERE signed_error_minutes > 5 AND signed_error_minutes <= 10) AS late_5_to_10_count,
			count(*) FILTER (WHERE signed_error_minutes > 10) AS late_over_10_count,
			count(*) AS sample_count
		FROM base`,
			strings.Join(whereClauses, " AND "),
		),
		args...,
	).Scan(&result).Error
	if err != nil {
		return result, false
	}

	return result, result.SampleCount > 0
}

func queryYouBikeAverageAvailability(stationUID string, side string, hour string) (float64, int64, bool) {
	valueColumn := "available_rent_bikes"
	if side == "return" {
		valueColumn = "available_return_bikes"
	}

	var result availabilityAggregate
	DBDashboard.Raw(
		fmt.Sprintf(
			`WITH availability AS (
				SELECT station_uid, %s, update_time FROM public.ubike_availability_tpe WHERE station_uid = ?
				UNION ALL
				SELECT station_uid, %s, update_time FROM public.ubike_availability_new_tpe WHERE station_uid = ?
			)
			SELECT
				COALESCE(round(avg(%s)::numeric, 1), 0)::float AS value,
				count(*) AS count
			FROM availability
			WHERE to_char(update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
			valueColumn,
			valueColumn,
			valueColumn,
		),
		stationUID,
		stationUID,
		hour,
	).Scan(&result)

	return result.Value, result.Count, result.Count > 0
}

func resolveTrainStationID(leg RouteReliabilityLegRequest) (string, string) {
	if DBDashboard == nil {
		return "", "unavailable"
	}

	directID := firstNonEmpty(leg.StationID, leg.StationUID)
	if directID != "" {
		var station struct {
			StationID string
		}
		DBDashboard.Raw(
			`SELECT station_id
			FROM public.train_station
			WHERE station_id = ? OR station_uid = ?
			LIMIT 1`,
			directID,
			directID,
		).Scan(&station)
		if station.StationID != "" {
			return station.StationID, "exact"
		}
		return directID, "exact"
	}

	for _, name := range trainStationNameCandidates(firstNonEmpty(leg.FromName, leg.ToName)) {
		var station struct {
			StationID string
		}
		DBDashboard.Raw(
			`SELECT station_id
			FROM public.train_station
			WHERE station_name_zh = ?
				OR station_name_zh ILIKE ?
				OR ? ILIKE '%' || station_name_zh || '%'
			ORDER BY
				CASE
					WHEN station_name_zh = ? THEN 1
					WHEN ? ILIKE '%' || station_name_zh || '%' THEN 2
					ELSE 3
				END
			LIMIT 1`,
			name,
			"%"+name+"%",
			name,
			name,
			name,
		).Scan(&station)
		if station.StationID != "" {
			return station.StationID, "fallback"
		}
	}

	return "", "unavailable"
}

func resolveYouBikeStationUID(stationName string) string {
	if DBDashboard == nil {
		return ""
	}

	for _, name := range stationNameCandidates(stationName) {
		var station struct {
			StationUID string
		}
		DBDashboard.Raw(
			`SELECT station_uid
			FROM (
				SELECT station_uid, station_name_zh FROM public.ubike_station_tpe
				UNION ALL
				SELECT station_uid, station_name_zh FROM public.ubike_station_new_tpe
			) s
			WHERE station_uid = ?
				OR station_name_zh = ?
				OR station_name_zh ILIKE ?
				OR ? ILIKE '%' || station_name_zh || '%'
			ORDER BY
				CASE
					WHEN station_uid = ? THEN 1
					WHEN station_name_zh = ? THEN 2
					WHEN ? ILIKE '%' || station_name_zh || '%' THEN 3
					ELSE 4
				END
			LIMIT 1`,
			name,
			name,
			"%"+name+"%",
			name,
			name,
			name,
			name,
		).Scan(&station)
		if station.StationUID != "" {
			return station.StationUID
		}
	}

	return ""
}

func parseReliabilityTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid departure_time")
	}
	return parsed.In(taipeiRouteReliabilityLocation), nil
}

func taipeiHourBucket(value time.Time) string {
	return value.In(taipeiRouteReliabilityLocation).Format("15:00")
}

func unavailableLegReliability(id string, mode string, reason string, source string) RouteLegReliability {
	return RouteLegReliability{
		ID:           id,
		Mode:         mode,
		Available:    false,
		Reason:       reason,
		Metrics:      []RouteReliabilityMetric{},
		Source:       source,
		MatchQuality: "unavailable",
	}
}

func normalizeRouteMode(mode string) string {
	return strings.ToUpper(strings.TrimSpace(mode))
}

func busRouteNameCandidates(routeName string) []string {
	candidates := stationNameCandidates(routeName)
	more := []string{}
	for _, candidate := range candidates {
		more = append(more, strings.ReplaceAll(candidate, "公車", ""))
		more = append(more, strings.ReplaceAll(candidate, "路線", ""))
	}
	return uniqueNonEmptyStrings(append(candidates, more...))
}

func roundReliabilityValue(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func trainStationNameCandidates(name string) []string {
	candidates := stationNameCandidates(name)
	more := []string{}
	for _, candidate := range candidates {
		more = append(more, strings.TrimSuffix(candidate, "火車站"))
		more = append(more, strings.TrimSuffix(candidate, "車站"))
		more = append(more, strings.TrimPrefix(candidate, "台鐵"))
		more = append(more, strings.TrimPrefix(candidate, "臺鐵"))
	}
	return uniqueNonEmptyStrings(append(candidates, more...))
}

func stationNameCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return []string{}
	}

	candidates := []string{name}
	candidates = append(candidates, strings.ReplaceAll(name, "台", "臺"))
	candidates = append(candidates, strings.ReplaceAll(name, "臺", "台"))
	return uniqueNonEmptyStrings(candidates)
}

func uniqueNonEmptyStrings(values []string) []string {
	candidates := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		candidates = append(candidates, value)
	}
	return candidates
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
