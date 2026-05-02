package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	RouteReliabilityGreen   = "green"
	RouteReliabilityYellow  = "yellow"
	RouteReliabilityRed     = "red"
	RouteReliabilityUnknown = "unknown"
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
	FromName          string  `json:"from_name"`
	ToName            string  `json:"to_name"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	DurationSeconds   float64 `json:"duration_seconds"`
	StationUID        string  `json:"station_uid"`
	PickupStationUID  string  `json:"pickup_station_uid"`
	ReturnStationUID  string  `json:"return_station_uid"`
	TrainNo           string  `json:"train_no"`
	TrainTypeCode     string  `json:"train_type_code"`
	PickupStationName string  `json:"pickup_station_name"`
	ReturnStationName string  `json:"return_station_name"`
}

type RouteReliabilityOutput struct {
	RouteStatus string                `json:"route_status"`
	Legs        []RouteLegReliability `json:"legs"`
}

type RouteLegReliability struct {
	ID           string                   `json:"id"`
	Status       string                   `json:"status"`
	Label        string                   `json:"label"`
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

type reliabilityAggregate struct {
	Value float64 `gorm:"column:value"`
	Count int64   `gorm:"column:count"`
}

type railReliabilityAggregate struct {
	DelayRate       float64 `gorm:"column:delay_rate"`
	SevereDelayRate float64 `gorm:"column:severe_delay_rate"`
	AvgDelayMinutes float64 `gorm:"column:avg_delay_minutes"`
	Count           int64   `gorm:"column:count"`
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

	return RouteReliabilityOutput{
		RouteStatus: aggregateRouteReliabilityStatus(legs),
		Legs:        legs,
	}, nil
}

func analyzeRouteLegReliability(leg RouteReliabilityLegRequest, legTime time.Time) RouteLegReliability {
	mode := strings.ToUpper(strings.TrimSpace(leg.Mode))
	switch mode {
	case "BUS":
		return analyzeBusReliability(leg, legTime)
	case "RAIL", "TRAIN", "TRA":
		return analyzeRailReliability(leg, legTime)
	case "SUBWAY", "MRT":
		return greenLegReliability(leg.ID, "捷運路段預設為可靠", "mrt_default")
	case "BICYCLE_RENTAL", "BIKE_RENTAL", "YOUBIKE":
		return analyzeYouBikeReliability(leg, legTime)
	case "WALK":
		return unavailableLegReliability(leg.ID, "步行路段無可靠度資料", "")
	default:
		return unavailableLegReliability(leg.ID, "此交通模式暫無可靠度資料", "")
	}
}

func analyzeBusReliability(leg RouteReliabilityLegRequest, legTime time.Time) RouteLegReliability {
	if DBDashboard == nil {
		return unavailableLegReliability(leg.ID, "資料庫尚未連線，無法計算公車可靠度", "mv_bus_arrival_error")
	}

	weekdayName := taipeiWeekdayName(legTime)
	hour := taipeiHourBucket(legTime)
	routeNames := busRouteNameCandidates(leg.RouteName)
	stopName := strings.TrimSpace(leg.FromName)

	var result reliabilityAggregate
	matchQuality := "exact"
	if len(routeNames) > 0 && stopName != "" {
		DBDashboard.Raw(
			`SELECT COALESCE(avg(y_axis), 0) AS value, count(*) AS count
			FROM public.mv_bus_arrival_error
			WHERE route_name IN ? AND stop_name = ? AND weekday_name = ? AND x_axis = ?`,
			routeNames,
			stopName,
			weekdayName,
			hour,
		).Scan(&result)
	}

	if result.Count == 0 && len(routeNames) > 0 {
		matchQuality = "fallback"
		DBDashboard.Raw(
			`SELECT COALESCE(avg(y_axis), 0) AS value, count(*) AS count
			FROM public.mv_bus_arrival_error
			WHERE route_name IN ? AND weekday_name = ? AND x_axis = ?`,
			routeNames,
			weekdayName,
			hour,
		).Scan(&result)
	}

	if result.Count == 0 {
		matchQuality = "fallback"
		DBDashboard.Raw(
			`SELECT COALESCE(avg(y_axis), 0) AS value, count(*) AS count
			FROM public.mv_bus_arrival_error
			WHERE weekday_name = ? AND x_axis = ?`,
			weekdayName,
			hour,
		).Scan(&result)
	}

	if result.Count == 0 {
		return unavailableLegReliability(leg.ID, "查無同時段公車到站預估誤差資料", "mv_bus_arrival_error")
	}

	status := classifyBusArrivalError(result.Value)
	return RouteLegReliability{
		ID:           leg.ID,
		Status:       status,
		Label:        routeReliabilityLabel(status),
		Reason:       fmt.Sprintf("同時段公車平均到站預估誤差約 %.1f 分鐘", result.Value),
		Source:       "mv_bus_arrival_error",
		MatchQuality: matchQuality,
		Metrics: []RouteReliabilityMetric{
			{
				Key:   "arrival_error_minutes",
				Label: "平均到站誤差",
				Value: roundReliabilityValue(result.Value),
				Unit:  "分鐘",
			},
		},
	}
}

func analyzeRailReliability(leg RouteReliabilityLegRequest, legTime time.Time) RouteLegReliability {
	if DBDashboard == nil {
		return unavailableLegReliability(leg.ID, "資料庫尚未連線，無法計算台鐵可靠度", "train_realtime")
	}

	hour := taipeiHourBucket(legTime)
	trainNo := strings.TrimSpace(leg.TrainNo)
	stationName := strings.TrimSpace(leg.FromName)
	trainTypeCode := strings.TrimSpace(leg.TrainTypeCode)

	var result railReliabilityAggregate
	matchQuality := "exact"
	if trainNo != "" {
		DBDashboard.Raw(
			`SELECT
				COALESCE(avg((delay_time >= 5)::int) * 100, 0) AS delay_rate,
				COALESCE(avg((delay_time >= 10)::int) * 100, 0) AS severe_delay_rate,
				COALESCE(avg(delay_time), 0) AS avg_delay_minutes,
				count(*) AS count
			FROM public.train_realtime
			WHERE train_no = ?
				AND to_char(update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
			trainNo,
			hour,
		).Scan(&result)
	}

	if result.Count == 0 && stationName != "" {
		matchQuality = "fallback"
		if trainTypeCode != "" {
			DBDashboard.Raw(
				`SELECT
					COALESCE(avg((r.delay_time >= 5)::int) * 100, 0) AS delay_rate,
					COALESCE(avg((r.delay_time >= 10)::int) * 100, 0) AS severe_delay_rate,
					COALESCE(avg(r.delay_time), 0) AS avg_delay_minutes,
					count(*) AS count
				FROM public.train_realtime r
				JOIN public.train_station s ON s.station_id = r.station_id
				WHERE s.station_name_zh = ?
					AND r.train_type_code = ?
					AND to_char(r.update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
				stationName,
				trainTypeCode,
				hour,
			).Scan(&result)
		} else {
			DBDashboard.Raw(
				`SELECT
					COALESCE(avg((r.delay_time >= 5)::int) * 100, 0) AS delay_rate,
					COALESCE(avg((r.delay_time >= 10)::int) * 100, 0) AS severe_delay_rate,
					COALESCE(avg(r.delay_time), 0) AS avg_delay_minutes,
					count(*) AS count
				FROM public.train_realtime r
				JOIN public.train_station s ON s.station_id = r.station_id
				WHERE s.station_name_zh = ?
					AND to_char(r.update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
				stationName,
				hour,
			).Scan(&result)
		}
	}

	if result.Count == 0 {
		matchQuality = "fallback"
		DBDashboard.Raw(
			`SELECT
				COALESCE(avg((delay_time >= 5)::int) * 100, 0) AS delay_rate,
				COALESCE(avg((delay_time >= 10)::int) * 100, 0) AS severe_delay_rate,
				COALESCE(avg(delay_time), 0) AS avg_delay_minutes,
				count(*) AS count
			FROM public.train_realtime
			WHERE to_char(update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
			hour,
		).Scan(&result)
	}

	if result.Count == 0 {
		return unavailableLegReliability(leg.ID, "查無同時段台鐵誤點資料", "train_realtime")
	}

	status := classifyRailDelay(result.SevereDelayRate, result.AvgDelayMinutes)
	return RouteLegReliability{
		ID:           leg.ID,
		Status:       status,
		Label:        routeReliabilityLabel(status),
		Reason:       fmt.Sprintf("同時段台鐵平均誤點 %.1f 分鐘，嚴重誤點率 %.1f%%", result.AvgDelayMinutes, result.SevereDelayRate),
		Source:       "train_realtime",
		MatchQuality: matchQuality,
		Metrics: []RouteReliabilityMetric{
			{Key: "delay_rate", Label: "誤點率", Value: roundReliabilityValue(result.DelayRate), Unit: "%"},
			{Key: "severe_delay_rate", Label: "嚴重誤點率", Value: roundReliabilityValue(result.SevereDelayRate), Unit: "%"},
			{Key: "avg_delay_minutes", Label: "平均誤點", Value: roundReliabilityValue(result.AvgDelayMinutes), Unit: "分鐘"},
		},
	}
}

func analyzeYouBikeReliability(leg RouteReliabilityLegRequest, legTime time.Time) RouteLegReliability {
	if DBDashboard == nil {
		return unavailableLegReliability(leg.ID, "資料庫尚未連線，無法計算 YouBike 可靠度", "ubike_availability")
	}

	hour := taipeiHourBucket(legTime)
	pickupUID := firstNonEmpty(leg.PickupStationUID, leg.StationUID)
	returnUID := leg.ReturnStationUID
	if pickupUID == "" && leg.PickupStationName != "" {
		pickupUID = resolveYouBikeStationUID(leg.PickupStationName)
	}
	if returnUID == "" && leg.ReturnStationName != "" {
		returnUID = resolveYouBikeStationUID(leg.ReturnStationName)
	}
	if pickupUID == "" && leg.FromName != "" {
		pickupUID = resolveYouBikeStationUID(leg.FromName)
	}
	if returnUID == "" && leg.ToName != "" {
		returnUID = resolveYouBikeStationUID(leg.ToName)
	}

	var metrics []RouteReliabilityMetric
	var rates []float64
	if pickupUID != "" {
		if rate, ok := queryYouBikeSuccessRate(pickupUID, "rent", hour); ok {
			rates = append(rates, rate)
			metrics = append(metrics, RouteReliabilityMetric{
				Key:   "rent_success_rate",
				Label: "可借成功率",
				Value: roundReliabilityValue(rate),
				Unit:  "%",
			})
		}
	}
	if returnUID != "" {
		if rate, ok := queryYouBikeSuccessRate(returnUID, "return", hour); ok {
			rates = append(rates, rate)
			metrics = append(metrics, RouteReliabilityMetric{
				Key:   "return_success_rate",
				Label: "可還成功率",
				Value: roundReliabilityValue(rate),
				Unit:  "%",
			})
		}
	}

	if len(rates) == 0 {
		return unavailableLegReliability(leg.ID, "查無同時段 YouBike 可借可還資料", "ubike_availability")
	}

	worstRate := minReliabilityValue(rates)
	status := classifyYouBikeSuccessRate(worstRate)
	return RouteLegReliability{
		ID:           leg.ID,
		Status:       status,
		Label:        routeReliabilityLabel(status),
		Reason:       fmt.Sprintf("同時段 YouBike 最低可用成功率約 %.1f%%", worstRate),
		Source:       "ubike_availability",
		MatchQuality: "exact",
		Metrics:      metrics,
	}
}

func queryYouBikeSuccessRate(stationUID string, side string, hour string) (float64, bool) {
	valueColumn := "available_rent_bikes"
	if side == "return" {
		valueColumn = "available_return_bikes"
	}

	var result reliabilityAggregate
	DBDashboard.Raw(
		fmt.Sprintf(
			`WITH stations AS (
				SELECT 'taipei' AS city, station_uid, bikes_capacity FROM public.ubike_station_tpe WHERE station_uid = ?
				UNION ALL
				SELECT 'new_tpe' AS city, station_uid, bikes_capacity FROM public.ubike_station_new_tpe WHERE station_uid = ?
			),
			availability AS (
				SELECT 'taipei' AS city, station_uid, %s, update_time FROM public.ubike_availability_tpe WHERE station_uid = ?
				UNION ALL
				SELECT 'new_tpe' AS city, station_uid, %s, update_time FROM public.ubike_availability_new_tpe WHERE station_uid = ?
			)
			SELECT
				COALESCE(avg(a.%s::float / NULLIF(s.bikes_capacity, 0) * 100), 0) AS value,
				count(*) AS count
			FROM stations s
			JOIN availability a ON a.city = s.city AND a.station_uid = s.station_uid
			WHERE s.bikes_capacity > 0
				AND to_char(a.update_time AT TIME ZONE 'Asia/Taipei', 'HH24:00') = ?`,
			valueColumn,
			valueColumn,
			valueColumn,
		),
		stationUID,
		stationUID,
		stationUID,
		stationUID,
		hour,
	).Scan(&result)

	return result.Value, result.Count > 0
}

func resolveYouBikeStationUID(stationName string) string {
	name := strings.TrimSpace(stationName)
	if name == "" || DBDashboard == nil {
		return ""
	}

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
		ORDER BY
			CASE
				WHEN station_uid = ? THEN 1
				WHEN station_name_zh = ? THEN 2
				ELSE 3
			END
		LIMIT 1`,
		name,
		name,
		"%"+name+"%",
		name,
		name,
	).Scan(&station)

	return station.StationUID
}

func parseReliabilityTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid departure_time")
	}
	return parsed.In(taipeiRouteReliabilityLocation), nil
}

