// Command bff runs the Model Bank backend-for-frontend.
//
// Until the banking core is extracted from cmd/demo it serves the in-memory
// stub bank (bff/stubbank), which is enough to develop the Flutter shell and
// the web rendering against.
//
//	go run ./cmd/bff                      # serve on :8090
//	go run ./cmd/bff -journeys out.d2     # write the customer-journey diagram and exit
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.bytestone.uk/hum3/gobank/bff"
	"git.bytestone.uk/hum3/gobank/bff/stubbank"
)

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	secure := flag.Bool("secure-cookies", false, "mark the session cookie Secure (set when served over HTTPS)")
	journeys := flag.String("journeys", "", "write the customer-journey d2 diagram to this file and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	bank := stubbank.New()

	if *journeys != "" {
		j, err := bff.Journeys(context.Background(), bank, "cust-001")
		if err != nil {
			log.Error("journeys", "err", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*journeys, []byte(j.D2()), 0o644); err != nil {
			log.Error("journeys", "err", err)
			os.Exit(1)
		}
		fmt.Println("wrote", *journeys)
		return
	}

	srv := bff.NewServer(bff.Config{Bank: bank, Auth: bank, SecureCookies: *secure, Logger: log})
	go func() {
		for range time.Tick(time.Minute) {
			srv.Sessions().Sweep()
		}
	}()

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdown)
	}()
	log.Info("bff.listen", "addr", *addr, "bank", bank.String(), "secure_cookies", *secure)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("bff.serve", "err", err)
		os.Exit(1)
	}
}
