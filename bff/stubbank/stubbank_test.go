package stubbank_test

import (
	"testing"

	"git.bytestone.uk/hum3/gobank/bff/stubbank"
	"git.bytestone.uk/hum3/gobank/core/coretest"
)

// The stub honours the same contract as the real core.
func TestContract(t *testing.T) {
	b := stubbank.New()
	coretest.Run(t, coretest.Fixture{Queries: b, Auth: b, CustomerID: "cust-001", Password: "password"})
}
