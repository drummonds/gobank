//go:build js && wasm

package main

// demoPassword is the app password in the tab, where the whole bank is
// one person's: the login screen states it.
const demoPassword = "demo"

func appPassword() string { return demoPassword }

func loginNote() string {
	return "Model bank in your tab: the password is " + demoPassword + ". Customer IDs are listed on the Customers page."
}

// adminPassword is the first admin's password in the tab, the same one.
func adminPassword() string { return demoPassword }

func staffLoginNote() string {
	return "Model bank in your tab: log in as admin, the password is " + demoPassword + "."
}
