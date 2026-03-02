//go:build js && wasm

package main

import "database/sql"

// initDB is a no-op on WASM — pglike/sqlite not supported.
func (ds *DemoState) initDB() {}

// DB returns nil on WASM.
func (ds *DemoState) DB() *sql.DB { return nil }
