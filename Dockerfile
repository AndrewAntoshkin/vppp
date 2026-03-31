FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /vppp-server ./cmd/server
RUN CGO_ENABLED=1 go build -o /vpn-cli ./cmd/cli

FROM alpine:3.20

RUN apk add --no-cache wireguard-tools iptables ip6tables

COPY --from=builder /vppp-server /usr/local/bin/vppp-server
COPY --from=builder /vpn-cli /usr/local/bin/vpn-cli
COPY web/ /app/web/

VOLUME ["/data", "/etc/wireguard"]

EXPOSE 51820/udp 8080/tcp

ENTRYPOINT ["/usr/local/bin/vppp-server"]
CMD ["--db", "/data/vppp.db", "--web", "/app/web"]
