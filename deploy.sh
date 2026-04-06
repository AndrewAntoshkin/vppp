#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[vppp]${NC} $*"; }
warn() { echo -e "${YELLOW}[vppp]${NC} $*"; }
err()  { echo -e "${RED}[vppp]${NC} $*" >&2; }

if [ "$(id -u)" -ne 0 ]; then
  err "This script must be run as root"
  exit 1
fi

ENDPOINT="${1:-}"
if [ -z "$ENDPOINT" ]; then
  ENDPOINT=$(curl -4 -s ifconfig.me || true)
  if [ -z "$ENDPOINT" ]; then
    err "Could not detect public IP. Usage: $0 <public-ip-or-domain>"
    exit 1
  fi
  warn "Auto-detected public IP: $ENDPOINT"
fi

WG_PORT="${WG_PORT:-443}"

log "Installing prerequisites..."

if ! command -v docker &>/dev/null; then
  log "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
else
  log "Docker already installed"
fi

if ! command -v docker compose &>/dev/null && ! docker compose version &>/dev/null 2>&1; then
  log "Installing Docker Compose plugin..."
  apt-get update -qq && apt-get install -y -qq docker-compose-plugin
fi

log "Setting up firewall..."
if command -v ufw &>/dev/null; then
  ufw allow 22/tcp comment "SSH"
  ufw allow "${WG_PORT}/udp" comment "WireGuard VPN"
  ufw allow 8080/tcp comment "VPN Management Panel"
  ufw --force enable || true
  log "UFW rules added (SSH + WireGuard + Panel)"
else
  warn "ufw not found, skipping firewall setup. Make sure ports ${WG_PORT}/udp and 8080/tcp are open."
fi

log "Enabling IP forwarding..."
sysctl -w net.ipv4.ip_forward=1
if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf; then
  echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
fi

INSTALL_DIR="/opt/vppp"
log "Setting up project in $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR"

if [ ! -d "$INSTALL_DIR/.git" ]; then
  if [ -f "docker-compose.yml" ]; then
    cp -r . "$INSTALL_DIR/"
  else
    err "Run this script from the project directory, or clone the repo to $INSTALL_DIR first."
    exit 1
  fi
fi

cd "$INSTALL_DIR"

cat > .env <<EOF
VPN_ENDPOINT=$ENDPOINT
WG_PORT=$WG_PORT
EOF

log "Building and starting services..."
docker compose up -d --build

log "Waiting for service to start..."
sleep 3

if docker compose ps | grep -q "running"; then
  log "Service is running!"
  echo ""
  log "=== Setup Complete ==="
  echo ""
  echo "  VPN Endpoint:  $ENDPOINT:$WG_PORT"
  echo "  Web Panel:     http://$ENDPOINT:8080"
  echo ""
  echo "  Your API key is in the container logs:"
  echo "    docker compose logs vpn | grep 'API Key'"
  echo ""
  echo "  Next steps:"
  echo "    1. Get your API key from the logs above"
  echo "    2. Open http://$ENDPOINT:8080 in your browser"
  echo "    3. Enter the API key"
  echo "    4. Add peers and scan QR codes with the WireGuard app"
  echo ""
  warn "For production: set up HTTPS with a reverse proxy (nginx/Caddy)"
else
  err "Service failed to start. Check: docker compose logs"
  exit 1
fi
