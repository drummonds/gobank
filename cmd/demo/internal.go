//go:build !(js && wasm)

package main

import (
	"fmt"
	"strings"
)

// knownTables is the allowlist of tables exposed in the viewer.
var knownTables = []string{"gilt_yields", "gilt_holdings"}

// BuildTablesHTML renders the list of DB tables with row counts.
func (ds *DemoState) BuildTablesHTML() string {
	db := ds.DB()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Database Tables</h2>`)

	if db == nil {
		s.WriteString(`<p class="has-text-danger">Database not available.</p>`)
		return s.String()
	}

	s.WriteString(`<div class="box">`)
	s.WriteString(`<table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Table</th><th class="has-text-right">Row Count</th></tr></thead>`)
	s.WriteString(`<tbody>`)

	for _, name := range knownTables {
		var count int
		err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, name)).Scan(&count)
		if err != nil {
			count = -1
		}
		countStr := fmt.Sprintf("%d", count)
		if count < 0 {
			countStr = "error"
		}
		s.WriteString(fmt.Sprintf(`<tr><td><a href="/internal/tables/%s">%s</a></td><td class="has-text-right">%s</td></tr>`, name, name, countStr))
	}

	s.WriteString(`</tbody></table>`)
	s.WriteString(`</div>`)

	return s.String()
}

// BuildTableDetailHTML renders the schema and first 50 rows of a named table.
func (ds *DemoState) BuildTableDetailHTML(name string) string {
	// Validate against allowlist
	valid := false
	for _, t := range knownTables {
		if t == name {
			valid = true
			break
		}
	}

	var s strings.Builder

	if !valid {
		s.WriteString(`<h2 class="title is-4">Table Not Found</h2>`)
		s.WriteString(fmt.Sprintf(`<p class="has-text-danger">Unknown table: %s</p>`, name))
		s.WriteString(`<p><a href="/internal/tables">Back to tables</a></p>`)
		return s.String()
	}

	db := ds.DB()
	if db == nil {
		s.WriteString(`<p class="has-text-danger">Database not available.</p>`)
		return s.String()
	}

	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">Table: %s</h2>`, name))
	s.WriteString(`<p class="mb-4"><a href="/internal/tables">&larr; Back to tables</a></p>`)

	// Schema via PRAGMA table_info
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Schema</h3>`)
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, name))
	if err == nil {
		s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
		s.WriteString(`<thead><tr><th>#</th><th>Name</th><th>Type</th><th>Not Null</th><th>Default</th><th>PK</th></tr></thead>`)
		s.WriteString(`<tbody>`)
		for rows.Next() {
			var cid int
			var cname, ctype string
			var notnull, pk int
			var dflt *string
			if err := rows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err == nil {
				dfltStr := ""
				if dflt != nil {
					dfltStr = *dflt
				}
				nnStr := ""
				if notnull == 1 {
					nnStr = "YES"
				}
				pkStr := ""
				if pk > 0 {
					pkStr = "YES"
				}
				s.WriteString(fmt.Sprintf(`<tr><td>%d</td><td><strong>%s</strong></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
					cid, cname, ctype, nnStr, dfltStr, pkStr))
			}
		}
		rows.Close()
		s.WriteString(`</tbody></table>`)
	} else {
		s.WriteString(fmt.Sprintf(`<p class="has-text-danger">Schema error: %v</p>`, err))
	}
	s.WriteString(`</div>`)

	// First 50 rows
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Data (first 50 rows)</h3>`)

	dataRows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s" LIMIT 50`, name))
	if err != nil {
		s.WriteString(fmt.Sprintf(`<p class="has-text-danger">Query error: %v</p>`, err))
		s.WriteString(`</div>`)
		return s.String()
	}
	defer dataRows.Close()

	cols, _ := dataRows.Columns()
	if len(cols) > 0 {
		s.WriteString(`<div style="overflow-x:auto">`)
		s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
		s.WriteString(`<thead><tr>`)
		for _, col := range cols {
			s.WriteString(fmt.Sprintf(`<th>%s</th>`, col))
		}
		s.WriteString(`</tr></thead><tbody>`)

		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		rowCount := 0
		for dataRows.Next() {
			if err := dataRows.Scan(ptrs...); err == nil {
				s.WriteString(`<tr>`)
				for _, v := range vals {
					s.WriteString(fmt.Sprintf(`<td>%v</td>`, v))
				}
				s.WriteString(`</tr>`)
				rowCount++
			}
		}
		s.WriteString(`</tbody></table></div>`)
		if rowCount == 0 {
			s.WriteString(`<p class="has-text-grey">No rows.</p>`)
		}
	}
	s.WriteString(`</div>`)

	return s.String()
}
