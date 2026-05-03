// Package models stores the models for the postgreSQL databases.
package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

/* ----- Models ----- */

// ChartDataQuery is the model for getting the chart data query.
type ChartDataQuery struct {
	QueryType  string `json:"query_type" gorm:"column:query_type"`
	QueryChart string `json:"query_chart" gorm:"column:query_chart"`
}

// HistoryDataQuery is the model for getting the history data query.
type HistoryDataQuery struct {
	QueryHistory string `json:"query_history" gorm:"column:query_history"`
}

/*
TwoDimensionalData Json Format:

	{
		"data": [
			{
				"data": [
					{ "x": "", "y": 17 },
					...
				]
			}
		]
	}
*/
type TwoDimensionalData struct {
	Xaxis string  `gorm:"column:x_axis" json:"x"`
	Data  float64 `gorm:"column:data" json:"y"`
}
type TwoDimensionalDataOutput struct {
	Data []TwoDimensionalData `json:"data"`
}

/*
ThreeDimensionalData & PercentData Json Format:

	{
		"data": [
			{
				"name": ""
				"data": [...]
			},
			...
		]
	}

>> ThreeDimensionalData is shared by 3D and percentage data
*/
type ThreeDimensionalData struct {
	Xaxis string `gorm:"column:x_axis"`
	Icon  string `gorm:"column:icon"`
	Yaxis string `gorm:"column:y_axis"`
	Data  int    `gorm:"column:data"`
}

/*
BoxPlotData Json Format (per ApexCharts boxPlot series):

	{
		"data": [
			{
				"data": [
					{ "x": "label", "y": [min, q1, median, q3, max] },
					...
				]
			}
		]
	}

The query_chart SQL must return columns: x_axis, min, q1, median, q3, max.
*/
type BoxPlotData struct {
	Xaxis  string  `gorm:"column:x_axis"`
	Min    float64 `gorm:"column:min"`
	Q1     float64 `gorm:"column:q1"`
	Median float64 `gorm:"column:median"`
	Q3     float64 `gorm:"column:q3"`
	Max    float64 `gorm:"column:max"`
}

type BoxPlotDataItem struct {
	X string    `json:"x"`
	Y []float64 `json:"y"`
}

type BoxPlotDataOutput struct {
	Data []BoxPlotDataItem `json:"data"`
}

type ThreeDimensionalDataOutput struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	Data []int  `json:"data"`
}

/*
TimeSeriesData Json Format:

	{
		"data": [
			{
				"name": "",
				"data": [
					{ "x": "2023-05-25T06:29:00+08:00", "y": 17 },
					...
				]
			},
			...
		]
	}
*/
type TimeSeriesData struct {
	Xaxis time.Time `gorm:"column:x_axis"`
	Yaxis string    `gorm:"column:y_axis"`
	Data  float64   `gorm:"column:data"`
}

type TimeSeriesDataItem struct {
	X string  `json:"x"`
	Y float64 `json:"y"`
}

type TimeSeriesDataOutput struct {
	Name string               `json:"name"`
	Data []TimeSeriesDataItem `json:"data"`
}

/*
MapLegendData Json Format:
*/
type MapLegendData struct {
	Name  string  `gorm:"column:name" json:"name"`
	Type  string  `gorm:"column:type" json:"type"`
	Icon  string  `gorm:"column:icon" json:"icon"`
	Value float64 `gorm:"column:value" json:"value"`
}

// ErrMissingChartSelector is returned when a query_chart requires a selector
// query parameter but the request did not provide it.
var ErrMissingChartSelector = errors.New("missing required chart selector")

// ChartQueryParams contains all dynamic values supported by query_charts.query_chart.
type ChartQueryParams struct {
	TimeFrom         string
	TimeTo           string
	Selector1        string
	Selector2        string
	RequireSelectors bool
}

/* ----- Handlers ----- */

func GetComponentChartDataQuery(id int, city string) (queryType string, queryString string, err error) {
	var chartDataQuery ChartDataQuery

	err = DBManager.
		Table("components").
		Select("query_charts.query_type, query_charts.query_chart").
		Joins("LEFT JOIN query_charts ON components.index = query_charts.index").
		Where("components.id = ?", id).
		Where("query_charts.city = ?", city).
		Find(&chartDataQuery).Error
	if err != nil {
		return queryType, queryString, err
	}
	return chartDataQuery.QueryType, chartDataQuery.QueryChart, nil
}

