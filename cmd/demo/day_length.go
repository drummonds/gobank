package main

import (
	"git.bytestone.uk/hum3/gobank/internal/daylength"
	"os"
	"time"
)

// dayLengthFromEnv is the configured day length (GOBANK_DAY_LENGTH), or an
// error the caller should refuse to start on.
func dayLengthFromEnv() (time.Duration, error) {
	return daylength.Parse(os.Getenv("GOBANK_DAY_LENGTH"))
}
