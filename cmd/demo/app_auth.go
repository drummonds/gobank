package main

import (
	"context"
	"crypto/subtle"

	"git.bytestone.uk/hum3/gobank/core"
)

// appLogin is the customer app's login (core.Authenticator): the one app
// password every customer logs in with (GOBANK_APP_PASSWORD), for a
// customer the bank knows. Empty means app login is off. Generated
// customers have no credentials of their own until the identity component
// arrives; the password is the app's business, not the bank's, so it
// lives here in the wiring.
type appLogin struct {
	bank     core.CustomerQueries
	password string
}

func newAppLogin(bank core.CustomerQueries, password string) appLogin {
	return appLogin{bank: bank, password: password}
}

// Authenticate implements core.Authenticator. Unknown customers and wrong
// passwords fail alike.
func (a appLogin) Authenticate(ctx context.Context, customerID, password string) (core.Customer, error) {
	ok := a.password != "" && subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
	cust, err := a.bank.Customer(ctx, customerID)
	if !ok || err != nil {
		return core.Customer{}, core.ErrBadCredentials
	}
	return cust, nil
}
