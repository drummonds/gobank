// Command apidoc walks the servers' packages and writes the API document.
// It is what `go generate ./api` runs; the api package's test fails when
// the committed document is behind the code.
//
//	apidoc -o api/openapi.json          write the OpenAPI document
//	apidoc -dump                        print the walked surface as JSON
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"git.bytestone.uk/hum3/gobank/api"
	"git.bytestone.uk/hum3/gobank/api/walk"
)

func main() {
	out := flag.String("o", "", "write the OpenAPI document here")
	dump := flag.Bool("dump", false, "print the walked surface as JSON and exit")
	flag.Parse()
	surface, err := walk.Walk(walk.Packages...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *dump {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(surface)
		return
	}
	doc, err := api.OpenAPI(surface)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	doc = append(doc, '\n')
	if *out == "" {
		os.Stdout.Write(doc)
		return
	}
	if err := os.WriteFile(*out, doc, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "apidoc: wrote %s (%d routes)\n", *out, len(surface.Routes))
}
