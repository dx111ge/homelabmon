package store

import (
	"context"
	"fmt"

	"github.com/dx111ge/homelabmon/internal/models"
)

func (s *Store) InsertMetric(ctx context.Context, m *models.MetricSnapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO metrics (host_id, collected_at, cpu_percent, load_1, load_5, load_15,
			mem_total, mem_used, mem_percent, swap_total, swap_used,
			disk_json, net_bytes_sent, net_bytes_recv)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.HostID, m.CollectedAt.UTC().Format("2006-01-02 15:04:05"),
		m.CPUPercent, m.Load1, m.Load5, m.Load15,
		m.MemTotal, m.MemUsed, m.MemPercent, m.SwapTotal, m.SwapUsed,
		m.DiskJSON, m.NetBytesSent, m.NetBytesRecv)
	return err
}

func (s *Store) GetLatestMetric(ctx context.Context, hostID string) (*models.MetricSnapshot, error) {
	m := &models.MetricSnapshot{}
	var collectedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, host_id, collected_at, cpu_percent, load_1, load_5, load_15,
			mem_total, mem_used, mem_percent, swap_total, swap_used,
			disk_json, net_bytes_sent, net_bytes_recv
		FROM metrics WHERE host_id = ? ORDER BY collected_at DESC LIMIT 1
	`, hostID).Scan(&m.ID, &m.HostID, &collectedAt,
		&m.CPUPercent, &m.Load1, &m.Load5, &m.Load15,
		&m.MemTotal, &m.MemUsed, &m.MemPercent, &m.SwapTotal, &m.SwapUsed,
		&m.DiskJSON, &m.NetBytesSent, &m.NetBytesRecv)
	if err != nil {
		return nil, err
	}
	m.CollectedAt = models.ParseDBTime(collectedAt)
	return m, nil
}

func (s *Store) GetMetricHistory(ctx context.Context, hostID string, hours int) ([]models.MetricSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, host_id, collected_at, cpu_percent, load_1, load_5, load_15,
			mem_total, mem_used, mem_percent, swap_total, swap_used,
			disk_json, net_bytes_sent, net_bytes_recv
		FROM metrics
		WHERE host_id = ? AND collected_at >= datetime('now', ?)
		ORDER BY collected_at ASC
	`, hostID, fmt.Sprintf("-%d hours", hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.MetricSnapshot
	for rows.Next() {
		var m models.MetricSnapshot
		var collectedAt string
		if err := rows.Scan(&m.ID, &m.HostID, &collectedAt,
			&m.CPUPercent, &m.Load1, &m.Load5, &m.Load15,
			&m.MemTotal, &m.MemUsed, &m.MemPercent, &m.SwapTotal, &m.SwapUsed,
			&m.DiskJSON, &m.NetBytesSent, &m.NetBytesRecv); err != nil {
			return nil, err
		}
		m.CollectedAt = models.ParseDBTime(collectedAt)
		metrics = append(metrics, m)
	}
	return metrics, nil
}

func (s *Store) PurgeOldMetrics(ctx context.Context, retentionDays int) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM metrics WHERE collected_at < datetime('now', ?)",
		fmt.Sprintf("-%d days", retentionDays))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
