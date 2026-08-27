#!/usr/bin/env bash
# Delete the Hetzner demo server (and its firewall). Deletion is what stops
# billing — Hetzner charges for powered-off servers. All server-side data
# (the PostgreSQL database) is destroyed; export a .goluca from the demo UI
# first if you want to keep the ledger.
#
#   tp secrets task cloud:down          # asks for confirmation
#   tp secrets task cloud:down -- -y    # no prompt
set -euo pipefail
cd "$(dirname "$0")"

NAME=${GOBANK_SERVER_NAME:-gobank-demo}
: "${HCLOUD_TOKEN:?HCLOUD_TOKEN not set — run via: tp secrets task cloud:down}"

if ! hcloud server describe "$NAME" >/dev/null 2>&1; then
	echo "server $NAME does not exist"
else
	IP=$(hcloud server ip "$NAME")
	if [ "${1:-}" != "-y" ]; then
		read -r -p "Delete server $NAME ($IP) and its database? [y/N] " ans
		case "$ans" in y | Y) ;; *)
			echo "aborted"
			exit 1
			;;
		esac
	fi
	hcloud server delete "$NAME"
fi

if hcloud firewall describe "$NAME" >/dev/null 2>&1; then
	hcloud firewall delete "$NAME"
fi
echo "done — nothing left billing"
