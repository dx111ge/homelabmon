package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Integration represents a configured external integration.
type Integration struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Config   map[string]string `json:"config"`
	Enabled  bool              `json:"enabled"`
	Status   string            `json:"status"`
	Error    string            `json:"error,omitempty"`
	LastSync *time.Time        `json:"last_sync,omitempty"`
}

func (s *Store) UpsertIntegration(ctx context.Context, ig *Integration) error {
	configJSON, _ := json.Marshal(ig.Config)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO integrations (id, type, name, config, enabled, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, config = excluded.config,
			enabled = excluded.enabled, status = excluded.status, error = excluded.error
	`, ig.ID, ig.Type, ig.Name, string(configJSON), ig.Enabled, ig.Status, ig.Error)
	return err
}

func (s *Store) GetIntegration(ctx context.Context, id string) (*Integration, error) {
	ig := &Integration{}
	var configJSON string
	var enabled int
	var lastSync sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, type, name, config, enabled, status, error, last_sync
		FROM integrations WHERE id = ?
	`, id).Scan(&ig.ID, &ig.Type, &ig.Name, &configJSON, &enabled, &ig.Status, &ig.Error, &lastSync)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ig.Enabled = enabled != 0
	json.Unmarshal([]byte(configJSON), &ig.Config)
	if lastSync.Valid {
		t := parseTime(lastSync.String)
		ig.LastSync = &t
	}
	return ig, nil
}

func (s *Store) ListIntegrations(ctx context.Context) ([]Integration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, name, config, enabled, status, error, last_sync
		FROM integrations ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Integration
	for rows.Next() {
		var ig Integration
		var configJSON string
		var enabled int
		var lastSync sql.NullString
		if err := rows.Scan(&ig.ID, &ig.Type, &ig.Name, &configJSON, &enabled, &ig.Status, &ig.Error, &lastSync); err != nil {
			return nil, err
		}
		ig.Enabled = enabled != 0
		json.Unmarshal([]byte(configJSON), &ig.Config)
		if lastSync.Valid {
			t := parseTime(lastSync.String)
			ig.LastSync = &t
		}
		list = append(list, ig)
	}
	return list, nil
}

func (s *Store) DeleteIntegration(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM integrations WHERE id = ?", id)
	return err
}

func (s *Store) UpdateIntegrationStatus(ctx context.Context, id, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE integrations SET status = ?, error = ?, last_sync = datetime('now') WHERE id = ?
	`, status, errMsg, id)
	return err
}

func parseTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
