package main

import (
	"os"
	"time"

	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
)

// dayLengthFromEnv is the configured day length (GOBANK_DAY_LENGTH), or an
// error the caller should refuse to start on.
func dayLengthFromEnv() (time.Duration, error) {
	return sim.ParseDayLength(os.Getenv("GOBANK_DAY_LENGTH"))
}
