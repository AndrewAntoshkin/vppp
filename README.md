# vppp — Personal WireGuard VPN

Self-hosted VPN service with a management API, web panel, and CLI. Built on WireGuard.

## Quick Start

### 1. Deploy to a VPS

Get a VPS (Hetzner, DigitalOcean, Vultr — any Ubuntu/Debian server, ~$5/mo), clone this repo, and run:

```bash
git clone https://github.com/andrewaitken/vppp.git
cd vppp
sudo ./deploy.sh
```

The script will:
- Install Docker if needed
- Open firewall ports (51820/udp, 8080/tcp)
- Build and start the service
- Print your API key

You can also pass your server's public IP explicitly:

```bash
sudo ./deploy.sh 203.0.113.42
```

### 2. Get Your API Key

```bash
docker compose logs vpn | grep "API Key"
```

### 3. Connect

Open `http://YOUR_SERVER_IP:8080` in your browser, enter the API key, and add peers.

Each peer gets a WireGuard config and QR code — scan it with the [WireGuard app](https://www.wireguard.com/install/) on your phone or import on desktop.

## CLI Usage

Set environment variables:

```bash
export VPN_API_KEY="your-api-key-here"
export VPN_SERVER_URL="http://your-server:8080"
```

Manage peers:

```bash
vpn-cli peer add "Andrew iPhone"     # Add a peer, get config
vpn-cli peer list                     # List all peers with status
vpn-cli peer config 1                 # Show peer's WireGuard config
vpn-cli peer qr 1                    # Get QR code URL
vpn-cli peer remove 1                # Remove a peer
vpn-cli status                       # Live WireGuard status
vpn-cli server                       # Server info
```

## API Endpoints

All `/api/` endpoints require the `X-API-Key` header (or `?api_key=` query param).

| Method   | Path                   | Description              |
|----------|------------------------|--------------------------|
| `GET`    | `/api/peers`           | List all peers           |
| `POST`   | `/api/peers`           | Add a peer `{"name":""}` |
| `GET`    | `/api/peers/:id`       | Get peer details         |
| `DELETE` | `/api/peers/:id`       | Remove a peer            |
| `GET`    | `/api/peers/:id/config`| Download client config   |
| `GET`    | `/api/peers/:id/qr`   | QR code as PNG           |
| `GET`    | `/api/status`          | WireGuard live status    |
| `GET`    | `/api/server`          | Server info              |

## Architecture

```
┌─────────────────────────────────────────────┐
│  VPS                                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │ WireGuard│◄─│ Mgmt API │──│ SQLite   │  │
│  │ (wg0)    │  │ (Go)     │  │          │  │
│  └────▲─────┘  └────▲─────┘  └──────────┘  │
│       │              │                       │
│       │ UDP:51820    │ TCP:8080              │
└───────┼──────────────┼───────────────────────┘
        │              │
   ┌────┴────┐    ┌────┴────┐
   │ WG Apps │    │ Browser │
   │ (peers) │    │ (admin) │
   └─────────┘    └─────────┘
```

## Project Structure

```
vppp/
├── cmd/
│   ├── server/main.go        # API server entry point
│   └── cli/main.go           # CLI tool
├── internal/
│   ├── api/
│   │   ├── handler.go        # REST API handlers
│   │   └── middleware.go     # Auth & logging
│   ├── peer/
│   │   ├── peer.go           # Peer model
│   │   └── store.go          # SQLite storage
│   ├── qr/
│   │   └── qr.go             # QR code generation
│   └── wireguard/
│       ├── wireguard.go      # WG key gen & interface management
│       └── config.go         # Config file generation
├── web/                      # Web panel (static HTML/CSS/JS)
├── docker-compose.yml
├── Dockerfile
└── deploy.sh                 # One-command VPS deploy
```

## Configuration

Environment variables (set in `.env` or pass to Docker):

| Variable       | Required | Default               | Description                    |
|----------------|----------|-----------------------|--------------------------------|
| `VPN_ENDPOINT` | Yes      | —                     | Server public IP or domain     |
| `API_KEY`      | No       | auto-generated        | API key for authentication     |

Server flags (passed as command args):

| Flag             | Default           | Description                    |
|------------------|-------------------|--------------------------------|
| `--listen`       | `:8080`           | API listen address             |
| `--db`           | `/data/vppp.db`   | SQLite database path           |
| `--web`          | `./web`           | Web panel files directory      |
| `--wg-iface`     | `wg0`             | WireGuard interface name       |
| `--wg-port`      | `51820`           | WireGuard listen port          |
| `--wg-address`   | `10.0.0.1/24`     | Server VPN address             |
| `--wg-dns`       | `1.1.1.1, 8.8.8.8`| DNS servers for clients       |
| `--wg-endpoint`  | —                 | Public endpoint (or use env)   |

## Production Checklist

- [ ] Set up HTTPS with a reverse proxy (Caddy recommended — auto TLS):
  ```
  your-domain.com {
      reverse_proxy localhost:8080
  }
  ```
- [ ] Restrict web panel access (change port 8080 to localhost-only, access via proxy)
- [ ] Back up `/data/vppp.db` regularly
- [ ] Monitor with `docker compose logs -f`

## License

MIT
