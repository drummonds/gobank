package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// The population generator (ADR-0002 stage 4): who joins the bank and when.
// It is simulation, not bank: it raises OpenCustomer on the bank and reads
// the bank through its staff queries, and touches nothing of the bank's
// own (TestGeneratorsReachTheBankOnlyThroughCommands).

// coreBank is the core as the generators see it: commands to raise events,
// staff queries to decide them.
type coreBank interface {
	core.Commands
	core.StaffQueries
}

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

// generateCustomer is a random customer joining on day: identity, a KYC
// standing, and one to three accounts across the catalogue, each with its
// opening money — a deposit of £500 to £9,999, or a loan asked of £1,000
// to £49,999 that the bank lends within its headroom.
func generateCustomer(rng *rand.Rand, catalogue []core.Product, day time.Time) core.NewCustomer {
	first := firstNames[rng.Intn(len(firstNames))]
	last := lastNames[rng.Intn(len(lastNames))]
	ni := fmt.Sprintf("%s%06dC", niPrefixes[rng.Intn(len(niPrefixes))], 100000+rng.Intn(900000))

	// DOB: 18-70 years before the day they join
	ageYears := 18 + rng.Intn(53)
	ageDays := rng.Intn(365)
	dob := day.AddDate(-ageYears, 0, -ageDays)

	streetNum := 1 + rng.Intn(150)
	street := streetNames[rng.Intn(len(streetNames))]
	city := cities[rng.Intn(len(cities))]
	postcodeArea := postcodeAreas[rng.Intn(len(postcodeAreas))]
	postcode := fmt.Sprintf("%s %d%c%c", postcodeArea, rng.Intn(10), 'A'+rune(rng.Intn(26)), 'A'+rune(rng.Intn(26)))
	address := fmt.Sprintf("%d %s, %s, %s", streetNum, street, city, postcode)
	email := fmt.Sprintf("%s.%s@example.com", strings.ToLower(first), strings.ToLower(last))
	phone := fmt.Sprintf("07%03d %06d", rng.Intn(1000), rng.Intn(1000000))

	riskRoll := rng.Float64()
	riskRating := "Low"
	if riskRoll > 0.95 {
		riskRating = "Medium"
	} else if riskRoll > 0.70 {
		riskRating = "Standard"
	}

	numAccounts := min(1+rng.Intn(3), len(catalogue))
	perm := rng.Perm(len(catalogue))
	accounts := make([]core.NewAccount, numAccounts)
	for j := range numAccounts {
		p := catalogue[perm[j]]
		var opening luca.Amount
		if p.Family == "Savings" {
			opening = luca.Amount(500+rng.Intn(9500)) * 100
		} else {
			opening = luca.Amount(1000+rng.Intn(49000)) * 100
		}
		accounts[j] = core.NewAccount{ProductID: p.ID, Opening: opening}
	}

	return core.NewCustomer{
		KYC:      core.KYC{Verified: true, LastCheck: day, RiskRating: riskRating},
		PII:      core.PII{Name: first + " " + last, NI: ni, DOB: dob.Format("2006-01-02"), Address: address, Email: email, Phone: phone},
		Accounts: accounts,
	}
}

// productCatalogue is the bank's catalogue as the generators draw from it,
// read once: the products do not change, their books do.
func (ds *DemoState) productCatalogue(ctx context.Context) []core.Product {
	ds.mu.Lock()
	catalogue := ds.catalogue
	ds.mu.Unlock()
	if catalogue != nil {
		return catalogue
	}
	catalogue, err := ds.bank.Products(ctx)
	if err != nil {
		log.Printf("generator: catalogue: %v", err)
		return nil
	}
	ds.mu.Lock()
	ds.catalogue = catalogue
	ds.mu.Unlock()
	return catalogue
}

