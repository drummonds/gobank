//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/drummonds/lofigui"
)

var (
	bank     = NewBankDemo()
	payments = NewPaymentSim()
)

// --- Bank functions ---

func goRender(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(bank.buildSVG())
	return js.ValueOf(lofigui.Buffer())
}

func goStart(this js.Value, args []js.Value) any {
	bank.Start()
	return nil
}

func goStop(this js.Value, args []js.Value) any {
	bank.Stop()
	return nil
}

func goAdvanceDay(this js.Value, args []js.Value) any {
	bank.AdvanceDay()
	return nil
}

func goDeposit(this js.Value, args []js.Value) any {
	bank.Deposit()
	return nil
}

func goWithdraw(this js.Value, args []js.Value) any {
	bank.Withdraw()
	return nil
}

func goReset(this js.Value, args []js.Value) any {
	bank.Reset()
	return nil
}

func goIsRunning(this js.Value, args []js.Value) any {
	return js.ValueOf(bank.IsRunning())
}

// --- Payments functions ---

func goRenderPayments(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(payments.BuildHTML())
	return js.ValueOf(lofigui.Buffer())
}

func goSendPayment(this js.Value, args []js.Value) any {
	payments.SendPayment()
	return nil
}

func goStartPayments(this js.Value, args []js.Value) any {
	payments.Start()
	return nil
}

func goStopPayments(this js.Value, args []js.Value) any {
	payments.Stop()
	return nil
}

func goIsPaymentsRunning(this js.Value, args []js.Value) any {
	return js.ValueOf(payments.IsRunning())
}

func goResetPayments(this js.Value, args []js.Value) any {
	payments.Reset()
	return nil
}

// --- Models functions ---

func goRenderModels(this js.Value, args []js.Value) any {
	lofigui.Reset()
	lofigui.HTML(BuildModelsHTML())
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

	js.Global().Call("wasmReady")

	<-make(chan struct{})
}