func GetComponentHistoryDataQuery(id int, city string, timeFrom string, timeTo string) (queryHistory string, err error) {
	var historyDataQuery HistoryDataQuery

	err = DBManager.
		Table("components").
		Select("query_charts.query_history").
		Joins("LEFT JOIN query_charts ON components.index = query_charts.index").
		Where("components.id = ?", id).
		Where("query_charts.city = ?", city).
		Find(&historyDataQuery).Error
	if err != nil {
		return queryHistory, err
	}
	if historyDataQuery.QueryHistory == "" {
		return historyDataQuery.QueryHistory, err
	}

	var timeStepUnit string

	timeFromTime, err := time.Parse("2006-01-02T15:04:05+08:00", timeFrom)
	if err != nil {
		return queryHistory, err
	}
	timeToTime, err := time.Parse("2006-01-02T15:04:05+08:00", timeTo)
	if err != nil {
		return queryHistory, err
	}

	/*
			timesteps are automatically determined based on the time range:
		  - Within 24hrs: hour
		  - Within 1 month: day
		  - Within 3 months: week
		  - Within 2 years: month
		  - More than 2 years: year
	*/
	if timeToTime.Sub(timeFromTime).Hours() <= 24 {
		timeStepUnit = "hour" // Within 24hrs
	} else if timeToTime.Sub(timeFromTime).Hours() < 24*32 {
		timeStepUnit = "day" // Within 1 month
	} else if timeToTime.Sub(timeFromTime).Hours() < 24*93 {
		timeStepUnit = "week" // Within 3 months
	} else if timeToTime.Sub(timeFromTime).Hours() < 24*740 {
		timeStepUnit = "month" // Within 2 years
	} else {
		timeStepUnit = "year" // More than 2 years
	}

	// Insert the time range and timestep unit into the query
	var queryInsertStrings []any

	if strings.Count(historyDataQuery.QueryHistory, "%s")%3 != 0 {
		return queryHistory, fmt.Errorf("invalid query string")
	}

	for i := 0; i < strings.Count(historyDataQuery.QueryHistory, "%s")/3; i++ {
		queryInsertStrings = append(queryInsertStrings, timeStepUnit, timeFrom, timeTo)
	}

	historyDataQuery.QueryHistory = fmt.Sprintf(historyDataQuery.QueryHistory, queryInsertStrings...)

	return historyDataQuery.QueryHistory, nil
}

/*
Below are the parsing functions for the four data types:
two_d, three_d, percent, and time. three_d and percent data share a common handler.
*/

func buildChartQuery(query string, params ChartQueryParams) (queryString string, args []any, err error) {
	queryString = query

	// Preserve the legacy query format where exactly two %s placeholders mean
	// timefrom and timeto.
	if strings.Count(queryString, "%s") == 2 {
		queryString = fmt.Sprintf(queryString, params.TimeFrom, params.TimeTo)
	}

	requiredSelectors := []struct {
		name  string
		value string
	}{
		{name: "selector_1", value: params.Selector1},
		{name: "selector_2", value: params.Selector2},
	}

	for _, selector := range requiredSelectors {
		if (params.RequireSelectors || hasNamedParam(queryString, selector.name)) && selector.value == "" {
			return "", nil, fmt.Errorf("%w: %s", ErrMissingChartSelector, selector.name)
		}
	}

	// query_charts uses :name placeholders, while GORM named args use @name.
	queryString = normalizeChartNamedParams(queryString)

	namedArgs := map[string]any{
		"timefrom":   params.TimeFrom,
		"timeto":     params.TimeTo,
		"selector_1": params.Selector1,
		"selector_2": params.Selector2,
	}

	for _, name := range []string{"timefrom", "timeto", "selector_1", "selector_2"} {
		if hasNamedParam(queryString, name) {
			args = append(args, sql.Named(name, namedArgs[name]))
		}
	}

	return queryString, args, nil
}

func hasNamedParam(query string, name string) bool {
	return hasNamedPlaceholder(query, ":"+name) || hasNamedPlaceholder(query, "@"+name)
}

func hasNamedPlaceholder(query string, placeholder string) bool {
	searchFrom := 0
	for {
		idx := strings.Index(query[searchFrom:], placeholder)
		if idx == -1 {
			return false
		}
		idx += searchFrom
		end := idx + len(placeholder)
		if end >= len(query) || !isParamNameChar(query[end]) {
			return true
		}
		searchFrom = end
	}
}