// openGenerated generates one customer and opens them on the bank,
// reporting whether the bank took them.
func (ds *DemoState) openGenerated(ctx context.Context) bool {
	catalogue := ds.productCatalogue(ctx)
	pos, err := ds.bank.Position(ctx)
	if err != nil || len(catalogue) == 0 {
		return false
	}
	ds.mu.Lock()
	c := generateCustomer(ds.rng, catalogue, pos.Day)
	ds.mu.Unlock()
	if _, err := ds.bank.OpenCustomer(ctx, c); err != nil {
		log.Printf("generator: open customer: %v", err)
		return false
	}
	return true
}

// createCustomer generates and opens one customer. Must not be called with
// ds.mu held.
func (ds *DemoState) createCustomer() {
	ds.openGenerated(context.Background())
}

// rollNewCustomer is the day's chance of a new customer: the more
// attractive the bank's rates against the base rate, the likelier someone
// joins, up to the population cap.
func (ds *DemoState) rollNewCustomer(ctx context.Context) {
	pos, err := ds.bank.Position(ctx)
	if err != nil || pos.Customers >= ds.settings.Get().MaxCustomers {
		return
	}
	catalogue := ds.productCatalogue(ctx)
	avgSavings := averageRate(catalogue, "Savings")
	avgLending := averageRate(catalogue, "Lending")

	savingsAttract, lendingAttract := 0.0, 0.0
	if pos.BoERate > 0 {
		savingsAttract = (avgSavings - pos.BoERate) / pos.BoERate
		lendingAttract = (pos.BoERate - avgLending) / pos.BoERate
	}
	attractiveness := clamp((savingsAttract+lendingAttract)/2, 0, 1)
	dailyProb := 0.10 + attractiveness*0.20

	ds.mu.Lock()
	roll := ds.rng.Float64()
	ds.mu.Unlock()
	if roll < dailyProb {
		ds.openGenerated(ctx)
	}
}

// AddCustomersBatch starts adding n customers in the background, with one
// worker per database writer. Each worker holds ds.mu only to generate a
// customer, so other operations proceed while the bank opens them.
func (ds *DemoState) AddCustomersBatch(n int) {
	ds.mu.Lock()
	if ds.addingCustRunning {
		ds.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ds.addingCustRunning = true
	ds.addingCustCancel = cancel
	ds.addingCustProgress = 0
	ds.addingCustTarget = n
	ds.addingCustStart = ds.now()
	workers := ds.dbWriters()
	ds.mu.Unlock()

	claimed := 0 // customers this batch has set out to open (mu)
	go func() {
		defer cancel()
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				for ctx.Err() == nil {
					pos, err := ds.bank.Position(ctx)
					ds.mu.Lock()
					if err != nil || claimed >= n || pos.Customers >= ds.settings.Get().MaxCustomers {
						ds.mu.Unlock()
						return
					}
					claimed++
					ds.mu.Unlock()

					opened := ds.openGenerated(ctx)

					ds.mu.Lock()
					if opened && ctx.Err() == nil {
						ds.addingCustProgress++
					}
					ds.mu.Unlock()
					yieldToBrowser()
				}
			})
		}
		wg.Wait()
		ds.mu.Lock()
		if ctx.Err() == nil { // a Reset has already finished a cancelled batch
			ds.finishAddingLocked()
		}
		ds.mu.Unlock()
	}()
}

// finishAddingLocked ends a batch add, keeping its customers/s for the
// dashboard. Must be called with ds.mu held.
func (ds *DemoState) finishAddingLocked() {
	if ds.addingCustProgress > 0 {
		ds.lastAddRate = perSecond(ds.addingCustProgress, ds.now().Sub(ds.addingCustStart))
	}
	ds.addingCustRunning = false
	ds.addingCustCancel = nil
	ds.addingCustProgress = 0
	ds.addingCustTarget = 0
}

// IsAddingCustomers returns true if a batch add is in progress.
func (ds *DemoState) IsAddingCustomers() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.addingCustRunning
}

// averageRate is the mean rate of the catalogue's products of one family.
func averageRate(catalogue []core.Product, family string) float64 {
	sum, count := 0.0, 0
	for _, p := range catalogue {
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
	return min(max(v, lo), hi)
}
