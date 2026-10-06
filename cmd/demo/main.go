//go:build !(js && wasm)

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// GOBANK_PG_DSN selects a real PostgreSQL backend (postgres://...);
	// unset means in-memory pglike, as before.
	state := NewDemoStateWithDSN(os.Getenv("GOBANK_PG_DSN"))
	// GOBANK_MEMORY_LIMIT sizes the auto-stop threshold to the box (the
	// deployer sets it); unset keeps the browser-sized default.
	limit, err := memoryLimitFromEnv()
	if err != nil {
		log.Fatalf("GOBANK_MEMORY_LIMIT: %v", err)
	}
	state.SetMemoryLimit(limit)
	// GOBANK_DAY_LENGTH slows the simulation to a wall-clock day length
	// (e.g. 2h); unset runs flat out. A day length set on the settings
	// page is the run's and outlives the process, so a resumed run keeps
	// its own.
	dayLength, err := dayLengthFromEnv()
	if err != nil {
		log.Fatalf("GOBANK_DAY_LENGTH: %v", err)
	}
	state.DefaultDayLength(dayLength)
	// A run that was going when the previous process stopped carries on.
	if state.ResumedRunning() {
		log.Printf("resume: the run was going; starting the loop")
		state.Start()
	}

	handler := newHandler(state, version, "/")

	// GOBANK_ADDR pins the listen address (e.g. ":1347" under systemd on a
	// cloud host) instead of scanning for a free port.
	if addr := os.Getenv("GOBANK_ADDR"); addr != "" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("listen %s: %v", addr, err)
		}
		log.Printf("Starting Model Bank Demo on %s", addr)
		serve(ln, handler, state)
		return
	}

	// Try ports starting from 1347, auto-increment if in use
	for port := 1347; port < 1357; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("Port %d in use, trying next...", port)
			continue
		}
		log.Printf("Starting Model Bank Demo on http://localhost%s", addr)
		serve(ln, handler, state)
		return
	}
	log.Fatal("Could not find an available port in range 1347-1356")
}

// serve runs the HTTP server on ln until SIGTERM or SIGINT, then stops the
// run loop, waits for the day in progress to finish writing and drains the
// server. This is the stop step of an in-place upgrade (ADR-0003); the
// service manager's stop timeout bounds it.
func serve(ln net.Listener, handler http.Handler, state *DemoState) {
	srv := &http.Server{Handler: handler}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	stop()
	log.Printf("shutdown: signal received; finishing the day in progress")
	if err := state.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown: run loop: %v", err)
	}
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		log.Printf("shutdown: server: %v", err)
	}
	state.RecordStop()
	log.Printf("shutdown: done")
}
