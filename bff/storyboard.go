package bff

import (
	"context"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/screen"
)

// The storyboard is the customer app as designed: every screen it has or
// is to have, built as a screen tree with sample data, so the wireframe
// (screen.Wireframe) and the journeys are drawn from the same trees the
// BFF serves. A screen is proposed until the BFF serves it in this form;
// the proposed builders here are the specification of the screens to come,
// and become the served screens when their story is built. New journeys
// (KYC onboarding, payees, statements) start here as proposed screens.

// Proposed screen paths.
const (
	PathScreenHome2     = "/v1/screen/home"     // the summary a logged-in customer lands on
	PathScreenPay       = "/v1/screen/pay"      // pay someone
	PathScreenPaySent   = "/v1/screen/pay/sent" // the payment made
	PathScreenMore      = "/v1/screen/more"     // profile, settings, log out
	PathScreenSignedOut = "/v1/screen/signed-out"
	PathScreenOpen      = "/v1/screen/open-account"
	PathScreenKYCDetail = "/v1/screen/onboarding/details"
	PathScreenKYCIdent  = "/v1/screen/onboarding/identity"
	PathScreenKYCWait   = "/v1/screen/onboarding/pending"
	PathPay             = "/v1/pay"
	PathKYCDetails      = "/v1/onboarding/details"
	PathKYCIdentity     = "/v1/onboarding/identity"
)

// Journey groups: the state a customer is in on a screen.
const (
	GroupLoggedOut  = "Logged out"
	GroupLoggedIn   = "Logged in"
	GroupOnboarding = "Onboarding"
)

// proposedNav is the tab bar of the designed app: Home is the summary.
func proposedNav(active string) *screen.Nav {
	return &screen.Nav{Active: active, Tabs: []screen.Tab{
		{Key: "home", Label: "Home", Icon: screen.IconHome, Screen: PathScreenHome2},
		{Key: "accounts", Label: "Accounts", Icon: screen.IconAccounts, Screen: PathScreenHome},
		{Key: "activity", Label: "Activity", Icon: screen.IconActivity, Screen: PathScreenTx},
		{Key: "more", Label: "More", Icon: screen.IconBank, Screen: PathScreenMore},
	}}
}

// HomeScreen is the summary a logged-in customer lands on: the position,
// what moved today, and the way to the accounts and to paying someone.
func HomeScreen(c core.Customer, accounts []core.Account, recent core.TransactionPage) screen.Screen {
	s := screen.New("home", PathScreenHome2, "Hello, "+c.Name)
	s.Subtitle = "Your money today"
	var savings, lending luca.Amount
	for _, a := range accounts {
		if a.Family == "Lending" {
			lending += a.Balance
		} else {
			savings += a.Balance
		}
	}
	s.Add(screen.Hero("Net Position", FormatMoney(savings-lending), screen.ToneSavings,
		screen.Pair{Label: "Savings", Value: FormatMoney(savings)},
		screen.Pair{Label: "Lending", Value: FormatMoney(lending)},
	))
	s.Add(screen.Row(screen.IconAccounts, "Accounts", fmt.Sprintf("%d open", len(accounts)), "", "", screen.ToneNeutral, screen.Go(PathScreenHome)))
	s.Add(screen.Row(screen.IconOut, "Pay someone", "To another Model Bank customer", "", "", screen.ToneNeutral, screen.Go(PathScreenPay)))
	s.Add(screen.Heading("Recent"))
	shown := core.TransactionPage{Entries: recent.Entries, Page: 1, PerPage: 3, Total: len(recent.Entries)}
	if len(shown.Entries) > 3 {
		shown.Entries = shown.Entries[:3]
	}
	addTransactions(&s, shown, true)
	s.Add(screen.Button("All activity", screen.ToneMuted, screen.Go(PathScreenTx)))
	s.Nav = proposedNav("home")
	return s
}

