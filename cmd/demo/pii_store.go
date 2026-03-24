package main

// PIIInput is the input struct for generating customer PII.
// Passed to the gobank-customers store for encryption and persistence.
type PIIInput struct {
	Name    string
	NI      string
	DOB     string
	Address string
	Email   string
	Phone   string
}

// PIIData is the output struct for displaying decrypted PII.
type PIIData struct {
	Name    string
	NI      string
	DOB     string
	Address string
	Email   string
	Phone   string
}
