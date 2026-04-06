package wireguard

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/crypto/curve25519"
)

type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

type PeerStatus struct {
	PublicKey       string
	Endpoint        string
	AllowedIPs      string
	LatestHandshake string
	TransferRx      string
	TransferTx      string
}

type Manager struct {
	InterfaceName string
	ListenPort    int
	Address       string // e.g. "10.0.0.1/24"
	DNS           string
	Endpoint      string // public IP/domain of the server
	mu            sync.Mutex
}

func NewManager(iface string, port int, address, dns, endpoint string) *Manager {
	return &Manager{
		InterfaceName: iface,
		ListenPort:    port,
		Address:       address,
		DNS:           dns,
		Endpoint:      endpoint,
	}
}

func GenerateKeyPair() (KeyPair, error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return KeyPair{}, fmt.Errorf("generate private key: %w", err)
	}

	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return KeyPair{}, fmt.Errorf("derive public key: %w", err)
	}

	return KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(privateKey[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(publicKey),
	}, nil
}

func GeneratePresharedKey() (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", fmt.Errorf("generate preshared key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key[:]), nil
}

func (m *Manager) AddPeer(publicKey, allowedIPs, presharedKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	args := []string{"set", m.InterfaceName, "peer", publicKey, "allowed-ips", allowedIPs}
	if presharedKey != "" {
		args = append(args, "preshared-key", "/dev/stdin")
	}

	cmd := exec.Command("wg", args...)
	if presharedKey != "" {
		cmd.Stdin = strings.NewReader(presharedKey)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg set peer: %s: %w", string(out), err)
	}

	return m.saveConfig()
}

func (m *Manager) RemovePeer(publicKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.Command("wg", "set", m.InterfaceName, "peer", publicKey, "remove")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg remove peer: %s: %w", string(out), err)
	}

	return m.saveConfig()
}

func (m *Manager) GetStatus() ([]PeerStatus, error) {
	cmd := exec.Command("wg", "show", m.InterfaceName, "dump")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wg show: %w", err)
	}

	var peers []PeerStatus
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 8 {
			continue
		}
		peers = append(peers, PeerStatus{
			PublicKey:       fields[0],
			Endpoint:        fields[2],
			AllowedIPs:      fields[3],
			LatestHandshake: fields[4],
			TransferRx:      fields[5],
			TransferTx:      fields[6],
		})
	}

	return peers, nil
}

func (m *Manager) saveConfig() error {
	cmd := exec.Command("wg-quick", "save", m.InterfaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg-quick save: %s: %w", string(out), err)
	}
	return nil
}

func (m *Manager) RestartInterface() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := exec.Command("wg", "show", m.InterfaceName).Run(); err == nil {
		cmd := exec.Command("wg-quick", "down", m.InterfaceName)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wg-quick down: %s: %w", string(out), err)
		}
	}

	cmd := exec.Command("wg-quick", "up", m.InterfaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg-quick up: %s: %w", string(out), err)
	}

	return nil
}

func AllocateIP(baseNetwork string, usedIPs []string) (string, error) {
	_, network, err := net.ParseCIDR(baseNetwork)
	if err != nil {
		return "", fmt.Errorf("parse network: %w", err)
	}

	used := make(map[string]bool)
	for _, ip := range usedIPs {
		used[strings.Split(ip, "/")[0]] = true
	}

	ip := make(net.IP, len(network.IP))
	copy(ip, network.IP)

	// Skip network address and server address (.0 and .1)
	for i := 2; ; i++ {
		candidate := incrementIP(network.IP, i)
		if !network.Contains(candidate) {
			break
		}
		if !used[candidate.String()] {
			return candidate.String() + "/32", nil
		}
	}

	return "", fmt.Errorf("no available IPs in %s", baseNetwork)
}

func incrementIP(base net.IP, offset int) net.IP {
	ip := make(net.IP, len(base))
	copy(ip, base)

	ip = ip.To4()
	if ip == nil {
		return nil
	}

	val := int(ip[0])<<24 | int(ip[1])<<16 | int(ip[2])<<8 | int(ip[3])
	val += offset

	return net.IPv4(byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
}
