package main

import (
	"fmt"
	"log"
	"strings"

	mermaid "github.com/bvolpato/mermaid-go-renderer"
)

const c4SystemContext = `C4Context
    title Model Bank - System Context

    Person(customer, "Bank Customer", "A customer of the model bank")
    System(modelBank, "Model Bank", "Simulates core banking: accounts, interest, payments")
    System_Ext(fps, "Faster Payments", "UK Faster Payments Service (mock-fps)")

    Rel(customer, modelBank, "Views accounts, sends payments")
    Rel(modelBank, fps, "Sends/receives payments")
`

const c4Container = `C4Container
    title Model Bank - Container Diagram

    Person(customer, "Bank Customer")

    System_Boundary(bank, "Model Bank") {
        Container(web, "Web UI", "Go WASM + Bulma", "Browser-based dashboard")
        Container(server, "Server", "Go + lofigui", "HTTP server with SSR")
        Container(sim, "Simulation Engine", "Go", "Account lifecycle, interest accrual")
        Container(payments, "Payment Sim", "Go", "Payment generation and processing")
        ContainerDb(ledger, "Ledger", "go-luca", "Double-entry accounting")
    }

    System_Ext(fps, "mock-fps", "FPS simulator")

    Rel(customer, web, "Uses", "HTTPS")
    Rel(customer, server, "Uses", "HTTPS")
    Rel(web, sim, "Calls", "WASM")
    Rel(server, sim, "Calls", "Go API")
    Rel(sim, ledger, "Records transactions")
    Rel(payments, fps, "Sends payments", "HTTP")
    Rel(payments, ledger, "Records movements")
`

func renderMermaidSVG(input string) string {
	svg, err := mermaid.Render(input)
	if err != nil {
		log.Printf("mermaid render error: %v", err)
		return fmt.Sprintf(`<div class="notification is-warning">Diagram render error: %v</div>`, err)
	}
	return svg
}

// BuildModelsHTML renders the C4 architecture diagrams as SVG using mmdg.
func BuildModelsHTML() string {
	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">Architecture Models</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">C4 architecture diagrams for the Model Bank</p>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">System Context</h3>`)
	s.WriteString(renderMermaidSVG(c4SystemContext))
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Container Diagram</h3>`)
	s.WriteString(renderMermaidSVG(c4Container))
	s.WriteString(`</div>`)

	return s.String()
}
