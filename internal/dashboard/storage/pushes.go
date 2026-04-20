package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
)

// SavePush persists a validated push request atomically.
//
// It writes:
//   - one row in `pushes` with the raw protobuf payload
//   - one row per Metric from every ModuleData in `metrics`
//   - an upsert in `agents` (first_seen_at preserved on conflict, last_push_at updated)
//
// All writes happen in a single transaction. If any step fails,
// the whole push is rolled back and an error is returned.
//
// SavePush trusts its input: validation (version, signature, nonce uniqueness,
// timestamp skew) is the caller's responsibility.
func (s *Storage) SavePush(ctx context.Context, req *bientotv1.PushRequest) error {
	if s.db == nil {
		return fmt.Errorf("storage is closed")
	}
	if req == nil {
		return fmt.Errorf("savepush: nil request")
	}

	// Re-marshal the request to get canonical bytes for raw_payload.
	// Deterministic so the stored blob is always the same for the same logical push.
	rawBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal push for storage: %w", err)
	}

	receivedAt := time.Now().UnixNano()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Defer ensures rollback on any early return — commit will void this.
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO pushes (machine_id, timestamp_ns, nonce, raw_payload, received_at)
		 VALUES (?, ?, ?, ?, ?)`,
		req.MachineId, req.TimestampNs, req.Nonce, rawBytes, receivedAt,
	)
	if err != nil {
		return fmt.Errorf("insert push: %w", err)
	}

	pushID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get push id: %w", err)
	}

	// Prepared statement reused for all metrics in this push.
	metricStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO metrics (push_id, machine_id, module, name, value, labels, timestamp_ns)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare metric stmt: %w", err)
	}
	defer metricStmt.Close()

	for _, mod := range req.Modules {
		for _, m := range mod.Metrics {
			labelsJSON, err := marshalLabels(m.Labels)
			if err != nil {
				return fmt.Errorf("marshal labels for %s/%s: %w", mod.Module, m.Name, err)
			}

			_, err = metricStmt.ExecContext(ctx,
				pushID,
				req.MachineId,
				mod.Module,
				m.Name,
				m.Value,
				labelsJSON,
				mod.TimestampNs,
			)
			if err != nil {
				return fmt.Errorf("insert metric %s/%s: %w", mod.Module, m.Name, err)
			}
		}
	}

	// first_seen_at preserved on conflict; last_push_at always updated.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO agents (machine_id, first_seen_at, last_push_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(machine_id) DO UPDATE SET
		   last_push_at = excluded.last_push_at`,
		req.MachineId, receivedAt, receivedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert agent: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// marshalLabels serializes a label map to JSON for storage in the `labels` column.
// Returns sql.NullString to keep NULL for empty maps (saves space, cleaner queries).
func marshalLabels(labels map[string]string) (sql.NullString, error) {
	if len(labels) == 0 {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(labels)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}
