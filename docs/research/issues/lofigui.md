# Issue draft — drummonds/lofigui (GitHub)

**Title:** Global-buffer concurrency semantics under-documented; README badge/claims drift

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

1. **Global buffer semantics**: `defaultContext` (lofigui.go:39) is locked
   per-call, but a whole render (`Reset → writes → Buffer`) is not atomic —
   concurrent writers interleave output. This is documented in the guides
   (GO_CONTROLLER_GUIDE.md:114,134) but not in the godoc/package comment,
   which is where API consumers look. Also, the package-level
   `Print/HTML/...` always target `defaultContext` even when a Controller is
   configured with a custom `Context` (controller.go:75,124) — the custom
   context is silently bypassed unless the `*Context` methods are used.
   Suggest: state both in the package godoc, and/or route package-level
   functions through the active controller's context.

2. README badge says `go-1.21+` (README.md:11) but go.mod declares `go 1.22`
   and the ServeMux patterns require 1.22 (wasm.go:88 comment).

3. README "No JavaScript: Pure HTML/CSS" (README.md:27) is true for the
   server build, but the `RunWASM` path exports and requires JS polling hooks
   (`goStart`/`goRender`, wasm.go:21-40) — worth a qualifier.

4. FYI: gobank's docs linked `https://h3-lofigui.statichost.page/research.html`,
   which 404s (the file is `RESEARCH.html`; the chart comparison is
   `research-charts.html`). Fixed on the gobank side; a lowercase
   `research.html` redirect/copy here would make the URL forgiving.
