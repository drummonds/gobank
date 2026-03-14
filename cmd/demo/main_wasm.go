//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/drummonds/lofigui"
)

var state = NewDemoState()

// --- Bank functions ---

func goRender(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.buildSVG())
	return js.ValueOf(lofigui.Buffer())
}

func goStart(this js.Value, args []js.Value) any {
	state.Start()
	return nil
}

func goStop(this js.Value, args []js.Value) any {
	state.Stop()
	return nil
}

func goAdvanceDay(this js.Value, args []js.Value) any {
	state.AdvanceDay()
	return nil
}

func goAddCustomers(this js.Value, args []js.Value) any {
	n := 100
	if len(args) >= 1 {
		n = args[0].Int()
	}
	state.AddCustomersBatch(n)
	return nil
}

func goIsAddingCustomers(this js.Value, args []js.Value) any {
	return js.ValueOf(state.IsAddingCustomers())
}

func goReset(this js.Value, args []js.Value) any {
	state.Reset()
	return nil
}

func goIsRunning(this js.Value, args []js.Value) any {
	return js.ValueOf(state.IsRunning())
}

// --- Payments functions ---

func goRenderPayments(this js.Value, args []js.Value) any {
	page := 1
	if len(args) >= 1 {
		page = args[0].Int()
	}
	state.mu.Lock()
	piiAuth := state.piiAuthorized
	state.mu.Unlock()
	lofigui.Reset()
	lofigui.HTML(state.BuildPaymentsHTML(piiAuth, page))
	return js.ValueOf(lofigui.Buffer())
}

func goSendPayment(this js.Value, args []js.Value) any {
	state.SendPayment()
	return nil
}

func goStartPayments(this js.Value, args []js.Value) any {
	state.StartPayments()
	return nil
}

func goStopPayments(this js.Value, args []js.Value) any {
	state.StopPayments()
	return nil
}

func goIsPaymentsRunning(this js.Value, args []js.Value) any {
	return js.ValueOf(state.IsPaymentsRunning())
}

func goResetPayments(this js.Value, args []js.Value) any {
	state.ResetPayments()
	return nil
}

// --- About functions ---

