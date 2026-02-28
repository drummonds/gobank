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

func goDeposit(this js.Value, args []js.Value) any {
	state.Deposit()
	return nil
}

func goWithdraw(this js.Value, args []js.Value) any {
	state.Withdraw()
	return nil
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
	lofigui.Reset()
	lofigui.HTML(state.BuildPaymentsHTML())
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
	lofigui.Reset()
	lofigui.HTML(state.BuildCustomersHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goRenderCustomerDetail(this js.Value, args []js.Value) any {
	id := ""
	if len(args) > 0 {
		id = args[0].String()
	}
	lofigui.Reset()
	lofigui.HTML(state.BuildCustomerDetailHTML(id))
	return js.ValueOf(lofigui.Buffer())
}

// --- Payment detail ---

func goRenderPaymentDetail(this js.Value, args []js.Value) any {
	id := 0
	if len(args) > 0 {
		id = args[0].Int()
	}
	lofigui.Reset()
	lofigui.HTML(state.BuildPaymentDetailHTML(id))
	return js.ValueOf(lofigui.Buffer())
}

// --- Reports functions ---

func goRenderBBSI(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(state.BuildBBSIHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goRenderCustomerViewReport(this js.Value, args []js.Value) any {
	id := ""
	if len(args) > 0 {
		id = args[0].String()
	}
	lofigui.Reset()
	lofigui.HTML(state.BuildCustomerViewHTML(id))
	return js.ValueOf(lofigui.Buffer())
}

func main() {
	// Bank
	js.Global().Set("goRender", js.FuncOf(goRender))
	js.Global().Set("goStart", js.FuncOf(goStart))
	js.Global().Set("goStop", js.FuncOf(goStop))
	js.Global().Set("goAdvanceDay", js.FuncOf(goAdvanceDay))
	js.Global().Set("goDeposit", js.FuncOf(goDeposit))
	js.Global().Set("goWithdraw", js.FuncOf(goWithdraw))
	js.Global().Set("goReset", js.FuncOf(goReset))
	js.Global().Set("goIsRunning", js.FuncOf(goIsRunning))

	// Payments
	js.Global().Set("goRenderPayments", js.FuncOf(goRenderPayments))
	js.Global().Set("goSendPayment", js.FuncOf(goSendPayment))
	js.Global().Set("goStartPayments", js.FuncOf(goStartPayments))
	js.Global().Set("goStopPayments", js.FuncOf(goStopPayments))
	js.Global().Set("goIsPaymentsRunning", js.FuncOf(goIsPaymentsRunning))
	js.Global().Set("goResetPayments", js.FuncOf(goResetPayments))

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

	js.Global().Call("wasmReady")

	<-make(chan struct{})
}
