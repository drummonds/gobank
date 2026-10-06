package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Every table in the live database belongs to exactly one component. A
// table nobody claims is a gap in the registry, not a free-for-all.
func TestEveryTableHasOneOwner(t *testing.T) {
	ds := NewDemoState()
	// Names starting with an underscore are the engine's own bookkeeping
	// (pglike's _sequences), not bank data.
	rows, err := ds.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '\_%' ESCAPE '\'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var live []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		live = append(live, name)
	}
	if len(live) == 0 {
		t.Fatal("no tables in the live database")
	}
	for _, table := range live {
		var owners []string
		for _, c := range components {
			for _, ct := range c.Tables {
				if ct == table {
					owners = append(owners, c.Name)
				}
			}
		}
		if len(owners) != 1 {
			t.Errorf("table %s: owned by %v, want exactly one component", table, owners)
		}
	}
	for _, c := range components {
		for _, ct := range c.Tables {
			if !slices.Contains(live, ct) {
				t.Errorf("component %s claims table %s, which the demo never creates", c.Name, ct)
			}
		}
		for _, v := range c.Views {
			var n int
			ds.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'view' AND name = $1`, v).Scan(&n)
			if n != 1 {
				t.Errorf("component %s publishes view %s, which the demo never creates", c.Name, v)
			}
		}
	}
}

// Every package and file a component claims exists, and no file is claimed
// twice.
func TestComponentFilesExist(t *testing.T) {
	seen := map[string]string{}
	for _, c := range components {
		if c.Name == "" || c.Purpose == "" {
			t.Errorf("component %+v needs a name and purpose", c)
		}
		if c.Package != "" {
			if _, err := os.Stat(filepath.Join("../..", c.Package)); err != nil {
				t.Errorf("component %s claims package %s: %v", c.Name, c.Package, err)
			}
		}
		for _, f := range c.Files {
			if _, err := os.Stat(f); err != nil {
				t.Errorf("component %s claims %s: %v", c.Name, f, err)
			}
			if prev, dup := seen[f]; dup {
				t.Errorf("%s claimed by both %s and %s", f, prev, c.Name)
			}
			seen[f] = c.Name
		}
		for _, v := range c.Views {
			if !strings.HasPrefix(v, "contract_") {
				t.Errorf("component %s: contract view %s must be named contract_*", c.Name, v)
			}
		}
	}
}

// The contract-view rule (ADR-0001): code may read or write another
// component's internal tables only through that component's contract views
// or API. SQL in one component's files naming another's table fails, unless
// the pair is in the pre-rule baseline; a baseline entry that no longer
// occurs fails too, so the debt list only shrinks.
func TestContractViewRule(t *testing.T) {
	owner := map[string]string{} // table -> component
	fileOwner := map[string]string{}
	for _, c := range components {
		for _, tbl := range c.Tables {
			owner[tbl] = c.Name
		}
		for _, f := range c.Files {
			fileOwner[f] = c.Name
		}
		if c.Package != "" {
			pkgFiles, _ := filepath.Glob(filepath.Join("../..", c.Package, "*.go"))
			for _, f := range pkgFiles {
				fileOwner[f] = c.Name
			}
		}
	}
	baseline := map[crossRead]bool{}
	for _, d := range contractDebt {
		baseline[d] = true
	}
	seenDebt := map[crossRead]bool{}

	files, _ := filepath.Glob("*.go")
	bankFiles, _ := filepath.Glob("../../bank/*/*.go")
	fset := token.NewFileSet()
	for _, file := range append(files, bankFiles...) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			for _, tbl := range sqlTables(lit.Value, owner) {
				if fileOwner[file] == owner[tbl] {
					continue
				}
				cr := crossRead{File: file, Table: tbl}
				if baseline[cr] {
					seenDebt[cr] = true
					continue
				}
				t.Errorf("%s: %s reads %s, an internal table of %s; use a contract_* view or its API (ADR-0001)",
					fset.Position(lit.Pos()), file, tbl, owner[tbl])
			}
			return true
		})
	}
	for d := range baseline {
		if !seenDebt[d] {
			t.Errorf("baseline entry %s/%s no longer occurs: remove it from contractDebt", d.File, d.Table)
		}
	}
}

// sqlTables returns the internal tables a string literal names in a SQL
// position: directly after FROM, JOIN, INTO, UPDATE, TABLE or VIEW. SQL in
// this package writes keywords in upper case, which keeps HTML templates
// and prose (a "/customers" link, a "Delete" button) out of the match.
func sqlTables(lit string, owner map[string]string) []string {
	var out []string
	for _, m := range sqlTableRef.FindAllStringSubmatch(lit, -1) {
		if _, internal := owner[m[1]]; internal && !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

var sqlTableRef = regexp.MustCompile(`\b(?:FROM|JOIN|INTO|UPDATE|TABLE|VIEW)\s+(?:IF NOT EXISTS\s+|IF EXISTS\s+)?([a-z_][a-z0-9_]*)\b`)

// The matcher sees tables only in SQL positions, so templates and prose
// naming a table or a verb do not trip the rule.
func TestSQLTableMatching(t *testing.T) {
	owner := map[string]string{"movements": "ledger", "gilt_yields": "treasury", "customers": "ledger"}
	cases := []struct {
		lit  string
		want []string
	}{
		{"SELECT a FROM movements m JOIN movements n ON 1", []string{"movements"}},
		{"CREATE TABLE IF NOT EXISTS gilt_yields (tenor VARCHAR(10))", []string{"gilt_yields"}},
		{"DELETE FROM customers", []string{"customers"}},
		{`<a href="/customers">Customers</a> <button>Delete</button>`, nil},
		{"SELECT 1 FROM contract_payments", nil},
		{"SELECT product_id FROM customer_accounts", nil},
	}
	for _, c := range cases {
		if got := sqlTables(c.lit, owner); !slices.Equal(got, c.want) {
			t.Errorf("sqlTables(%q) = %v, want %v", c.lit, got, c.want)
		}
	}
}

// The baseline existed only for reads that predate ADR-0001. It is empty:
// new cross-reads get a contract view or an API, never a baseline entry.
func TestNoContractDebt(t *testing.T) {
	if len(contractDebt) != 0 {
		t.Errorf("contractDebt has %d entries, want none: %v", len(contractDebt), contractDebt)
	}
}
