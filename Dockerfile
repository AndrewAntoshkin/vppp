FROM alpine:3.20

RUN apk add --no-cache wireguard-tools iptables ip6tables

COPY vppp-server-linux /usr/local/bin/vppp-server
COPY vpn-cli-linux /usr/local/bin/vpn-cli
COPY entrypoint.sh /entrypoint.sh
COPY web/ /app/web/

RUN chmod +x /usr/local/bin/vppp-server /usr/local/bin/vpn-cli /entrypoint.sh

VOLUME ["/data", "/etc/wireguard"]

EXPOSE 443/udp 8080/tcp

ENTRYPOINT ["/entrypoint.sh"]
CMD ["--db", "/data/vppp.db", "--web", "/app/web"]
