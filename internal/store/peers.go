package store

import (
	"context"
	"database/sql"

	"github.com/dx111ge/homelabmon/internal/models"
)

func (s *Store) UpsertPeer(ctx context.Context, p *models.PeerInfo) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO peers (id, hostname, address, last_heartbeat, status, version)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname=excluded.hostname, address=excluded.address,
			last_heartbeat=excluded.last_heartbeat, status=excluded.status,
			version=excluded.version
	`, p.ID, p.Hostname, p.Address, p.LastHeartbeat, p.Status, p.Version)
	return err
}

func (s *Store) GetPeer(ctx context.Context, id string) (*models.PeerInfo, error) {
	p := &models.PeerInfo{}
	var lastHB, enrolledAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, hostname, address, last_heartbeat, status, version, enrolled_at FROM peers WHERE id = ?",
		id).Scan(&p.ID, &p.Hostname, &p.Address, &lastHB, &p.Status, &p.Version, &enrolledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastHB.Valid {
		t := models.ParseDBTime(lastHB.String)
		p.LastHeartbeat = &t
	}
	if enrolledAt.Valid {
		p.EnrolledAt = models.ParseDBTime(enrolledAt.String)
	}
	return p, nil
}

func (s *Store) ListPeers(ctx context.Context) ([]models.PeerInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, hostname, address, last_heartbeat, status, version, enrolled_at FROM peers ORDER BY hostname")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []models.PeerInfo
	for rows.Next() {
		var p models.PeerInfo
		var lastHB, enrolledAt sql.NullString
		if err := rows.Scan(&p.ID, &p.Hostname, &p.Address, &lastHB, &p.Status, &p.Version, &enrolledAt); err != nil {
			return nil, err
		}
		if lastHB.Valid {
			t := models.ParseDBTime(lastHB.String)
			p.LastHeartbeat = &t
		}
		if enrolledAt.Valid {
			p.EnrolledAt = models.ParseDBTime(enrolledAt.String)
		}
		peers = append(peers, p)
	}
	return peers, nil
}

func (s *Store) DeletePeer(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM peers WHERE id = ?", id)
	return err
}