func taipeiWeekdayName(value time.Time) string {
	switch value.In(taipeiRouteReliabilityLocation).Weekday() {
	case time.Monday:
		return "星期一"
	case time.Tuesday:
		return "星期二"
	case time.Wednesday:
		return "星期三"
	case time.Thursday:
		return "星期四"
	case time.Friday:
		return "星期五"
	case time.Saturday:
		return "星期六"
	default:
		return "星期日"
	}
}

func taipeiHourBucket(value time.Time) string {
	return value.In(taipeiRouteReliabilityLocation).Format("15:00")
}

func busRouteNameCandidates(routeName string) []string {
	candidates := []string{}
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		candidates = append(candidates, value)
	}

	add(routeName)
	cleaned := strings.ReplaceAll(routeName, "公車", "")
	cleaned = strings.ReplaceAll(cleaned, "路線", "")
	add(cleaned)
	return candidates
}

func classifyBusArrivalError(minutes float64) string {
	if minutes <= 5 {
		return RouteReliabilityGreen
	}
	if minutes <= 10 {
		return RouteReliabilityYellow
	}
	return RouteReliabilityRed
}

func classifyRailDelay(severeDelayRate float64, avgDelayMinutes float64) string {
	if severeDelayRate >= 25 || avgDelayMinutes >= 10 {
		return RouteReliabilityRed
	}
	if severeDelayRate >= 10 || avgDelayMinutes >= 5 {
		return RouteReliabilityYellow
	}
	return RouteReliabilityGreen
}

