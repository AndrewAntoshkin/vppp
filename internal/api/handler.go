package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/andrewaitken/vppp/internal/peer"
	"github.com/andrewaitken/vppp/internal/qr"
	"github.com/andrewaitken/vppp/internal/wireguard"
)

type Handler struct {
	store   *peer.Store
	wg      *wireguard.Manager
	mux     *http.ServeMux
	webDir  string
}

func NewHandler(store *peer.Store, wg *wireguard.Manager, webDir string) *Handler {
	h := &Handler{store: store, wg: wg, webDir: webDir}
	h.mux = http.NewServeMux()
	h.registerRoutes()
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) registerRoutes() {
	h.mux.HandleFunc("/healthz", h.handleHealth)
	h.mux.HandleFunc("/api/peers", h.handlePeers)
	h.mux.HandleFunc("/api/peers/", h.handlePeerByID)
	h.mux.HandleFunc("/api/status", h.handleStatus)
	h.mux.HandleFunc("/api/server", h.handleServerInfo)
	h.mux.Handle("/", http.FileServer(http.Dir(h.webDir)))
}

type createPeerRequest struct {
	Name string `json:"name"`
}

type createPeerResponse struct {
	Peer   peer.Peer `json:"peer"`
	Config string    `json:"config"`
	QR     string    `json:"qr"`
}

