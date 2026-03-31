package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type config struct {
	ServerURL string
	APIKey    string
}

func main() {
	cfg := config{
		ServerURL: envOrDefault("VPN_SERVER_URL", "http://localhost:8080"),
		APIKey:    os.Getenv("VPN_API_KEY"),
	}

	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: VPN_API_KEY environment variable is required")
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "peer":
		err = handlePeer(cfg)
	case "status":
		err = handleStatus(cfg)
	case "server":
		err = handleServer(cfg)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func handlePeer(cfg config) error {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vpn-cli peer <add|list|remove|config|qr> [args]")
		return nil
	}

	switch os.Args[2] {
	case "add":
		return peerAdd(cfg)
	case "list", "ls":
		return peerList(cfg)
	case "remove", "rm":
		return peerRemove(cfg)
	case "config":
		return peerConfig(cfg)
	case "qr":
		return peerQR(cfg)
	default:
		return fmt.Errorf("unknown peer command: %s", os.Args[2])
	}
}

func peerAdd(cfg config) error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: vpn-cli peer add <name>")
	}

	name := strings.Join(os.Args[3:], " ")
	body, _ := json.Marshal(map[string]string{"name": name})

	resp, err := apiRequest(cfg, "POST", "/api/peers", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Peer struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			AllowedIPs string `json:"allowed_ips"`
		} `json:"peer"`
		Config string `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Peer created:\n")
	fmt.Printf("  ID:   %d\n", result.Peer.ID)
	fmt.Printf("  Name: %s\n", result.Peer.Name)
	fmt.Printf("  IP:   %s\n", result.Peer.AllowedIPs)
	fmt.Printf("\n--- Client Config ---\n%s\n", result.Config)
	fmt.Printf("\nUse 'vpn-cli peer qr %d' to show QR code\n", result.Peer.ID)

	return nil
}

func peerList(cfg config) error {
	resp, err := apiRequest(cfg, "GET", "/api/peers", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var peers []struct {
		ID              int    `json:"id"`
		Name            string `json:"name"`
		AllowedIPs      string `json:"allowed_ips"`
		Enabled         bool   `json:"enabled"`
		Endpoint        string `json:"endpoint"`
		LatestHandshake string `json:"latest_handshake"`
		TransferRx      string `json:"transfer_rx"`
		TransferTx      string `json:"transfer_tx"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(peers) == 0 {
		fmt.Println("No peers configured. Use 'vpn-cli peer add <name>' to add one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tIP\tSTATUS\tENDPOINT\tHANDSHAKE\tRX\tTX")
	for _, p := range peers {
		status := "active"
		if !p.Enabled {
			status = "disabled"
		}
		handshake := "-"
		if p.LatestHandshake != "" && p.LatestHandshake != "0" {
			ts, err := strconv.ParseInt(p.LatestHandshake, 10, 64)
			if err == nil && ts > 0 {
				handshake = time.Unix(ts, 0).Format("15:04:05")
			}
		}
		endpoint := p.Endpoint
		if endpoint == "(none)" || endpoint == "" {
			endpoint = "-"
		}
		rx := formatBytes(p.TransferRx)
		tx := formatBytes(p.TransferTx)

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			p.ID, p.Name, p.AllowedIPs, status, endpoint, handshake, rx, tx)
	}
	w.Flush()
	return nil
}

func peerRemove(cfg config) error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: vpn-cli peer remove <id>")
	}
	id := os.Args[3]

	resp, err := apiRequest(cfg, "DELETE", "/api/peers/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("Peer %s removed\n", id)
	return nil
}

func peerConfig(cfg config) error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: vpn-cli peer config <id>")
	}
	id := os.Args[3]

	resp, err := apiRequest(cfg, "GET", "/api/peers/"+id+"/config", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
	return nil
}

func peerQR(cfg config) error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: vpn-cli peer qr <id>")
	}
	id := os.Args[3]

	resp, err := apiRequest(cfg, "GET", "/api/peers/"+id+"/config", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	configData, _ := io.ReadAll(resp.Body)

	qrResp, err := apiRequest(cfg, "GET", "/api/peers/"+id+"/qr", nil)
	if err != nil {
		return err
	}
	defer qrResp.Body.Close()

	if qrResp.Header.Get("Content-Type") == "image/png" {
		fmt.Printf("QR code saved. Use the web panel or download from:\n")
		fmt.Printf("  %s/api/peers/%s/qr?api_key=%s\n", cfg.ServerURL, id, cfg.APIKey)
		fmt.Printf("\n--- Config ---\n%s", string(configData))
	}

	return nil
}

func handleStatus(cfg config) error {
	resp, err := apiRequest(cfg, "GET", "/api/status", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var statuses []struct {
		PublicKey       string `json:"PublicKey"`
		Endpoint        string `json:"Endpoint"`
		LatestHandshake string `json:"LatestHandshake"`
		TransferRx      string `json:"TransferRx"`
		TransferTx      string `json:"TransferTx"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if len(statuses) == 0 {
		fmt.Println("No active peers")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PUBLIC KEY\tENDPOINT\tHANDSHAKE\tRX\tTX")
	for _, s := range statuses {
		fmt.Fprintf(w, "%.16s...\t%s\t%s\t%s\t%s\n",
			s.PublicKey, s.Endpoint, s.LatestHandshake,
			formatBytes(s.TransferRx), formatBytes(s.TransferTx))
	}
	w.Flush()
	return nil
}

func handleServer(cfg config) error {
	resp, err := apiRequest(cfg, "GET", "/api/server", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	fmt.Println("Server Info:")
	for k, v := range info {
		fmt.Printf("  %-12s %v\n", k+":", v)
	}
	return nil
}

func apiRequest(cfg config, method, path string, body io.Reader) (*http.Response, error) {
	url := cfg.ServerURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-API-Key", cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(errBody))
	}
	return resp, nil
}

func formatBytes(s string) string {
	if s == "" {
		return "-"
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s
	}
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func printUsage() {
	fmt.Println(`vpn-cli — VPN peer management tool

Usage:
  vpn-cli <command> [subcommand] [args]

Commands:
  peer add <name>      Add a new VPN peer
  peer list            List all peers with status
  peer remove <id>     Remove a peer
  peer config <id>     Show peer's WireGuard config
  peer qr <id>         Get QR code for mobile setup
  status               Show live WireGuard status
  server               Show server info

Environment:
  VPN_API_KEY          API key (required)
  VPN_SERVER_URL       Server URL (default: http://localhost:8080)`)
}
