package controllers

import (
	"net/http"

	"TaipeiCityDashboardBE/app/models"

	"github.com/gin-gonic/gin"
)

func GetRouteReliability(c *gin.Context) {
	var req models.RouteReliabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request body"})
		return
	}
	if req.DepartureTime == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "departure_time is required"})
		return
	}
	if len(req.Legs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "legs is required"})
		return
	}

	result, err := models.GetRouteReliability(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": result})
}
