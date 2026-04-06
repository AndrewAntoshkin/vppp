package peer

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS peers (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT NOT NULL,
		public_key    TEXT NOT NULL UNIQUE,
		private_key   TEXT NOT NULL,
		preshared_key TEXT NOT NULL,
		allowed_ips   TEXT NOT NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS server_config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	_, err := s.db.Exec(query)
	return err
}

func (s *Store) Create(p *Peer) error {
	result, err := s.db.Exec(
		`INSERT INTO peers (name, public_key, private_key, preshared_key, allowed_ips, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.PublicKey, p.PrivateKey, p.PresharedKey, p.AllowedIPs, p.Enabled, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert peer: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	p.ID = int(id)
	return nil
}

func (s *Store) GetByID(id int) (*Peer, error) {
	p := &Peer{}
	err := s.db.QueryRow(
		`SELECT id, name, public_key, private_key, preshared_key, allowed_ips, enabled, created_at
		 FROM peers WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.PublicKey, &p.PrivateKey, &p.PresharedKey, &p.AllowedIPs, &p.Enabled, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query peer: %w", err)
	}
	return p, nil
}

func (s *Store) GetByPublicKey(pubKey string) (*Peer, error) {
	p := &Peer{}
	err := s.db.QueryRow(
		`SELECT id, name, public_key, private_key, preshared_key, allowed_ips, enabled, created_at
		 FROM peers WHERE public_key = ?`, pubKey,
	).Scan(&p.ID, &p.Name, &p.PublicKey, &p.PrivateKey, &p.PresharedKey, &p.AllowedIPs, &p.Enabled, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query peer by pubkey: %w", err)
	}
	return p, nil
}

func (s *Store) List() ([]Peer, error) {
	rows, err := s.db.Query(
		`SELECT id, name, public_key, private_key, preshared_key, allowed_ips, enabled, created_at
		 FROM peers ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query peers: %w", err)
	}
	defer rows.Close()

	var peers []Peer
	for rows.Next() {
		var p Peer
		if err := rows.Scan(&p.ID, &p.Name, &p.PublicKey, &p.PrivateKey, &p.PresharedKey, &p.AllowedIPs, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan peer: %w", err)
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

func (s *Store) ListAllowedIPs() ([]string, error) {
	rows, err := s.db.Query(`SELECT allowed_ips FROM peers`)
	if err != nil {
		return nil, fmt.Errorf("query allowed_ips: %w", err)
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, fmt.Errorf("scan ip: %w", err)
		}
		ips = append(ips, ip)
	}
	return ips, rows.Err()
}

func (s *Store) Delete(id int) error {
	result, err := s.db.Exec(`DELETE FROM peers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete peer: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("peer %d not found", id)
	}
	return nil
}

func (s *Store) SetEnabled(id int, enabled bool) error {
	result, err := s.db.Exec(`UPDATE peers SET enabled = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return fmt.Errorf("update peer: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("peer %d not found", id)
	}
	return nil
}

func (s *Store) SetServerConfig(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO server_config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value,
	)
	return err
}

func (s *Store) GetServerConfig(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM server_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) Close() error {
	return s.db.Close()
}
