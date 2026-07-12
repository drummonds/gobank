# Issue draft — hum3/gobanks-customers

**Title:** NI hash is unsalted SHA-256 (brute-forceable, linkable); no key-length validation; README example key is 31 bytes

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

1. **Unsalted NI hash** (crypto.go:57-60, stored in the indexed plaintext
   `ni_hash` column, schema.go:26,30). UK NI numbers have a tiny keyspace, so
   the deterministic SHA-256 is brute-forceable offline, and equal NIs are
   linkable across rows — undermining the otherwise sound AES-GCM design.
   Fix: HMAC-SHA-256 keyed from the `KeyProvider` (keeps equality lookups
   working, removes offline brute force).

2. **No key-length validation** (key_provider.go): all providers return raw
   bytes; a wrong-sized key only fails at first encrypt/decrypt with a
   generic `aes.NewCipher` error. `EnvKeyProvider` takes the env var verbatim
   — a hex/base64-encoded 32-byte key silently becomes a 44/64-byte invalid
   key. Validate 16/24/32 bytes at construction (README claims AES-256, so
   arguably require exactly 32).

3. **README example key is 31 bytes** (README.md:57:
   `[]byte("32-byte-key-here!!!!!!!!!!!!!!!")`) — following the quick-start
   fails at runtime. (The test key in customer_test.go:24 is correctly 32.)
   README also omits `EnvKeyProvider` (present in code + CHANGELOG).

4. Minor: `RotateKeys` re-encrypts row-by-row outside a transaction
   (sql_store.go:359-363) — a mid-run failure leaves a mixed-key table
   (recoverable via `key_version`, but not atomic, unlike `Create` which uses
   a tx).

5. Naming: `gobanks-customers` (plural) vs every sibling `gobank-*` is never
   explained — one README line ("why the plural") would stop people and
   tooling from "correcting" it.

Positives: AES-GCM with fresh 12-byte crypto/rand nonce per encryption,
authenticated decrypts with proper error handling, fully parameterized SQL,
no PII in error strings.