func classifyYouBikeSuccessRate(rate float64) string {
	if rate >= 70 {
		return RouteReliabilityGreen
	}
	if rate >= 40 {
		return RouteReliabilityYellow
	}
	return RouteReliabilityRed
}

func aggregateRouteReliabilityStatus(legs []RouteLegReliability) string {
	hasGreen := false
	hasYellow := false
	for _, leg := range legs {
		switch leg.Status {
		case RouteReliabilityRed:
			return RouteReliabilityRed
		case RouteReliabilityYellow:
			hasYellow = true
		case RouteReliabilityGreen:
			hasGreen = true
		}
	}
	if hasYellow {
		return RouteReliabilityYellow
	}
	if hasGreen {
		return RouteReliabilityGreen
	}
	return RouteReliabilityUnknown
}

func unavailableLegReliability(id string, reason string, source string) RouteLegReliability {
	return RouteLegReliability{
		ID:           id,
		Status:       RouteReliabilityUnknown,
		Label:        routeReliabilityLabel(RouteReliabilityUnknown),
		Reason:       reason,
		Metrics:      []RouteReliabilityMetric{},
		Source:       source,
		MatchQuality: "unavailable",
	}
}

func greenLegReliability(id string, reason string, source string) RouteLegReliability {
	return RouteLegReliability{
		ID:           id,
		Status:       RouteReliabilityGreen,
		Label:        routeReliabilityLabel(RouteReliabilityGreen),
		Reason:       reason,
		Metrics:      []RouteReliabilityMetric{},
		Source:       source,
		MatchQuality: "default",
	}
}

func routeReliabilityLabel(status string) string {
	switch status {
	case RouteReliabilityGreen:
		return "可靠"
	case RouteReliabilityYellow:
		return "中等風險"
	case RouteReliabilityRed:
		return "不可靠"
	default:
		return "資料不足"
	}
}

func roundReliabilityValue(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func minReliabilityValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
