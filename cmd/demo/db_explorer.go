package main

import (
	"fmt"
	"html"
	"slices"
	"strings"
)

// getDBTables returns all user table names from sqlite_master.
func (ds *DemoState) getDBTables() []string {
	db := ds.DB()
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tables = append(tables, name)
		}
	}
	return tables
}

// BuildExplorerHTML renders the DB explorer overview: all tables with row/column counts and FK relationships.
func (ds *DemoState) BuildExplorerHTML() string {
	db := ds.DB()
	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">DB Explorer</h2>`)

	if db == nil {
		s.WriteString(`<p class="has-text-danger">Database not available.</p>`)
		return s.String()
	}

	tables := ds.getDBTables()
	if len(tables) == 0 {
		s.WriteString(`<p class="has-text-grey">No tables found.</p>`)
		return s.String()
	}

	// Table list with row/column counts
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Tables</h3>`)
	s.WriteString(`<table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Table</th><th class="has-text-right">Columns</th><th class="has-text-right">Rows</th></tr></thead>`)
	s.WriteString(`<tbody>`)

	for _, name := range tables {
		var rowCount int
		err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, name)).Scan(&rowCount)
		rowStr := fmt.Sprintf("%d", rowCount)
		if err != nil {
			rowStr = "error"
		}

		var colCount int
		colRows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, name))
		if err == nil {
			for colRows.Next() {
				colCount++
			}
			colRows.Close()
		}

		s.WriteString(fmt.Sprintf(`<tr><td><a href="/internal/explorer/%s">%s</a></td><td class="has-text-right">%d</td><td class="has-text-right">%s</td></tr>`,
			name, name, colCount, rowStr))
	}

	s.WriteString(`</tbody></table>`)
	s.WriteString(`</div>`)

	// Foreign key relationships
	type fkRef struct {
		from, fromCol, to, toCol string
	}
	var refs []fkRef

	for _, name := range tables {
		fkRows, err := db.Query(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, name))
		if err != nil {
			continue
		}
		for fkRows.Next() {
			var id, seq int
			var refTable, fromCol, toCol, onUpdate, onDelete, match string
			if fkRows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match) == nil {
				refs = append(refs, fkRef{from: name, fromCol: fromCol, to: refTable, toCol: toCol})
			}
		}
		fkRows.Close()
	}

	if len(refs) > 0 {
		s.WriteString(`<div class="box">`)
		s.WriteString(`<h3 class="title is-5">Relationships</h3>`)
		s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
		s.WriteString(`<thead><tr><th>Table</th><th>Column</th><th></th><th>References</th><th>Column</th></tr></thead>`)
		s.WriteString(`<tbody>`)
		for _, r := range refs {
			s.WriteString(fmt.Sprintf(`<tr><td><a href="/internal/explorer/%s">%s</a></td><td>%s</td><td>&rarr;</td><td><a href="/internal/explorer/%s">%s</a></td><td>%s</td></tr>`,
				r.from, r.from, r.fromCol, r.to, r.to, r.toCol))
		}
		s.WriteString(`</tbody></table>`)
		s.WriteString(`</div>`)
	}

	return s.String()
}

// BuildExplorerTableHTML renders the detail view for a single table: schema, FKs, indexes, and paginated data.
func (ds *DemoState) BuildExplorerTableHTML(name string, page int, sort string, dir string) string {
	db := ds.DB()
	var s strings.Builder

	if db == nil {
		s.WriteString(`<p class="has-text-danger">Database not available.</p>`)
		return s.String()
	}

	// Validate table name against actual DB catalog
	tables := ds.getDBTables()
	valid := slices.Contains(tables, name)
	if !valid {
		s.WriteString(`<h2 class="title is-4">Table Not Found</h2>`)
		s.WriteString(fmt.Sprintf(`<p class="has-text-danger">Unknown table: %s</p>`, html.EscapeString(name)))
		s.WriteString(`<p><a href="/internal/explorer">&larr; Back to explorer</a></p>`)
		return s.String()
	}

	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">Table: %s</h2>`, name))
	s.WriteString(`<p class="mb-4"><a href="/internal/explorer">&larr; Back to explorer</a></p>`)

	// --- Schema ---
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Schema</h3>`)
	var colNames []string
	schemaRows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, name))
	if err == nil {
		s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
		s.WriteString(`<thead><tr><th>#</th><th>Name</th><th>Type</th><th>Not Null</th><th>Default</th><th>PK</th></tr></thead>`)
		s.WriteString(`<tbody>`)
		for schemaRows.Next() {
			var cid int
			var cname, ctype string
			var notnull, pk int
			var dflt *string
			if schemaRows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk) == nil {
				colNames = append(colNames, cname)
				dfltStr := ""
				if dflt != nil {
					dfltStr = html.EscapeString(*dflt)
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
		schemaRows.Close()
		s.WriteString(`</tbody></table>`)
	} else {
		s.WriteString(fmt.Sprintf(`<p class="has-text-danger">Schema error: %v</p>`, err))
	}
	s.WriteString(`</div>`)

	// --- Foreign Keys ---
	fkRows, err := db.Query(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, name))
	if err == nil {
		type fk struct {
			id                                        int
			refTable, fromCol, toCol, onUpdate, onDel string
		}
		var fks []fk
		for fkRows.Next() {
			var id, seq int
			var refTable, fromCol, toCol, onUpdate, onDelete, match string
			if fkRows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match) == nil {
				fks = append(fks, fk{id, refTable, fromCol, toCol, onUpdate, onDelete})
			}
		}
		fkRows.Close()
		if len(fks) > 0 {
			s.WriteString(`<div class="box">`)
			s.WriteString(`<h3 class="title is-5">Foreign Keys</h3>`)
			s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
			s.WriteString(`<thead><tr><th>Column</th><th>References</th><th>Column</th><th>On Update</th><th>On Delete</th></tr></thead>`)
			s.WriteString(`<tbody>`)
			for _, f := range fks {
				s.WriteString(fmt.Sprintf(`<tr><td>%s</td><td><a href="/internal/explorer/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
					f.fromCol, f.refTable, f.refTable, f.toCol, f.onUpdate, f.onDel))
			}
			s.WriteString(`</tbody></table>`)
			s.WriteString(`</div>`)
		}
	}

	// --- Indexes ---
	idxRows, err := db.Query(fmt.Sprintf(`PRAGMA index_list("%s")`, name))
	if err == nil {
		type idx struct {
			name, origin string
			unique       bool
			columns      []string
		}
		var idxs []idx
		for idxRows.Next() {
			var seq int
			var iname, origin string
			var unique, partial int
			if idxRows.Scan(&seq, &iname, &unique, &origin, &partial) == nil {
				ix := idx{name: iname, origin: origin, unique: unique == 1}
				// Get index columns
				ixColRows, err := db.Query(fmt.Sprintf(`PRAGMA index_info("%s")`, iname))
				if err == nil {
					for ixColRows.Next() {
						var seqno, cid int
						var colName string
						if ixColRows.Scan(&seqno, &cid, &colName) == nil {
							ix.columns = append(ix.columns, colName)
						}
					}
					ixColRows.Close()
				}
				idxs = append(idxs, ix)
			}
		}
		idxRows.Close()
		if len(idxs) > 0 {
			s.WriteString(`<div class="box">`)
			s.WriteString(`<h3 class="title is-5">Indexes</h3>`)
			s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
			s.WriteString(`<thead><tr><th>Name</th><th>Columns</th><th>Unique</th><th>Origin</th></tr></thead>`)
			s.WriteString(`<tbody>`)
			for _, ix := range idxs {
				uniq := ""
				if ix.unique {
					uniq = "YES"
				}
				s.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
					ix.name, strings.Join(ix.columns, ", "), uniq, ix.origin))
			}
			s.WriteString(`</tbody></table>`)
			s.WriteString(`</div>`)
		}
	}

	// --- Data browser ---
	const pageSize = 50
	if page < 1 {
		page = 1
	}

	// Validate sort column against actual columns
	validSort := slices.Contains(colNames, sort)
	if !validSort {
		sort = ""
	}
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}

	// Get total row count
	var totalRows int
	db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, name)).Scan(&totalRows)
	totalPages := max((totalRows+pageSize-1)/pageSize, 1)
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize

	s.WriteString(`<div class="box">`)
	s.WriteString(fmt.Sprintf(`<h3 class="title is-5">Data (%d rows)</h3>`, totalRows))

	// Build query
	query := fmt.Sprintf(`SELECT * FROM "%s"`, name)
	if sort != "" {
		query += fmt.Sprintf(` ORDER BY "%s" %s`, sort, dir)
	}
	query += fmt.Sprintf(` LIMIT %d OFFSET %d`, pageSize, offset)

	dataRows, err := db.Query(query)
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

		// Build base URL for sort links
		baseURL := fmt.Sprintf("/internal/explorer/%s?page=%d", name, page)
		for _, col := range cols {
			newDir := "asc"
			arrow := ""
			if col == sort {
				if dir == "asc" {
					newDir = "desc"
					arrow = " &uarr;"
				} else {
					arrow = " &darr;"
				}
			}
			s.WriteString(fmt.Sprintf(`<th><a href="%s&sort=%s&dir=%s">%s%s</a></th>`,
				baseURL, col, newDir, col, arrow))
		}
		s.WriteString(`</tr></thead><tbody>`)

		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		rowCount := 0
		for dataRows.Next() {
			if dataRows.Scan(ptrs...) == nil {
				s.WriteString(`<tr>`)
				for _, v := range vals {
					s.WriteString(fmt.Sprintf(`<td>%s</td>`, html.EscapeString(fmt.Sprintf("%v", v))))
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

	// Pagination
	if totalPages > 1 {
		sortParams := ""
		if sort != "" {
			sortParams = fmt.Sprintf("&sort=%s&dir=%s", sort, dir)
		}
		s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
		if page > 1 {
			s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="/internal/explorer/%s?page=%d%s">Previous</a>`,
				name, page-1, sortParams))
		} else {
			s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
		}
		if page < totalPages {
			s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="/internal/explorer/%s?page=%d%s">Next</a>`,
				name, page+1, sortParams))
		} else {
			s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
		}
		s.WriteString(`<ul class="pagination-list">`)

		// Show page numbers: first, current-1, current, current+1, last
		shown := map[int]bool{}
		pagesToShow := []int{1}
		if page > 2 {
			pagesToShow = append(pagesToShow, page-1)
		}
		pagesToShow = append(pagesToShow, page)
		if page < totalPages-1 {
			pagesToShow = append(pagesToShow, page+1)
		}
		if totalPages > 1 {
			pagesToShow = append(pagesToShow, totalPages)
		}

		lastShown := 0
		for _, p := range pagesToShow {
			if shown[p] {
				continue
			}
			shown[p] = true
			if lastShown > 0 && p > lastShown+1 {
				s.WriteString(`<li><span class="pagination-ellipsis">&hellip;</span></li>`)
			}
			if p == page {
				s.WriteString(fmt.Sprintf(`<li><a class="pagination-link is-current">%d</a></li>`, p))
			} else {
				s.WriteString(fmt.Sprintf(`<li><a class="pagination-link" href="/internal/explorer/%s?page=%d%s">%d</a></li>`,
					name, p, sortParams, p))
			}
			lastShown = p
		}
		s.WriteString(`</ul></nav>`)
	}

	s.WriteString(`</div>`)
	return s.String()
}
