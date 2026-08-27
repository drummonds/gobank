# Hetzner demo server — Postgres-backed performance testing

Provisions a throwaway Hetzner Cloud server (project "Test") running the
Model Bank demo against **real PostgreSQL** on the same box, at a chosen
machine scale, for higher-performance simulation runs than the in-memory
WASM/pglike demo allows. The server bills by the hour and is deleted — not
just powered off — to stop billing.

## Usage

All commands need `HCLOUD_TOKEN`, so run them through `tp secrets`:

```sh
tp secrets task cloud:up                # small (cx23: 2 vCPU / 4 GB)
tp secrets task cloud:up SCALE=medium   # cx33:  4 vCPU /  8 GB
tp secrets task cloud:up SCALE=large    # cx53: 16 vCPU / 32 GB
tp secrets task cloud:up SCALE=xl       # ccx33: 8 dedicated vCPU / 32 GB
tp secrets task cloud:up SCALE=cax31    # ...or any: hcloud server-type list
tp secrets task cloud:status            # URL + serving state
tp secrets task cloud:ssh               # root shell on the box
tp secrets task cloud:down              # DELETE server + db, stop billing
```

`cloud:up` prints the demo URL (`http://<ip>:1347/`) when the service
answers. Re-running `cloud:up` against a live server only rebuilds and
redeploys the binary; to change scale, `cloud:down` first. Set
`GOBANK_SERVER_NAME=gobank-demo-2` to run several boxes side by side.

## What up.sh does

1. Cross-compiles `cmd/demo` for linux (arm64 for `cax*` types) with
   `CGO_ENABLED=0`, so the local charts-fork replace directive still works.
2. Creates a firewall (`22`, `1347`, ICMP only) and the server with
   cloud-init ([cloud-init.yaml](cloud-init.yaml)), which:
   - installs PostgreSQL from apt and creates the `gobank` role + database
     with a password generated on the box (never leaves the server);
   - sizes PostgreSQL to the machine (25% RAM `shared_buffers`, 50%
     `effective_cache_size`), so bigger `SCALE` also means a bigger DB;
   - writes the `gobank-demo` systemd unit with
     `GOBANK_PG_DSN=postgres://gobank:...@127.0.0.1/gobank` and
     `GOBANK_ADDR=:1347`.
3. Waits for ssh + cloud-init, installs the binary to `/opt/gobank/demo`,
   starts the service, and curl-checks the URL.

PostgreSQL only listens on localhost; the only exposure is the demo HTTP
port itself, which serves synthetic data (real PII stays encrypted at rest
via the demo's PII store). SSH uses the keys already registered in the
Hetzner project, with host keys pinned in `build/known_hosts` (gitignored,
safe to delete — servers come and go, so `~/.ssh/known_hosts` is avoided).

## Requirements

- `hcloud` CLI: `go install github.com/hetznercloud/cli/cmd/hcloud@latest`
- An SSH key registered in the Hetzner project (up.sh checks and explains).
- `tp unlock` session for the Bitwarden-held `HCLOUD_TOKEN`.
