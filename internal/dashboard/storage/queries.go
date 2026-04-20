package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Agent represents one monitored machine as seen by the dashboard.
type Agent struct {
	MachineID   string
	FirstSeenAt time.Time
	LastPushAt  time.Time
}

// Metric is the latest recorded value of a named metric for an agent.
type Metric struct {
	Name        string
	Value       float64
	Labels      map[string]string
	Module      string
	TimestampNs int64
}

// MetricPoint is a single (timestamp, value) sample used for charts.
type MetricPoint struct {
	TimestampNs int64
	Value       float64
}

// ListAgents returns all known agents ordered by machine_id ascending.
// Returns an empty slice (not nil) if no agents exist.
func (s *Storage) ListAgents(ctx context.Context) ([]Agent, error) {
	if s.db == nil {
		return nil, fmt.Errorf("storage is closed")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT machine_id, first_seen_at, last_push_at
		 FROM agents
		 ORDER BY machine_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer rows.Close()

	agents := []Agent{}
	for rows.Next() {
		var (
			machineID           string
			firstSeenNs, lastNs int64
		)
		if err := rows.Scan(&machineID, &firstSeenNs, &lastNs); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, Agent{
			MachineID:   machineID,
			FirstSeenAt: time.Unix(0, firstSeenNs),
			LastPushAt:  time.Unix(0, lastNs),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}

	return agents, nil
}

// GetLatestMetrics returns the most recent value of every distinct metric name
// for the given agent. The map key is the metric name.
//
// If the agent has multiple metrics with the same name but different labels
// (e.g. cpu_user_seconds_total{cpu="0"} and {cpu="1"}), only the latest one
// written is returned. For per-label access, use GetMetricPoints with a wider
// filter at query-layer (post-MVP).
//
// Returns an empty map (not nil) if the agent has no metrics yet.
func (s *Storage) GetLatestMetrics(ctx context.Context, machineID string) (map[string]Metric, error) {
	if s.db == nil {
		return nil, fmt.Errorf("storage is closed")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT m.name, m.value, m.labels, m.module, m.timestamp_ns
		 FROM metrics m
		 INNER JOIN (
		   SELECT name, MAX(timestamp_ns) AS max_ts
		   FROM metrics
		   WHERE machine_id = ?
		   GROUP BY name
		 ) latest ON m.name = latest.name AND m.timestamp_ns = latest.max_ts
		 WHERE m.machine_id = ?
		 GROUP BY m.name
		 HAVING m.id = MAX(m.id)`,
		machineID, machineID,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest metrics: %w", err)
	}
	defer rows.Close()

	result := make(map[string]Metric)
	for rows.Next() {
		var (
			m        Metric
			labelStr sql.NullString
		)
		if err := rows.Scan(&m.Name, &m.Value, &labelStr, &m.Module, &m.TimestampNs); err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		if labelStr.Valid {
			if err := json.Unmarshal([]byte(labelStr.String), &m.Labels); err != nil {
				return nil, fmt.Errorf("unmarshal labels for %s: %w", m.Name, err)
			}
		}
		result[m.Name] = m
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest metrics: %w", err)
	}

	return result, nil
}

// ModuleInfo summarizes one module's metric activity for an agent.
// Used by the dashboard to populate the "active modules" grid.
type ModuleInfo struct {
	Module       string
	MetricCount  int       // distinct metric names recorded for this module
	LastUpdateAt time.Time // most recent push that carried any metric
}

// ListModulesForAgent returns the list of distinct modules with at least
// one metric recorded for the given agent. Ordered alphabetically.
// Returns an empty slice (not nil) if the agent has no metrics.
func (s *Storage) ListModulesForAgent(ctx context.Context, machineID string) ([]ModuleInfo, error) {
	if s.db == nil {
		return nil, fmt.Errorf("storage is closed")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT
		   module,
		   COUNT(DISTINCT name) AS metric_count,
		   MAX(timestamp_ns)    AS last_update_ns
		 FROM metrics
		 WHERE machine_id = ?
		 GROUP BY module
		 ORDER BY module ASC`,
		machineID,
	)
	if err != nil {
		return nil, fmt.Errorf("query modules for agent: %w", err)
	}
	defer rows.Close()

	modules := []ModuleInfo{}
	for rows.Next() {
		var (
			name       string
			count      int
			lastUpdate int64
		)
		if err := rows.Scan(&name, &count, &lastUpdate); err != nil {
			return nil, fmt.Errorf("scan module info: %w", err)
		}
		modules = append(modules, ModuleInfo{
			Module:       name,
			MetricCount:  count,
			LastUpdateAt: time.Unix(0, lastUpdate),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate modules: %w", err)
	}

	return modules, nil
}

// AgentExists returns true if an agent with the given machine_id has been
// seen at least once. Fast: hits the primary key on agents.
func (s *Storage) AgentExists(ctx context.Context, machineID string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("storage is closed")
	}

	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM agents WHERE machine_id = ? LIMIT 1`,
		machineID,
	).Scan(&one)

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check agent existence: %w", err)
	}
	return true, nil
}

// GetMetricPoints returns all recorded (timestamp, value) samples for one
// metric of one agent in the half-open interval [start, end).
//
// Results are ordered by timestamp ascending — ready to feed a line chart.
// Multiple points can exist at the same timestamp (one per label set);
// the caller aggregates if needed.
//
// Returns an empty slice (not nil) if no points match.
func (s *Storage) GetMetricPoints(
	ctx context.Context,
	machineID, metricName string,
	start, end time.Time,
) ([]MetricPoint, error) {
	if s.db == nil {
		return nil, fmt.Errorf("storage is closed")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp_ns, value
		 FROM metrics
		 WHERE machine_id = ?
		   AND name = ?
		   AND timestamp_ns >= ?
		   AND timestamp_ns < ?
		 ORDER BY timestamp_ns ASC`,
		machineID, metricName, start.UnixNano(), end.UnixNano(),
	)
	if err != nil {
		return nil, fmt.Errorf("query metric points: %w", err)
	}
	defer rows.Close()

	points := []MetricPoint{}
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.TimestampNs, &p.Value); err != nil {
			return nil, fmt.Errorf("scan metric point: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric points: %w", err)
	}

	return points, nil
}
