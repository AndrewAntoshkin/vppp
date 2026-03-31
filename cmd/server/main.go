package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/andrewaitken/vppp/internal/api"
	"github.com/andrewaitken/vppp/internal/peer"
	"github.com/andrewaitken/vppp/internal/wireguard"
)

func main() {
	listenAddr := flag.String("listen", ":8080", "API listen address")
	dbPath := flag.String("db", "/data/vppp.db", "SQLite database path")
	webDir := flag.String("web", "./web", "Web panel static files directory")
	wgIface := flag.String("wg-iface", "wg0", "WireGuard interface name")
	wgPort := flag.Int("wg-port", 51820, "WireGuard listen port")
	wgAddress := flag.String("wg-address", "10.0.0.1/24", "WireGuard server address")
	wgDNS := flag.String("wg-dns", "1.1.1.1, 8.8.8.8", "DNS servers for clients")
	wgEndpoint := flag.String("wg-endpoint", "", "Public endpoint (IP or domain)")
	apiKeyFlag := flag.String("api-key", "", "API key (auto-generated if empty)")
	flag.Parse()

	if *wgEndpoint == "" {
		*wgEndpoint = os.Getenv("VPN_ENDPOINT")
		if *wgEndpoint == "" {
			log.Fatal("--wg-endpoint or VPN_ENDPOINT is required")
		}
	}

	store, err := peer.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer store.Close()

	wgManager := wireguard.NewManager(*wgIface, *wgPort, *wgAddress, *wgDNS, *wgEndpoint)

	if err := initServerKeys(store, wgManager); err != nil {
		log.Fatalf("init server keys: %v", err)
	}

	apiKey := *apiKeyFlag
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY")
	}
	if apiKey == "" {
		stored, _ := store.GetServerConfig("api_key")
		if stored != "" {
			apiKey = stored
		} else {
			apiKey, err = api.GenerateAPIKey()
			if err != nil {
				log.Fatalf("generate api key: %v", err)
			}
			store.SetServerConfig("api_key", apiKey)
			fmt.Printf("\n=== Generated API Key (save this!) ===\n%s\n======================================\n\n", apiKey)
		}
	}

	handler := api.NewHandler(store, wgManager, *webDir)

	var srv http.Handler = handler
	srv = api.AuthMiddleware(apiKey, srv)
	srv = api.LoggingMiddleware(srv)

	log.Printf("Starting VPN management API on %s", *listenAddr)
	log.Printf("WireGuard interface: %s, port: %d, address: %s", *wgIface, *wgPort, *wgAddress)
	log.Printf("Endpoint: %s", *wgEndpoint)

	if err := http.ListenAndServe(*listenAddr, srv); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func initServerKeys(store *peer.Store, wg *wireguard.Manager) error {
	existing, _ := store.GetServerConfig("private_key")
	if existing != "" {
		return nil
	}

	keys, err := wireguard.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate server keys: %w", err)
	}

	if err := store.SetServerConfig("private_key", keys.PrivateKey); err != nil {
		return fmt.Errorf("store private key: %w", err)
	}
	if err := store.SetServerConfig("public_key", keys.PublicKey); err != nil {
		return fmt.Errorf("store public key: %w", err)
	}

	log.Printf("Generated server keys. Public key: %s", keys.PublicKey)

	peers, err := store.List()
	if err != nil {
		return fmt.Errorf("list peers: %w", err)
	}

	var serverPeers []wireguard.ServerPeerConfig
	for _, p := range peers {
		if p.Enabled {
			serverPeers = append(serverPeers, wireguard.ServerPeerConfig{
				PublicKey:    p.PublicKey,
				PresharedKey: p.PresharedKey,
				AllowedIPs:   p.AllowedIPs,
			})
		}
	}

	config, err := wireguard.GenerateServerConfig(wireguard.ServerConfig{
		PrivateKey: keys.PrivateKey,
		Address:    wg.Address,
		Port:       wg.ListenPort,
		Peers:      serverPeers,
	})
	if err != nil {
		return fmt.Errorf("generate server config: %w", err)
	}

	configPath := fmt.Sprintf("/etc/wireguard/%s.conf", wg.InterfaceName)
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		log.Printf("warning: could not create /etc/wireguard: %v", err)
		return nil
	}
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		log.Printf("warning: could not write wg config: %v", err)
	}

	return nil
}