// PayScreen is paying another customer from a savings account.
func PayScreen(accounts []core.Account) screen.Screen {
	s := screen.New("pay", PathScreenPay, "Pay someone")
	s.Back = screen.Go(PathScreenHome2)
	from := "your savings account"
	for _, a := range accounts {
		if a.Family == "Savings" {
			from = a.ProductName + " · " + FormatMoney(a.Balance)
			break
		}
	}
	s.Add(
		screen.Text("From "+from, screen.ToneMuted),
		screen.Form("Pay", PathPay,
			screen.Field{Name: "to", Label: "To (customer ID)", Kind: "text", Placeholder: "e.g. cust-002", Required: true},
			screen.Field{Name: "amount", Label: "Amount", Kind: "number", Placeholder: "0.00", Required: true},
			screen.Field{Name: "reference", Label: "Reference", Kind: "text", Placeholder: "What it's for"},
		),
	)
	s.Nav = proposedNav("home")
	return s
}

// PaySentScreen confirms a payment made.
func PaySentScreen(p core.Payment) screen.Screen {
	s := screen.New("pay-sent", PathScreenPaySent, "Payment sent")
	s.Add(
		screen.Hero("Sent", FormatMoney(p.Amount), screen.TonePositive,
			screen.Pair{Label: "To", Value: p.To},
			screen.Pair{Label: "Reference", Value: p.Reference},
		),
		screen.Details("Status", screen.Pair{Label: "Status", Value: string(p.Status)}, screen.Pair{Label: "When", Value: p.CreatedAt.Format("2 Jan 2006 15:04")}),
		screen.Button("Done", screen.ToneSavings, screen.Go(PathScreenHome2)),
	)
	s.Nav = proposedNav("home")
	return s
}

// MoreScreen is the customer's own details and the way out.
func MoreScreen(c core.Customer) screen.Screen {
	s := screen.New("more", PathScreenMore, "More")
	s.Subtitle = c.Name
	s.Add(
		screen.Details("You", screen.Pair{Label: "Customer ID", Value: c.ID}, screen.Pair{Label: "Name", Value: c.Name}),
		screen.Button("Log out", screen.ToneMuted, screen.Logout()),
	)
	s.Nav = proposedNav("more")
	return s
}

// SignedOutScreen is where a log out or an expired session lands.
func SignedOutScreen(notice string) screen.Screen {
	s := screen.New("signed-out", PathScreenSignedOut, "Signed out")
	s.Subtitle = "Model Bank"
	if notice == "" {
		notice = "You have been signed out."
	}
	s.Add(
		screen.Text(notice, screen.ToneMuted),
		screen.Button("Log in again", screen.ToneSavings, screen.Go(PathScreenLogin)),
	)
	return s
}

// OpenAccountScreen is the front door for someone who is not a customer yet.
func OpenAccountScreen() screen.Screen {
	s := screen.New("open-account", PathScreenOpen, "Open an account")
	s.Subtitle = "Model Bank"
	s.Back = screen.Go(PathScreenLogin)
	s.Add(
		screen.Text("Takes about five minutes. You will need a photo ID.", screen.ToneMuted),
		screen.Button("Start", screen.ToneSavings, screen.Go(PathScreenKYCDetail)),
	)
	return s
}

// OnboardingDetailsScreen collects who the applicant is.
func OnboardingDetailsScreen() screen.Screen {
	s := screen.New("onboarding-details", PathScreenKYCDetail, "Your details")
	s.Subtitle = "Step 1 of 2"
	s.Back = screen.Go(PathScreenOpen)
	s.Add(screen.Form("Continue", PathKYCDetails,
		screen.Field{Name: "name", Label: "Full name", Kind: "text", Required: true},
		screen.Field{Name: "dob", Label: "Date of birth", Kind: "text", Placeholder: "YYYY-MM-DD", Required: true},
		screen.Field{Name: "address", Label: "Address", Kind: "text", Required: true},
		screen.Field{Name: "email", Label: "Email", Kind: "text", Required: true},
		screen.Field{Name: "phone", Label: "Mobile", Kind: "text", Required: true},
	))
	return s
}

// OnboardingIdentityScreen is the identity check.
func OnboardingIdentityScreen() screen.Screen {
	s := screen.New("onboarding-identity", PathScreenKYCIdent, "Identity check")
	s.Subtitle = "Step 2 of 2"
	s.Back = screen.Go(PathScreenKYCDetail)
	s.Add(screen.Form("Submit", PathKYCIdentity,
		screen.Field{Name: "document", Label: "Photo ID", Kind: "text", Placeholder: "Passport or driving licence", Required: true},
		screen.Field{Name: "selfie", Label: "Selfie", Kind: "text", Placeholder: "Take a photo", Required: true},
	))
	return s
}

