package main

import (
	"database/sql"
	"log"

	_ "github.com/drummonds/go-postgres"
)

// initDB opens an in-memory pglike database and creates the gilt tables.
func (ds *DemoState) initDB() {
	if ds.db != nil {
		ds.db.Close()
	}
	db, err := sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	if err != nil {
		log.Printf("initDB: open failed: %v", err)
		return
	}
	ds.db = db

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS gilt_yields (
		tenor VARCHAR(10) PRIMARY KEY,
		rate REAL NOT NULL,
		effective_date TIMESTAMP DEFAULT NOW()
	)`)
	if err != nil {
		log.Printf("initDB: create gilt_yields: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS gilt_holdings (
		id SERIAL PRIMARY KEY,
		tenor VARCHAR(10) NOT NULL,
		face_value REAL NOT NULL,
		purchase_date TIMESTAMP NOT NULL,
		yield REAL NOT NULL
	)`)
	if err != nil {
		log.Printf("initDB: create gilt_holdings: %v", err)
	}

	// Seed gilt yields
	yields := []struct {
		tenor string
		rate  float64
	}{
		{"1Y", 0.0435},
		{"2Y", 0.0410},
		{"5Y", 0.0395},
		{"10Y", 0.0405},
		{"30Y", 0.0445},
	}
	for _, y := range yields {
		_, err = db.Exec(`INSERT INTO gilt_yields (tenor, rate) VALUES ($1, $2)
			ON CONFLICT (tenor) DO UPDATE SET rate = EXCLUDED.rate`, y.tenor, y.rate)
		if err != nil {
			log.Printf("initDB: seed %s: %v", y.tenor, err)
		}
	}
}

// DB returns the pglike database handle.
func (ds *DemoState) DB() *sql.DB {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.db
}
