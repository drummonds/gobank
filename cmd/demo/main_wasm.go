//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/drummonds/lofigui"
)

var bank = NewBankDemo()

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

func main() {
	js.Global().Set("goRender", js.FuncOf(goRender))
	js.Global().Set("goStart", js.FuncOf(goStart))
	js.Global().Set("goStop", js.FuncOf(goStop))
	js.Global().Set("goAdvanceDay", js.FuncOf(goAdvanceDay))
	js.Global().Set("goDeposit", js.FuncOf(goDeposit))
	js.Global().Set("goWithdraw", js.FuncOf(goWithdraw))
	js.Global().Set("goReset", js.FuncOf(goReset))
	js.Global().Set("goIsRunning", js.FuncOf(goIsRunning))

	js.Global().Call("wasmReady")

	<-make(chan struct{})
}
