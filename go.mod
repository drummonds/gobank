module github.com/drummonds/gobank

go 1.26.0

replace github.com/drummonds/go-luca => ../go-luca

replace codeberg.org/hum3/gobank-products => ../gobank-products

replace github.com/ncruces/go-sqlite3 v0.32.0 => github.com/ncruces/go-sqlite3 v0.30.6-0.20260318175627-361fdc52faa5

require codeberg.org/hum3/gobank-products v0.0.0

require (
	github.com/drummonds/go-luca v0.2.22 // indirect
	github.com/drummonds/go-postgres v0.5.0 // indirect
	github.com/drummonds/gotreesitter v0.6.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/ncruces/go-sqlite3 v0.32.0 // indirect
	github.com/ncruces/go-sqlite3-wasm v1.0.1-0.20260318174050-59cb2401d3ff // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)
