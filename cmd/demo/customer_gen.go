package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
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

var streetNames = []string{
	"High Street", "Station Road", "Church Lane", "Mill Lane", "Park Avenue",
	"Victoria Road", "Kings Road", "Queens Road", "London Road", "Manor Drive",
	"Elm Close", "Oak Lane", "Willow Way", "Cedar Avenue", "Birch Road",
}

var cities = []string{
	"London", "Manchester", "Birmingham", "Leeds", "Bristol",
	"Sheffield", "Liverpool", "Newcastle", "Nottingham", "Edinburgh",
	"Glasgow", "Cardiff", "Oxford", "Cambridge", "Bath",
}

var postcodeAreas = []string{
	"SW1", "EC2", "W1", "SE1", "N1", "E1", "NW1",
	"M1", "B1", "LS1", "BS1", "S1", "L1", "NE1", "NG1",
}

const bankSortCode = "30-90-01"

// generateCustomer creates a new random customer with accounts and PII.
func generateCustomer(rng *rand.Rand, seq int, products []Product, openDate time.Time) (CustomerRecord, PIIInput) {
	first := firstNames[rng.Intn(len(firstNames))]
	last := lastNames[rng.Intn(len(lastNames))]
	name := first + " " + last
	id := fmt.Sprintf("cust-%03d", seq)
	ni := fmt.Sprintf("%s%06dC", niPrefixes[rng.Intn(len(niPrefixes))], 100000+rng.Intn(900000))

	// DOB: 18-70 years before openDate
	ageYears := 18 + rng.Intn(53)
	ageDays := rng.Intn(365)
	dob := openDate.AddDate(-ageYears, 0, -ageDays)

	// Address: street number + name + city + postcode
	streetNum := 1 + rng.Intn(150)
	street := streetNames[rng.Intn(len(streetNames))]
	city := cities[rng.Intn(len(cities))]
	postcodeArea := postcodeAreas[rng.Intn(len(postcodeAreas))]
	postcode := fmt.Sprintf("%s %d%c%c", postcodeArea, rng.Intn(10), 'A'+rune(rng.Intn(26)), 'A'+rune(rng.Intn(26)))
	address := fmt.Sprintf("%d %s, %s, %s", streetNum, street, city, postcode)

	// Email
	email := fmt.Sprintf("%s.%s@example.com", strings.ToLower(first), strings.ToLower(last))

	// Phone: 07XXX XXXXXX
	phone := fmt.Sprintf("07%03d %06d", rng.Intn(1000), rng.Intn(1000000))

	// KYC: weighted random risk rating
	riskRoll := rng.Float64()
	riskRating := "Low"
	if riskRoll > 0.95 {
		riskRating = "Medium"
	} else if riskRoll > 0.70 {
		riskRating = "Standard"
	}

	numAccounts := 1 + rng.Intn(3)
	perm := rng.Perm(len(products))
	if numAccounts > len(products) {
		numAccounts = len(products)
	}

	accounts := make([]CustomerAccount, numAccounts)
	for j := 0; j < numAccounts; j++ {
		p := products[perm[j]]
		acctNum := fmt.Sprintf("%08d", rng.Intn(100000000))
		accounts[j] = CustomerAccount{
			ProductID:   p.ID,
			ProductName: p.Name,
			Family:      p.Family,
			Balance:     0,
			Rate:        p.Rate,
			OpenDate:    openDate,
			SortCode:    bankSortCode,
			AccountNum:  acctNum,
		}
	}

	cust := CustomerRecord{
		ID:       id,
		Accounts: accounts,
		JoinDate: openDate,
		KYCStatus: KYCStatus{
			Verified:      true,
			LastCheckDate: openDate,
			RiskRating:    riskRating,
		},
	}
	pii := PIIInput{
		Name:    name,
		NI:      ni,
		DOB:     dob.Format("2006-01-02"),
		Address: address,
		Email:   email,
		Phone:   phone,
	}
	return cust, pii
}

// averageRate calculates the average rate for products of a given family.
func averageRate(products []Product, family gbp.ProductFamily) float64 {
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
