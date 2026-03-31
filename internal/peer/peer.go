package peer

import "time"

type Peer struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	PublicKey    string    `json:"public_key"`
	PrivateKey   string    `json:"private_key,omitempty"`
	PresharedKey string    `json:"preshared_key,omitempty"`
	AllowedIPs   string    `json:"allowed_ips"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`

	// Live status (populated from wg show, not stored)
	Endpoint        string `json:"endpoint,omitempty"`
	LatestHandshake string `json:"latest_handshake,omitempty"`
	TransferRx      string `json:"transfer_rx,omitempty"`
	TransferTx      string `json:"transfer_tx,omitempty"`
}
