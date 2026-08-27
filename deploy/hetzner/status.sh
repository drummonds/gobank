#!/usr/bin/env bash
# Show whether the Hetzner demo server exists and where it is serving.
#   tp secrets task cloud:status
set -euo pipefail
cd "$(dirname "$0")"

NAME=${GOBANK_SERVER_NAME:-gobank-demo}
: "${HCLOUD_TOKEN:?HCLOUD_TOKEN not set — run via: tp secrets task cloud:status}"

if ! hcloud server describe "$NAME" >/dev/null 2>&1; then
	echo "server $NAME: not provisioned (no billing)"
	exit 0
fi

hcloud server list -o columns=name,type,status,ipv4,datacenter | sed -n "1p;/^$NAME /p"
IP=$(hcloud server ip "$NAME")
echo
echo "demo:  http://$IP:1347/"
if curl -fsS -o /dev/null -m 5 "http://$IP:1347/" 2>/dev/null; then
	echo "state: serving"
else
	echo "state: NOT answering (booting? check: task cloud:ssh, journalctl -u gobank-demo)"
fi
echo "ssh:   task cloud:ssh   (or: ssh root@$IP)"
echo "note:  server bills until deleted — task cloud:down"
