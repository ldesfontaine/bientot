package api

import (
	"net/http"
	"strconv"
	"time"
)

const (
	maxPointsPerResponse = 10000
	defaultRange         = 24 * time.Hour
)

// metricPointsResponse is the JSON envelope for a single metric's time series.
type metricPointsResponse struct {
	Name      string     `json:"name"`
	MachineID string     `json:"machineId"`
	Start     int64      `json:"start"` // Unix ms
	End       int64      `json:"end"`   // Unix ms
	Points    []pointDTO `json:"points"`
}

// pointDTO is the compact representation of a (timestamp, value) sample.
// Compact field names ("t", "v") keep the response small for series that
// can carry thousands of points (uPlot-friendly).
type pointDTO struct {
	T int64   `json:"t"` // Unix ms
	V float64 `json:"v"`
}

// handleGetMetricPoints returns the time-series points for one metric of one agent.
//
// Query parameters:
//   - name (required): the metric name
//   - range (optional): duration string (e.g. "1h", "24h", "30m"); default 24h
//   - start, end (optional): Unix ms timestamps; if both given, override range
//
// Returns 404 if the agent doesn't exist; 400 if name missing or params malformed;
// 200 with empty points array if agent exists but has no data in range.
//
// Route: GET /api/agents/{id}/metric-points
func (s *Server) handleGetMetricPoints(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	if machineID == "" {
		writeError(w, s.log, http.StatusBadRequest, "missing agent id")
		return
	}

	q := r.URL.Query()

	name := q.Get("name")
	if name == "" {
		writeError(w, s.log, http.StatusBadRequest, "missing required parameter: name")
		return
	}

	start, end, err := parseTimeRange(q, time.Now())
	if err != nil {
		writeError(w, s.log, http.StatusBadRequest, err.Error())
		return
	}

	exists, err := s.db.AgentExists(r.Context(), machineID)
	if err != nil {
		s.log.Error("check agent existence failed", "machine_id", machineID, "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "internal error")
		return
	}
	if !exists {
		writeError(w, s.log, http.StatusNotFound, "agent not found")
		return
	}

	points, err := s.db.GetMetricPoints(r.Context(), machineID, name, start, end)
	if err != nil {
		s.log.Error("get metric points failed",
			"machine_id", machineID, "name", name, "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "failed to fetch metric points")
		return
	}

	if len(points) > maxPointsPerResponse {
		writeError(w, s.log, http.StatusBadRequest,
			"range too wide: would return more than "+strconv.Itoa(maxPointsPerResponse)+" points")
		return
	}

	dtos := make([]pointDTO, 0, len(points))
	for _, p := range points {
		dtos = append(dtos, pointDTO{
			T: timestampNsToMillis(p.TimestampNs),
			V: p.Value,
		})
	}

	resp := metricPointsResponse{
		Name:      name,
		MachineID: machineID,
		Start:     start.UnixMilli(),
		End:       end.UnixMilli(),
		Points:    dtos,
	}

	writeJSON(w, s.log, http.StatusOK, resp)
}

// parseTimeRange resolves the start/end window from query params.
//
// Precedence:
//  1. Both `start` and `end` present → absolute mode
//  2. Else `range` present → relative mode (start = now - range, end = now)
//  3. Else → default range (24h ending now)
//
// Returns error for malformed values or invalid ranges.
func parseTimeRange(q map[string][]string, now time.Time) (start, end time.Time, err error) {
	startStr := getQuery(q, "start")
	endStr := getQuery(q, "end")

	if startStr != "" && endStr != "" {
		startMs, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			return time.Time{}, time.Time{}, errBadParam("start", "must be Unix ms integer")
		}
		endMs, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			return time.Time{}, time.Time{}, errBadParam("end", "must be Unix ms integer")
		}
		start = time.UnixMilli(startMs)
		end = time.UnixMilli(endMs)
		if !start.Before(end) {
			return time.Time{}, time.Time{}, errBadParam("start/end", "start must be before end")
		}
		return start, end, nil
	}

	if startStr != "" || endStr != "" {
		return time.Time{}, time.Time{}, errBadParam("start/end", "both must be provided together")
	}

	rangeStr := getQuery(q, "range")
	rangeDur := defaultRange
	if rangeStr != "" {
		d, err := time.ParseDuration(rangeStr)
		if err != nil {
			return time.Time{}, time.Time{}, errBadParam("range", "must be a duration like 1h, 24h, 30m")
		}
		if d <= 0 {
			return time.Time{}, time.Time{}, errBadParam("range", "must be positive")
		}
		rangeDur = d
	}

	end = now
	start = now.Add(-rangeDur)
	return start, end, nil
}

// getQuery returns "" instead of nil/empty for a missing key.
func getQuery(q map[string][]string, key string) string {
	v := q[key]
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func errBadParam(name, reason string) error {
	return badParamError{name: name, reason: reason}
}

type badParamError struct {
	name   string
	reason string
}

func (e badParamError) Error() string {
	return "bad parameter " + e.name + ": " + e.reason
}
