package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

// simulationFiles hold the generators (ADR-0002 stage 4, story a): the
// population and its payments. They raise events on the bank through
// core.Commands and decide them from core.StaffQueries, and touch nothing
// of the bank's own. Story (c) moves them into a package of their own,
// where the compiler takes over this check.
var simulationFiles = []string{"sim_customers.go", "sim_payments.go", "sim_clock.go", "sim_rates.go"}

// simulationState is what of DemoState the generators may use: the
// simulation's own knobs and progress, its randomness, its clock, and the
// bank as the core presents it.
var simulationState = []string{
	"mu", "epoch", "rng", "settings", "now", "bank", "catalogue", "simClock",
	"payRunning", "payCancel",
	"addingCustRunning", "addingCustCancel", "addingCustProgress", "addingCustTarget", "addingCustStart", "lastAddRate",
	"dbWriters",
}

// The generators reach the bank only through its commands and queries: in
// a simulation file, a DemoState member used is either simulation state
// or a method the simulation files themselves declare.
func TestGeneratorsReachTheBankOnlyThroughCommands(t *testing.T) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range simulationFiles {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		files = append(files, f)
	}
	allowed := slices.Clone(simulationState)
	for _, f := range files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && receiverIsDemoState(fn) {
				allowed = append(allowed, fn.Name.Name)
			}
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !receiverIsDemoState(fn) || fn.Body == nil {
				continue
			}
			recv := fn.Recv.List[0].Names[0].Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == recv && !slices.Contains(allowed, sel.Sel.Name) {
					t.Errorf("%s: %s.%s is the bank's own; the simulation reaches the bank through %s.bank",
						fset.Position(sel.Pos()), recv, sel.Sel.Name, recv)
				}
				return true
			})
		}
	}
}

func receiverIsDemoState(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "DemoState"
}