func goRenderAbout(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(BuildProjectAboutHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goRenderRuntime(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildRuntimeHTML())
	return js.ValueOf(lofigui.Buffer())
}

// --- Treasury functions ---

func goRenderTreasuryCash(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildCashPositionHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goRenderTreasuryCapital(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildCapitalHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goRenderTreasuryGilts(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildGiltsHTML())
	return js.ValueOf(lofigui.Buffer())
}

// --- Models functions ---

func goRenderModels(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(BuildModelsHTML())
	return js.ValueOf(lofigui.Buffer())
}

// --- Accounting functions ---

func goRenderPnL(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildPnLHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goRenderBalanceSheet(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildBalanceSheetHTML())
	return js.ValueOf(lofigui.Buffer())
}

// --- Products functions ---

func goRenderProducts(this js.Value, args []js.Value) any {
	family := FamilySavings
	if len(args) > 0 && args[0].String() == "lending" {
		family = FamilyLending
	}
	lofigui.Reset()
	lofigui.HTML(state.BuildProductsHTML(family))
	return js.ValueOf(lofigui.Buffer())
}

// --- Customers functions ---

func goRenderCustomers(this js.Value, args []js.Value) any {
	page := 1
	if len(args) >= 1 {
		page = args[0].Int()
	}
	lofigui.Reset()
	lofigui.HTML(state.BuildCustomersHTML(page))
	return js.ValueOf(lofigui.Buffer())
}

func goRenderCustomerDetail(this js.Value, args []js.Value) any {
	id := ""
	txPage := 1
	if len(args) > 0 {
		id = args[0].String()
	}
	if len(args) > 1 {
		txPage = args[1].Int()
	}
	state.mu.Lock()
	piiAuth := state.piiAuthorized
	state.mu.Unlock()
	lofigui.Reset()
	lofigui.HTML(state.BuildCustomerDetailHTML(id, piiAuth, txPage))
	return js.ValueOf(lofigui.Buffer())
}

// --- Payment detail ---

func goRenderPaymentDetail(this js.Value, args []js.Value) any {
	id := 0
	if len(args) > 0 {
		id = args[0].Int()
	}
	state.mu.Lock()
	piiAuth := state.piiAuthorized
	state.mu.Unlock()
	lofigui.Reset()
	lofigui.HTML(state.BuildPaymentDetailHTML(id, piiAuth))
	return js.ValueOf(lofigui.Buffer())
}

// --- Reports functions ---

func goRenderBBSI(this js.Value, args []js.Value) any {
	state.mu.Lock()
	piiAuth := state.piiAuthorized
	state.mu.Unlock()
	lofigui.Reset()
	lofigui.HTML(state.BuildBBSIHTML(piiAuth))
	return js.ValueOf(lofigui.Buffer())
}

func goRenderCustomerViewReport(this js.Value, args []js.Value) any {
	id := ""
	if len(args) > 0 {
		id = args[0].String()
	}
	state.mu.Lock()
	piiAuth := state.piiAuthorized
	state.mu.Unlock()
	lofigui.Reset()
	lofigui.HTML(state.BuildCustomerViewHTML(id, piiAuth))
	return js.ValueOf(lofigui.Buffer())
}

// --- Settings functions ---

func goRenderSettings(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildSettingsHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goUpdateSettings(this js.Value, args []js.Value) any {
	if len(args) >= 1 {
		maxCust := args[0].Int()
		state.UpdateSettings(maxCust)
	}
	return nil
}

// --- PII Auth functions ---

func goAuthorizePII(this js.Value, args []js.Value) any {
	state.mu.Lock()
	state.piiAuthorized = true
	state.mu.Unlock()
	return nil
}

func goRevokePII(this js.Value, args []js.Value) any {
	state.mu.Lock()
	state.piiAuthorized = false
	state.mu.Unlock()
	return nil
}

func goIsPIIAuthorized(this js.Value, args []js.Value) any {
	state.mu.Lock()
	auth := state.piiAuthorized
	state.mu.Unlock()
	return js.ValueOf(auth)
}

func main() {
	// Bank
	js.Global().Set("goRender", js.FuncOf(goRender))
	js.Global().Set("goStart", js.FuncOf(goStart))
	js.Global().Set("goStop", js.FuncOf(goStop))
	js.Global().Set("goAdvanceDay", js.FuncOf(goAdvanceDay))
	js.Global().Set("goAddCustomers", js.FuncOf(goAddCustomers))
	js.Global().Set("goReset", js.FuncOf(goReset))
	js.Global().Set("goIsRunning", js.FuncOf(goIsRunning))
	js.Global().Set("goIsAddingCustomers", js.FuncOf(goIsAddingCustomers))

	// Payments
	js.Global().Set("goRenderPayments", js.FuncOf(goRenderPayments))
	js.Global().Set("goSendPayment", js.FuncOf(goSendPayment))
	js.Global().Set("goStartPayments", js.FuncOf(goStartPayments))
	js.Global().Set("goStopPayments", js.FuncOf(goStopPayments))
	js.Global().Set("goIsPaymentsRunning", js.FuncOf(goIsPaymentsRunning))
	js.Global().Set("goResetPayments", js.FuncOf(goResetPayments))

	// About
	js.Global().Set("goRenderAbout", js.FuncOf(goRenderAbout))
	js.Global().Set("goRenderRuntime", js.FuncOf(goRenderRuntime))

	// Treasury
	js.Global().Set("goRenderTreasuryCash", js.FuncOf(goRenderTreasuryCash))
	js.Global().Set("goRenderTreasuryCapital", js.FuncOf(goRenderTreasuryCapital))
	js.Global().Set("goRenderTreasuryGilts", js.FuncOf(goRenderTreasuryGilts))

	// Models
	js.Global().Set("goRenderModels", js.FuncOf(goRenderModels))

	// Accounting
	js.Global().Set("goRenderPnL", js.FuncOf(goRenderPnL))
	js.Global().Set("goRenderBalanceSheet", js.FuncOf(goRenderBalanceSheet))

	// Products
	js.Global().Set("goRenderProducts", js.FuncOf(goRenderProducts))

	// Customers
	js.Global().Set("goRenderCustomers", js.FuncOf(goRenderCustomers))
	js.Global().Set("goRenderCustomerDetail", js.FuncOf(goRenderCustomerDetail))

	// Payment detail
	js.Global().Set("goRenderPaymentDetail", js.FuncOf(goRenderPaymentDetail))

	// Reports
	js.Global().Set("goRenderBBSI", js.FuncOf(goRenderBBSI))
	js.Global().Set("goRenderCustomerViewReport", js.FuncOf(goRenderCustomerViewReport))

	// Settings
	js.Global().Set("goRenderSettings", js.FuncOf(goRenderSettings))
	js.Global().Set("goUpdateSettings", js.FuncOf(goUpdateSettings))

	// PII Auth
	js.Global().Set("goAuthorizePII", js.FuncOf(goAuthorizePII))
	js.Global().Set("goRevokePII", js.FuncOf(goRevokePII))
	js.Global().Set("goIsPIIAuthorized", js.FuncOf(goIsPIIAuthorized))

	js.Global().Call("wasmReady")

	<-make(chan struct{})
}
