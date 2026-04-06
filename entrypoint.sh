#!/bin/sh
set -e

if [ -f /etc/wireguard/wg0.conf ]; then
    wg-quick up wg0 || true
fi

exec /usr/local/bin/vppp-server "$@"