func (h *Handler) handlePeers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listPeers(w, r)
	case http.MethodPost:
		h.createPeer(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listPeers(w http.ResponseWriter, _ *http.Request) {
	peers, err := h.store.List()
	if err != nil {
		jsonError(w, "list peers: "+err.Error(), http.StatusInternalServerError)
		return
	}

	statuses, err := h.wg.GetStatus()
	if err != nil {
		log.Printf("warning: could not get wg status: %v", err)
	}

	statusMap := make(map[string]wireguard.PeerStatus)
	for _, s := range statuses {
		statusMap[s.PublicKey] = s
	}
	for i := range peers {
		peers[i].PrivateKey = ""
		peers[i].PresharedKey = ""
		if s, ok := statusMap[peers[i].PublicKey]; ok {
			peers[i].Endpoint = s.Endpoint
			peers[i].LatestHandshake = s.LatestHandshake
			peers[i].TransferRx = s.TransferRx
			peers[i].TransferTx = s.TransferTx
		}
	}

	jsonResponse(w, peers, http.StatusOK)
}

func (h *Handler) createPeer(w http.ResponseWriter, r *http.Request) {
	var req createPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}

	keys, err := wireguard.GenerateKeyPair()
	if err != nil {
		jsonError(w, "generate keys: "+err.Error(), http.StatusInternalServerError)
		return
	}

	psk, err := wireguard.GeneratePresharedKey()
	if err != nil {
		jsonError(w, "generate preshared key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	usedIPs, err := h.store.ListAllowedIPs()
	if err != nil {
		jsonError(w, "list ips: "+err.Error(), http.StatusInternalServerError)
		return
	}

	network := strings.Split(h.wg.Address, "/")[0]
	baseNetwork := network[:strings.LastIndex(network, ".")] + ".0/24"

	allocatedIP, err := wireguard.AllocateIP(baseNetwork, usedIPs)
	if err != nil {
		jsonError(w, "allocate ip: "+err.Error(), http.StatusInternalServerError)
		return
	}

	p := &peer.Peer{
		Name:         req.Name,
		PublicKey:    keys.PublicKey,
		PrivateKey:   keys.PrivateKey,
		PresharedKey: psk,
		AllowedIPs:   allocatedIP,
		Enabled:      true,
	}

	if err := h.store.Create(p); err != nil {
		jsonError(w, "create peer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.wg.AddPeer(p.PublicKey, p.AllowedIPs, p.PresharedKey); err != nil {
		log.Printf("warning: could not add peer to wg (will apply on restart): %v", err)
	}

	serverPubKey, _ := h.store.GetServerConfig("public_key")
	config, err := wireguard.GenerateClientConfig(wireguard.ClientConfig{
		PrivateKey:      keys.PrivateKey,
		Address:         allocatedIP,
		DNS:             h.wg.DNS,
		ServerPublicKey: serverPubKey,
		PresharedKey:    psk,
		Endpoint:        h.wg.Endpoint,
		Port:            h.wg.ListenPort,
	})
	if err != nil {
		jsonError(w, "generate config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	qrBase64, err := qr.GenerateBase64(config, 512)
	if err != nil {
		log.Printf("warning: could not generate qr: %v", err)
	}

	resp := createPeerResponse{
		Peer:   *p,
		Config: config,
		QR:     qrBase64,
	}

	jsonResponse(w, resp, http.StatusCreated)
}

func (h *Handler) handlePeerByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/peers/")

	parts := strings.Split(idStr, "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "invalid peer id", http.StatusBadRequest)
		return
	}

	if len(parts) == 2 && parts[1] == "config" && r.Method == http.MethodGet {
		h.getPeerConfig(w, id)
		return
	}

	if len(parts) == 2 && parts[1] == "qr" && r.Method == http.MethodGet {
		h.getPeerQR(w, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getPeer(w, id)
	case http.MethodDelete:
		h.deletePeer(w, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) getPeer(w http.ResponseWriter, id int) {
	p, err := h.store.GetByID(id)
	if err != nil {
		jsonError(w, "get peer: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		jsonError(w, "peer not found", http.StatusNotFound)
		return
	}
	p.PrivateKey = ""
	p.PresharedKey = ""
	jsonResponse(w, p, http.StatusOK)
}

func (h *Handler) getPeerConfig(w http.ResponseWriter, id int) {
	p, err := h.store.GetByID(id)
	if err != nil {
		jsonError(w, "get peer: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		jsonError(w, "peer not found", http.StatusNotFound)
		return
	}

	serverPubKey, _ := h.store.GetServerConfig("public_key")
	config, err := wireguard.GenerateClientConfig(wireguard.ClientConfig{
		PrivateKey:      p.PrivateKey,
		Address:         p.AllowedIPs,
		DNS:             h.wg.DNS,
		ServerPublicKey: serverPubKey,
		PresharedKey:    p.PresharedKey,
		Endpoint:        h.wg.Endpoint,
		Port:            h.wg.ListenPort,
	})
	if err != nil {
		jsonError(w, "generate config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.conf"`, p.Name))
	w.Write([]byte(config))
}

func (h *Handler) getPeerQR(w http.ResponseWriter, id int) {
	p, err := h.store.GetByID(id)
	if err != nil {
		jsonError(w, "get peer: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		jsonError(w, "peer not found", http.StatusNotFound)
		return
	}

	serverPubKey, _ := h.store.GetServerConfig("public_key")
	config, err := wireguard.GenerateClientConfig(wireguard.ClientConfig{
		PrivateKey:      p.PrivateKey,
		Address:         p.AllowedIPs,
		DNS:             h.wg.DNS,
		ServerPublicKey: serverPubKey,
		PresharedKey:    p.PresharedKey,
		Endpoint:        h.wg.Endpoint,
		Port:            h.wg.ListenPort,
	})
	if err != nil {
		jsonError(w, "generate config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	png, err := qr.GeneratePNG(config, 512)
	if err != nil {
		jsonError(w, "generate qr: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

func (h *Handler) deletePeer(w http.ResponseWriter, id int) {
	p, err := h.store.GetByID(id)
	if err != nil {
		jsonError(w, "get peer: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		jsonError(w, "peer not found", http.StatusNotFound)
		return
	}

	if err := h.wg.RemovePeer(p.PublicKey); err != nil {
		log.Printf("warning: could not remove peer from wg: %v", err)
	}

	if err := h.store.Delete(id); err != nil {
		jsonError(w, "delete peer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	statuses, err := h.wg.GetStatus()
	if err != nil {
		jsonError(w, "get status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, statuses, http.StatusOK)
}

func (h *Handler) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	pubKey, _ := h.store.GetServerConfig("public_key")
	info := map[string]interface{}{
		"interface":   h.wg.InterfaceName,
		"listen_port": h.wg.ListenPort,
		"address":     h.wg.Address,
		"endpoint":    h.wg.Endpoint,
		"public_key":  pubKey,
		"dns":         h.wg.DNS,
	}
	jsonResponse(w, info, http.StatusOK)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
