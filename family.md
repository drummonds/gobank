# The family

The Model Bank is built from a family of our own modules. This map is
read from the demo's build (`go mod graph` and `go list -m all` in
`cmd/demo`, by `cmd/gendocs`), so it is the build's, not a drawing: one
node per repository with the version the build selected, the root to
the left, the leaves to the right, and one edge per requirement a
module's `go.mod` makes of another.

![The family](family.svg)

A dashed amber edge is a lag: the dependent's `go.mod` asks for an
older version than the build selects, because another module asked for
the newer one and Go's minimal version selection took the highest.
The code still runs against the newer version; the lag says which
`go.mod` to bring up to date, and the diagram is read right to left as
the release order that clears them.

Third-party modules are not drawn. The dashboard's
[dependency page](https://dashboard.docs.bytestone.uk/deps-gobank.html)
shows the same family across every repository from their `go.mod`
files; this page shows what one build of the bank actually selected.
