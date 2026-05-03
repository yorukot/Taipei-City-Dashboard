// Package controllers stores all the controllers for the Gin router.
package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/app/util"

	"github.com/gin-gonic/gin"
)

func handleComponentChartDataError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, models.ErrMissingChartSelector) {
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"status": "error", "message": err.Error()})
}

func resolveChartQueryType(queryType string) (baseQueryType string, requireSelectors bool, ok bool) {
	switch queryType {
	case "two_d", "three_d", "percent", "time", "map_legend", "five_d":
		return queryType, false, true
	case "two_selector", "two_selectors", "two_selector_three_d":
		return "three_d", true, true
	case "two_selector_two_d":
		return "two_d", true, true
	case "two_selector_percent":
		return "percent", true, true
	case "two_selector_time":
		return "time", true, true
	case "two_selector_map_legend":
		return "map_legend", true, true
	default:
		return "", false, false
	}
}

/*
GetComponentChartData retrieves the chart data for a component.
/api/v1/components/:id/chart

header: time_from, time_to (optional)
*/
func GetComponentChartData(c *gin.Context) {
	// 1. Get the component id from the URL
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid component ID"})
		return
	}

	// 1.1 Get the city name from the URL
	var query componentQuery
	c.ShouldBindQuery(&query)
	if !(query.City == "taipei" || query.City == "metrotaipei" || query.City == "") {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid City Name"})
		return
	}

	if query.City == "" {
		query.City = "taipei"
	}

	// 2. Get the chart data query and chart data type from the database
	queryType, queryString, err := models.GetComponentChartDataQuery(id, query.City)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if (queryString == "") || (queryType == "") {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "No chart data available"})
		return
	}

	timeFrom, timeTo, err := util.GetTime(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	chartQueryParams := models.ChartQueryParams{
		TimeFrom:  timeFrom,
		TimeTo:    timeTo,
		Selector1: c.Query("selector_1"),
		Selector2: c.Query("selector_2"),
	}
	baseQueryType, requireSelectors, ok := resolveChartQueryType(queryType)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid chart query type"})
		return
	}
	chartQueryParams.RequireSelectors = requireSelectors

	// 3. Get and parse the chart data based on chart data type
	if baseQueryType == "two_d" {
		chartData, err := models.GetTwoDimensionalData(&queryString, chartQueryParams)
		if err != nil {
			handleComponentChartDataError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": chartData})
	} else if baseQueryType == "three_d" || baseQueryType == "percent" {
		chartData, categories, err := models.GetThreeDimensionalData(&queryString, chartQueryParams)
		if err != nil {
			handleComponentChartDataError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": chartData, "categories": categories})
	} else if baseQueryType == "five_d" {
		chartData, err := models.GetFiveDimensionalData(&queryString, chartQueryParams)
		if err != nil {
			handleComponentChartDataError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": chartData})
	} else if baseQueryType == "time" {
		chartData, err := models.GetTimeSeriesData(&queryString, chartQueryParams)
		if err != nil {
			handleComponentChartDataError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": chartData})
	} else if baseQueryType == "map_legend" {
		chartData, err := models.GetMapLegendData(&queryString, chartQueryParams)
		if err != nil {
			handleComponentChartDataError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": chartData})
	}
}

/*
GetComponentHistoryData retrieves the history data for a component.
/api/v1/components/:id/history

header: time_from, time_to (mandatory)
timesteps are automatically determined based on the time range:
  - Within 24hrs: hour
  - Within 1 month: day
  - Within 3 months: week
  - Within 2 years: month
  - More than 2 years: year
*/
func GetComponentHistoryData(c *gin.Context) {
	// 1. Get the component id from the URL
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid component ID"})
		return
	}

	// 1.1 Get the city name from the URL
	var query componentQuery
	c.ShouldBindQuery(&query)
	if !(query.City == "taipei" || query.City == "metrotaipei" || query.City == "") {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid City Name"})
		return
	}

	if query.City == "" {
		query.City = "taipei"
	}

	timeFrom, timeTo, err := util.GetTime(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	// 2. Get the history data query from the database
	queryHistory, err := models.GetComponentHistoryDataQuery(id, query.City, timeFrom, timeTo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if queryHistory == "" {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "No history data available"})
		return
	}

	// 3. Get and parse the history data
	chartData, err := models.GetTimeSeriesData(&queryHistory, models.ChartQueryParams{TimeFrom: timeFrom, TimeTo: timeTo})
	if err != nil {
		handleComponentChartDataError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": chartData})
}
