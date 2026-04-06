package wireguard

import (
	"bytes"
	"fmt"
	"text/template"
)

const clientConfigTemplate = `[Interface]
PrivateKey = {{ .PrivateKey }}
Address = {{ .Address }}
DNS = {{ .DNS }}
MTU = 1280

[Peer]
PublicKey = {{ .ServerPublicKey }}
PresharedKey = {{ .PresharedKey }}
Endpoint = {{ .Endpoint }}:{{ .Port }}
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`

const serverConfigTemplate = `[Interface]
PrivateKey = {{ .PrivateKey }}
Address = {{ .Address }}
ListenPort = {{ .Port }}
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
{{ range .Peers }}
[Peer]
PublicKey = {{ .PublicKey }}
PresharedKey = {{ .PresharedKey }}
AllowedIPs = {{ .AllowedIPs }}
{{ end }}`

type ClientConfig struct {
	PrivateKey      string
	Address         string
	DNS             string
	ServerPublicKey string
	PresharedKey    string
	Endpoint        string
	Port            int
}

type ServerConfig struct {
	PrivateKey string
	Address    string
	Port       int
	Peers      []ServerPeerConfig
}

type ServerPeerConfig struct {
	PublicKey    string
	PresharedKey string
	AllowedIPs  string
}

func GenerateClientConfig(cfg ClientConfig) (string, error) {
	tmpl, err := template.New("client").Parse(clientConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("parse client template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("execute client template: %w", err)
	}

	return buf.String(), nil
}

func GenerateServerConfig(cfg ServerConfig) (string, error) {
	tmpl, err := template.New("server").Parse(serverConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("parse server template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("execute server template: %w", err)
	}

	return buf.String(), nil
}
