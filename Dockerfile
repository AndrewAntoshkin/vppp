FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /vppp-server ./cmd/server
RUN CGO_ENABLED=0 go build -o /vpn-cli ./cmd/cli

FROM alpine:3.20

RUN apk add --no-cache wireguard-tools iptables ip6tables

COPY --from=builder /vppp-server /usr/local/bin/vppp-server
COPY --from=builder /vpn-cli /usr/local/bin/vpn-cli
COPY entrypoint.sh /entrypoint.sh
COPY web/ /app/web/

RUN chmod +x /usr/local/bin/vppp-server /usr/local/bin/vpn-cli /entrypoint.sh

VOLUME ["/data", "/etc/wireguard"]

EXPOSE 443/udp 8080/tcp

ENTRYPOINT ["/entrypoint.sh"]
CMD ["--db", "/data/vppp.db", "--web", "/app/web"]
