package main

import (
	"fmt"
	"math/rand"
	"time"
)

var firstNames = []string{
	"Amelia", "Benjamin", "Charlotte", "Daniel", "Eleanor",
	"Freddie", "Georgina", "Harry", "Imogen", "James",
	"Katherine", "Liam", "Maisie", "Noah", "Olivia",
	"Patrick", "Quinn", "Rosie", "Samuel", "Tabitha",
	"Ursula", "Victor", "Wendy", "Xavier", "Yasmin", "Zara",
}

var lastNames = []string{
	"Adams", "Brown", "Clarke", "Davies", "Evans",
	"Foster", "Green", "Hughes", "Iqbal", "Jones",
	"Khan", "Lewis", "Morgan", "Patel",
}

var niPrefixes = []string{
	"AB", "CD", "EF", "GH", "JK", "LM", "NP", "RS", "TW", "YZ",
	"AE", "BG", "CH", "DL", "EM", "FK", "GP", "HN", "JR", "KS",
}

// generateCustomer creates a new random customer with accounts.
func generateCustomer(rng *rand.Rand, seq int, products []Product, openDate time.Time) (CustomerRecord, string, string) {
	first := firstNames[rng.Intn(len(firstNames))]
	last := lastNames[rng.Intn(len(lastNames))]
	name := first + " " + last
	id := fmt.Sprintf("cust-%03d", seq)
	ni := fmt.Sprintf("%s%06dC", niPrefixes[rng.Intn(len(niPrefixes))], 100000+rng.Intn(900000))

	numAccounts := 1 + rng.Intn(3)
	perm := rng.Perm(len(products))
	if numAccounts > len(products) {
		numAccounts = len(products)
	}

	accounts := make([]CustomerAccount, numAccounts)
	for j := 0; j < numAccounts; j++ {
		p := products[perm[j]]
		accounts[j] = CustomerAccount{
			ProductID:   p.ID,
			ProductName: p.Name,
			Family:      p.Family,
			Balance:     0,
			Rate:        p.Rate,
			OpenDate:    openDate,
		}
	}

	cust := CustomerRecord{
		ID:       id,
		Accounts: accounts,
	}
	return cust, name, ni
}

// averageRate calculates the average rate for products of a given family.
func averageRate(products []Product, family ProductFamily) float64 {
	sum := 0.0
	count := 0
	for _, p := range products {
		if p.Family == family {
			sum += p.Rate
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
