# Issue draft — hum3/gotreesitter

**Title:** README quick-start points at github.com/odvcencio fork, not this module; grammar count stale

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

1. README.md:6 says `go get github.com/odvcencio/gotreesitter` and the import
   examples (README.md:27-28) use the same path — but the module is
   `codeberg.org/hum3/gotreesitter` (go.mod; the README's own Links table has
   the right URL). Anyone following the quick-start installs an unrelated
   fork. Replace all `github.com/odvcencio/…` references with
   `codeberg.org/hum3/gotreesitter`.

2. README claims "206 grammars ship in the registry"; there are 210
   `*_register.go` files and 210 `grammar_blobs/*.bin`. Consider generating
   the count in docs from the registry to stop it drifting.

3. go.mod declares `go 1.24.0` while the rest of the family is on 1.26.0 —
   fine if intentional (wider compatibility), just flagging the outlier.
