module github.com/drummonds/gobank/cmd/demo

go 1.26.0

require (
	codeberg.org/hum3/gobank-products v0.0.0
	github.com/drummonds/go-luca v0.2.22
	github.com/drummonds/go-postgres v0.5.0
	github.com/drummonds/gobanks-customers v0.0.0
	github.com/drummonds/lofigui v0.17.5
	github.com/flosch/pongo2/v6 v6.0.0
	github.com/go-analyze/charts v0.5.25
	github.com/jackc/pgx/v5 v5.8.0
)

require (
	github.com/drummonds/gotreesitter v0.6.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-analyze/bulk v0.1.3 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/ncruces/go-sqlite3 v0.32.0 // indirect
	github.com/ncruces/go-sqlite3-wasm v1.0.1-0.20260318174050-59cb2401d3ff // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	golang.org/x/image v0.24.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace github.com/drummonds/go-luca => ../../../go-luca

replace github.com/drummonds/lofigui => ../../../../minor/lofigui

replace codeberg.org/hum3/gobank-products => ../../../gobank-products

replace github.com/drummonds/gobanks-customers => ../../../gobanks-customers

replace github.com/ncruces/go-sqlite3 v0.32.0 => github.com/ncruces/go-sqlite3 v0.30.6-0.20260318175627-361fdc52faa5
