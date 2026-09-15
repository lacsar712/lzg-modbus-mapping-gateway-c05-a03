package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bytecode/modbus-mapping-gateway/internal/domain"
	"github.com/bytecode/modbus-mapping-gateway/internal/usecase"
)

func (s *Server) alarms() *usecase.AlarmService { return s.svc.Alarms() }

// GET /api/alarms?status=&deviceId=&point=&limit=
func (s *Server) listAlarms(c *gin.Context) {
	f := usecase.AlarmFilter{
		DeviceID: c.Query("deviceId"),
		Point:    c.Query("point"),
		Status:   c.Query("status"),
	}
	if f.Status != "" && f.Status != domain.AlarmStatusActive && f.Status != domain.AlarmStatusResolved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be active or resolved"})
		return
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		f.Limit = n
	}
	c.JSON(http.StatusOK, gin.H{
		"events":      s.alarms().ListEvents(f),
		"activeCount": s.alarms().ActiveCount(f.DeviceID),
	})
}

// GET /api/alarms/summary
func (s *Server) alarmSummary(c *gin.Context) {
	perDevice := s.alarms().ActiveCounts()
	total := 0
	for _, n := range perDevice {
		total += n
	}
	c.JSON(http.StatusOK, gin.H{"activeCount": total, "perDevice": perDevice})
}

// GET /api/alarm-rules?deviceId=
func (s *Server) listAlarmRules(c *gin.Context) {
	rules := s.alarms().ListRules()
	dev := c.Query("deviceId")
	if dev != "" {
		filtered := make([]domain.AlarmRule, 0, len(rules))
		for _, r := range rules {
			if r.DeviceID == dev {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

type alarmRuleBody struct {
	High    *float64 `json:"high"`
	Low     *float64 `json:"low"`
	Enabled *bool    `json:"enabled"`
}

// PUT /api/devices/:id/alarm-rules/:point
func (s *Server) upsertAlarmRule(c *gin.Context) {
	if !s.requireEngineer(c) {
		return
	}
	var body alarmRuleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body, expect {high, low, enabled}"})
		return
	}
	rule := domain.AlarmRule{
		DeviceID: c.Param("id"),
		Point:    c.Param("point"),
		High:     body.High,
		Low:      body.Low,
		Enabled:  body.Enabled == nil || *body.Enabled,
	}
	saved, err := s.alarms().UpsertRule(rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, saved)
}

// DELETE /api/devices/:id/alarm-rules/:point
func (s *Server) deleteAlarmRule(c *gin.Context) {
	if !s.requireEngineer(c) {
		return
	}
	if err := s.alarms().DeleteRule(c.Param("id"), c.Param("point")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/alarms/:id/clear
func (s *Server) clearAlarm(c *gin.Context) {
	if !s.requireEngineer(c) {
		return
	}
	e, err := s.alarms().ClearEvent(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, e)
}
