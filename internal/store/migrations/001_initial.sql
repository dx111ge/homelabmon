-- Hosts: unified table for agent nodes, passive devices, and API-discovered devices
CREATE TABLE IF NOT EXISTS hosts (
    id             TEXT PRIMARY KEY,
    hostname       TEXT NOT NULL,
    monitor_type   TEXT NOT NULL DEFAULT 'agent',
    device_type    TEXT NOT NULL DEFAULT 'server',
    os             TEXT NOT NULL DEFAULT '',
    arch           TEXT NOT NULL DEFAULT '',
    platform       TEXT NOT NULL DEFAULT '',
    kernel         TEXT NOT NULL DEFAULT '',
    cpu_model      TEXT NOT NULL DEFAULT '',
    cpu_cores      INTEGER NOT NULL DEFAULT 0,
    memory_total   INTEGER NOT NULL DEFAULT 0,
    ip_addresses   TEXT NOT NULL DEFAULT '[]',
    mac_address    TEXT NOT NULL DEFAULT '',
    vendor         TEXT NOT NULL DEFAULT '',
    discovered_via TEXT NOT NULL DEFAULT 'agent',
    first_seen     DATETIME NOT NULL DEFAULT (datetime('now')),
    last_seen      DATETIME NOT NULL DEFAULT (datetime('now')),
    status         TEXT NOT NULL DEFAULT 'unknown'
);
CREATE INDEX IF NOT EXISTS idx_hosts_monitor_type ON hosts(monitor_type);
CREATE INDEX IF NOT EXISTS idx_hosts_status ON hosts(status);

-- Metrics: time-series for agent nodes
CREATE TABLE IF NOT EXISTS metrics (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id          TEXT NOT NULL REFERENCES hosts(id),
    collected_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    cpu_percent      REAL NOT NULL DEFAULT 0,
    load_1           REAL NOT NULL DEFAULT 0,
    load_5           REAL NOT NULL DEFAULT 0,
    load_15          REAL NOT NULL DEFAULT 0,
    mem_total        INTEGER NOT NULL DEFAULT 0,
    mem_used         INTEGER NOT NULL DEFAULT 0,
    mem_percent      REAL NOT NULL DEFAULT 0,
    swap_total       INTEGER NOT NULL DEFAULT 0,
    swap_used        INTEGER NOT NULL DEFAULT 0,
    disk_json        TEXT NOT NULL DEFAULT '[]',
    net_bytes_sent   INTEGER NOT NULL DEFAULT 0,
    net_bytes_recv   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_metrics_host_time ON metrics(host_id, collected_at);

-- Discovered services
CREATE TABLE IF NOT EXISTS services (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id    TEXT NOT NULL REFERENCES hosts(id),
    name       TEXT NOT NULL DEFAULT '',
    port       INTEGER NOT NULL,
    protocol   TEXT NOT NULL DEFAULT 'tcp',
    process    TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'active',
    first_seen DATETIME NOT NULL DEFAULT (datetime('now')),
    last_seen  DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(host_id, port, protocol)
);

-- Alerts
CREATE TABLE IF NOT EXISTS alerts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id     TEXT NOT NULL REFERENCES hosts(id),
    severity    TEXT NOT NULL DEFAULT 'warning',
    category    TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_alerts_host ON alerts(host_id);

-- Peers: known mesh nodes
CREATE TABLE IF NOT EXISTS peers (
    id             TEXT PRIMARY KEY,
    hostname       TEXT NOT NULL DEFAULT '',
    address        TEXT NOT NULL,
    last_heartbeat DATETIME,
    status         TEXT NOT NULL DEFAULT 'unknown',
    version        TEXT NOT NULL DEFAULT '',
    enrolled_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Plugin state
CREATE TABLE IF NOT EXISTS plugins (
    name       TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'registered',
    config     TEXT NOT NULL DEFAULT '{}',
    last_run   DATETIME,
    error_msg  TEXT NOT NULL DEFAULT ''
);
