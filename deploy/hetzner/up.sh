#!/usr/bin/env bash
# Provision (or redeploy to) a Hetzner Cloud server running the Model Bank
# demo backed by real PostgreSQL, for performance testing at chosen scale.
#
#   tp secrets task cloud:up                 # small (cx22)
#   tp secrets task cloud:up SCALE=large     # or: small|medium|large|xl
#   tp secrets task cloud:up SCALE=ccx43     # or any hcloud server type
#
# Needs HCLOUD_TOKEN in the environment — run via `tp secrets`.
# Re-running against an existing server just rebuilds and redeploys the
# binary (the server type is not changed; `task cloud:down` first to resize).
# Deleting the server (down.sh) is what stops billing — Hetzner charges for
# powered-off servers too.
set -euo pipefail
cd "$(dirname "$0")"

NAME=${GOBANK_SERVER_NAME:-gobank-demo}
LOCATION=${LOCATION:-fsn1}
IMAGE=ubuntu-24.04

SCALE=${1:-small}
case "$SCALE" in
small) TYPE=cx23 ;;   # 2 vCPU / 4 GB shared x86
medium) TYPE=cx33 ;;  # 4 vCPU / 8 GB shared x86
large) TYPE=cx53 ;;   # 16 vCPU / 32 GB shared x86
xl) TYPE=ccx33 ;;     # 8 dedicated vCPU / 32 GB
*) TYPE=$SCALE ;;     # any type from: hcloud server-type list
esac

case "$TYPE" in
cax*) GOARCH=arm64 ;; # cax = Ampere ARM
*) GOARCH=amd64 ;;
esac

: "${HCLOUD_TOKEN:?HCLOUD_TOKEN not set — run via: tp secrets task cloud:up}"
command -v hcloud >/dev/null || {
	echo "hcloud CLI not found. Install with:" >&2
	echo "  go install github.com/hetznercloud/cli/cmd/hcloud@latest" >&2
	exit 1
}

mkdir -p build
SSH_OPTS=(-o UserKnownHostsFile="$PWD/build/known_hosts" -o StrictHostKeyChecking=accept-new)

echo "== build demo binary (linux/$GOARCH)"
GIT_TAG=$(git describe --tags --always 2>/dev/null || echo dev)
(cd ../../cmd/demo && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 \
	go build -ldflags "-X main.version=$GIT_TAG" -o "$OLDPWD/build/demo" .)

CREATED=0
if hcloud server describe "$NAME" >/dev/null 2>&1; then
	echo "== server $NAME already exists — redeploying binary only"
else
	CREATED=1
	echo "== firewall $NAME"
	if ! hcloud firewall describe "$NAME" >/dev/null 2>&1; then
		hcloud firewall create --name "$NAME" >/dev/null
		hcloud firewall add-rule "$NAME" --description ssh --direction in --protocol tcp --port 22 --source-ips 0.0.0.0/0 --source-ips ::/0
		hcloud firewall add-rule "$NAME" --description demo --direction in --protocol tcp --port 1347 --source-ips 0.0.0.0/0 --source-ips ::/0
		hcloud firewall add-rule "$NAME" --description ping --direction in --protocol icmp --source-ips 0.0.0.0/0 --source-ips ::/0
	fi

	mapfile -t SSH_KEYS < <(hcloud ssh-key list -o noheader -o columns=name)
	[ ${#SSH_KEYS[@]} -gt 0 ] || {
		echo "No SSH keys in the Hetzner project — add one first:" >&2
		echo "  tp secrets hcloud ssh-key create --name <name> --public-key-from-file ~/.ssh/<key>.pub" >&2
		exit 1
	}
	KEY_ARGS=()
	for k in "${SSH_KEYS[@]}"; do KEY_ARGS+=(--ssh-key "$k"); done

	echo "== create server $NAME ($TYPE, $LOCATION)"
	hcloud server create --name "$NAME" --type "$TYPE" --image "$IMAGE" \
		--location "$LOCATION" --firewall "$NAME" "${KEY_ARGS[@]}" \
		--user-data-from-file cloud-init.yaml --label project=gobank
fi

IP=$(hcloud server ip "$NAME")

# Hetzner reuses IPs: a freshly created server has a new host key, so drop
# any key pinned for this IP by a previous incarnation — otherwise the ssh
# wait below fails on host-key verification forever.
if [ "$CREATED" = 1 ] && [ -f build/known_hosts ]; then
	ssh-keygen -f "$PWD/build/known_hosts" -R "$IP" >/dev/null 2>&1 || true
fi

echo "== wait for ssh ($IP)"
for _ in $(seq 1 60); do
	err=$(ssh "${SSH_OPTS[@]}" -o ConnectTimeout=5 "root@$IP" true 2>&1) && break
	if grep -q "Host key verification failed" <<<"$err"; then
		echo "$err" >&2
		echo "Pinned key is stale (server replaced?). Fix with:" >&2
		echo "  ssh-keygen -f '$PWD/build/known_hosts' -R '$IP'" >&2
		exit 1
	fi
	sleep 5
done
ssh "${SSH_OPTS[@]}" "root@$IP" true || {
	echo "could not reach root@$IP over ssh" >&2
	exit 1
}

echo "== wait for cloud-init (postgres install; first boot takes a minute or two)"
ssh "${SSH_OPTS[@]}" "root@$IP" cloud-init status --wait >/dev/null || true

echo "== install binary + start service"
scp "${SSH_OPTS[@]}" -q build/demo "root@$IP:/opt/gobank/demo.new"
ssh "${SSH_OPTS[@]}" "root@$IP" '
	set -e
	install -m 0755 -o gobank -g gobank /opt/gobank/demo.new /opt/gobank/demo
	rm /opt/gobank/demo.new
	systemctl daemon-reload
	systemctl enable gobank-demo >/dev/null 2>&1
	systemctl restart gobank-demo
'

echo "== check"
for _ in $(seq 1 12); do
	curl -fsS -o /dev/null "http://$IP:1347/" 2>/dev/null && break
	sleep 5
done
curl -fsS -o /dev/null "http://$IP:1347/" || {
	echo "service not answering yet — check: task cloud:ssh, then journalctl -u gobank-demo" >&2
	exit 1
}

echo
echo "Model Bank demo ($GIT_TAG) on $TYPE at http://$IP:1347/"
echo "Turn off (and stop billing) with: tp secrets task cloud:down"