// OnboardingPendingScreen is the wait while the checks run.
func OnboardingPendingScreen() screen.Screen {
	s := screen.New("onboarding-pending", PathScreenKYCWait, "Checking your details")
	s.Subtitle = "Usually a few minutes"
	s.Add(
		screen.Text("We will text you when your account is open. Then log in with the customer ID in the message.", screen.ToneMuted),
		screen.Button("Back to log in", screen.ToneMuted, screen.Go(PathScreenLogin)),
	)
	return s
}

// Storyboard is the app as designed over the fixture customer: the screens
// served today and the proposed ones, grouped by the customer's state.
func Storyboard(ctx context.Context, bank core.CustomerQueries, customerID string) (screen.Journeys, error) {
	cust, err := bank.Customer(ctx, customerID)
	if err != nil {
		return screen.Journeys{}, err
	}
	accts, err := bank.Accounts(ctx, customerID)
	if err != nil {
		return screen.Journeys{}, err
	}
	txs, err := bank.Transactions(ctx, customerID, 1)
	if err != nil {
		return screen.Journeys{}, err
	}

	// Served screens, reframed: the designed nav on every logged-in
	// screen, and the log out moved from the accounts list to More.
	login := LoginScreen("")
	login.Add(screen.Button("New to Model Bank? Open an account", screen.ToneMuted, screen.Go(PathScreenOpen)))
	accounts := AccountsScreen(cust, accts)
	accounts.Body = accounts.Body[:len(accounts.Body)-1] // the Log out button
	accounts.Nav = proposedNav("accounts")
	activity := ActivityScreen(cust, txs)
	activity.Nav = proposedNav("activity")
	var product screen.Screen
	if len(accts) > 0 {
		page, err := bank.AccountTransactions(ctx, customerID, accts[0].Index, 1)
		if err != nil {
			return screen.Journeys{}, err
		}
		product = ProductScreen(cust, accts[0], page)
		product.Nav = proposedNav("accounts")
	}
	var sent core.Payment
	if len(txs.Entries) > 0 {
		sent = core.Payment{Amount: 25_00, To: "cust-002", Reference: "PAY-000123", Status: core.PaymentCompleted, CreatedAt: time.Date(2026, 7, 7, 10, 30, 0, 0, time.UTC)}
	}

	screens := []screen.Screen{
		login, SignedOutScreen(""), OpenAccountScreen(),
		OnboardingDetailsScreen(), OnboardingIdentityScreen(), OnboardingPendingScreen(),
		HomeScreen(cust, accts, txs), accounts, product, activity, PayScreen(accts), PaySentScreen(sent), MoreScreen(cust),
	}
	groups := map[string]string{}
	for _, p := range []string{PathScreenLogin, PathScreenSignedOut, PathScreenOpen} {
		groups[screen.NodeID(p)] = GroupLoggedOut
	}
	for _, p := range []string{PathScreenKYCDetail, PathScreenKYCIdent, PathScreenKYCWait} {
		groups[screen.NodeID(p)] = GroupOnboarding
	}
	for _, p := range []string{PathScreenHome2, PathScreenHome, PathScreenTx, PathScreenProduct + "0", PathScreenPay, PathScreenPaySent, PathScreenMore} {
		groups[screen.NodeID(p)] = GroupLoggedIn
	}
	// Everything the BFF does not yet serve in this form.
	proposed := map[string]bool{}
	for _, s := range screens {
		proposed[screen.NodeID(s.Path)] = true
	}
	return screen.Journeys{
		Screens: screens,
		Endpoints: map[string]string{
			PathLogin: PathScreenHome2, PathLogout: PathScreenSignedOut, PathPay: PathScreenPaySent,
			PathKYCDetails: PathScreenKYCIdent, PathKYCIdentity: PathScreenKYCWait,
		},
		Groups:   groups,
		Proposed: proposed,
	}, nil
}