func isParamNameChar(char byte) bool {
	return char == '_' ||
		(char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9')
}

func normalizeChartNamedParams(query string) string {
	for _, name := range []string{"timefrom", "timeto", "selector_1", "selector_2"} {
		query = replaceNamedPlaceholder(query, ":"+name, "@"+name)
	}
	return query
}

func replaceNamedPlaceholder(query string, placeholder string, replacement string) string {
	searchFrom := 0
	changed := false
	var builder strings.Builder

	for {
		idx := strings.Index(query[searchFrom:], placeholder)
		if idx == -1 {
			if !changed {
				return query
			}
			builder.WriteString(query[searchFrom:])
			return builder.String()
		}

		idx += searchFrom
		end := idx + len(placeholder)
		if end < len(query) && isParamNameChar(query[end]) {
			if changed {
				builder.WriteString(query[searchFrom:end])
			}
			searchFrom = end
			continue
		}

		if !changed {
			builder.Grow(len(query))
			changed = true
		}
		builder.WriteString(query[searchFrom:idx])
		if strings.HasPrefix(query[end:], "::") {
			builder.WriteString("(")
			builder.WriteString(replacement)
			builder.WriteString(")")
		} else {
			builder.WriteString(replacement)
		}
		searchFrom = end
	}
}

func scanChartData(query *string, params ChartQueryParams, dest any) error {
	queryString, args, err := buildChartQuery(*query, params)
	if err != nil {
		return err
	}
	return DBDashboard.Raw(queryString, args...).Scan(dest).Error
}

func GetTwoDimensionalData(query *string, params ChartQueryParams) (chartDataOutput []TwoDimensionalDataOutput, err error) {
	var chartData []TwoDimensionalData

	// 2. Get the data from the database
	err = scanChartData(query, params, &chartData)
	if err != nil {
		return chartDataOutput, err
	}
	if len(chartData) == 0 {
		return chartDataOutput, err
	}

	// 3. Convert the data to the format required by the front-end
	chartDataOutput = append(chartDataOutput, TwoDimensionalDataOutput{Data: chartData})

	return chartDataOutput, nil
}

func GetThreeDimensionalData(query *string, params ChartQueryParams) (chartDataOutput []ThreeDimensionalDataOutput, categories []string, err error) {
	var chartData []ThreeDimensionalData

	// 2. Get the data from the database
	err = scanChartData(query, params, &chartData)
	if err != nil {
		return chartDataOutput, categories, err
	}
	if len(chartData) == 0 {
		return chartDataOutput, categories, err
	}

	// 3. Convert the data to the format required by the front-end
	for _, data := range chartData {
		// Get unique categories from xAxis
		var foundX bool
		for _, category := range categories {
			if category == data.Xaxis {
				foundX = true
				break
			}
		}

		// If a unique xAxis is found, append it to the existing list of categories
		if !foundX {
			categories = append(categories, data.Xaxis)
		}

		// Group data together by yAxis
		var foundY bool
		for i, output := range chartDataOutput {
			if output.Name == data.Yaxis {
				// Append the data to the output
				chartDataOutput[i].Data = append(output.Data, data.Data)
				foundY = true
				break
			}
		}

		// If a unique yAxis is found, create a new entry in the output
		if !foundY {
			chartDataOutput = append(chartDataOutput, ThreeDimensionalDataOutput{Name: data.Yaxis, Icon: data.Icon, Data: []int{data.Data}})
		}
	}

	return chartDataOutput, categories, nil
}

func GetBoxPlotData(query *string, params ChartQueryParams) (chartDataOutput []BoxPlotDataOutput, err error) {
	var chartData []BoxPlotData

	err = scanChartData(query, params, &chartData)
	if err != nil {
		return chartDataOutput, err
	}
	if len(chartData) == 0 {
		return chartDataOutput, err
	}

	items := make([]BoxPlotDataItem, 0, len(chartData))
	for _, row := range chartData {
		items = append(items, BoxPlotDataItem{
			X: row.Xaxis,
			Y: []float64{row.Min, row.Q1, row.Median, row.Q3, row.Max},
		})
	}

	chartDataOutput = append(chartDataOutput, BoxPlotDataOutput{Data: items})
	return chartDataOutput, nil
}

func GetTimeSeriesData(query *string, params ChartQueryParams) (chartDataOutput []TimeSeriesDataOutput, err error) {
	var chartData []TimeSeriesData

	// 2. Get the data from the database
	err = scanChartData(query, params, &chartData)
	if err != nil {
		return chartDataOutput, err
	}
	if len(chartData) == 0 {
		return chartDataOutput, err
	}

	// 3. Convert the data to the format required by the front-end
	for _, data := range chartData {
		// Group data together by yAxis
		var foundY bool
		formattedDate := data.Xaxis.Format("2006-01-02T15:04:05+08:00")
		for i, output := range chartDataOutput {
			if output.Name == data.Yaxis {
				// Append the data to the output
				chartDataOutput[i].Data = append(output.Data, TimeSeriesDataItem{X: formattedDate, Y: data.Data})
				foundY = true
				break
			}
		}

		// If a unique yAxis is found, create a new entry in the output
		if !foundY {
			chartDataOutput = append(chartDataOutput, TimeSeriesDataOutput{Name: data.Yaxis, Data: []TimeSeriesDataItem{{X: formattedDate, Y: data.Data}}})
		}
	}

	return chartDataOutput, nil
}

func GetMapLegendData(query *string, params ChartQueryParams) (chartData []MapLegendData, err error) {
	// 2. Get the data from the database
	err = scanChartData(query, params, &chartData)
	if err != nil {
		return chartData, err
	}
	if len(chartData) == 0 {
		return chartData, err
	}

	return chartData, nil
}
