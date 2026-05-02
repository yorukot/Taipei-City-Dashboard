package controllers

import (
	"TaipeiCityDashboardBE/app/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetAllRoutes(c *gin.Context) {
	routes, err := models.GetAllRoutes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   routes,
	})
}

func GetAllStopByRouteID(c *gin.Context) {
	routeID := c.Param("route_id")
	routeIDInt, err := strconv.Atoi(routeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid route ID"})
		return
	}
	stops, err := models.GetAllStopByRouteID(routeIDInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stops,
	})
}
