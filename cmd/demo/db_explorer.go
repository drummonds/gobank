package main

import (
	"fmt"
	"html"
	"slices"
	"strings"
)

// The explorer reads catalog metadata through the helpers below, which
// branch on the backend: pglike exposes SQLite-style sqlite_master/PRAGMA,
// real PostgreSQL uses information_schema/pg_catalog.

// dbColInfo describes one column of a table for the explorer.
type dbColInfo struct {
	pos     int
	name    string
	ctype   string
	notNull bool
	dflt    string
	pk      bool
}

// dbFKInfo describes one foreign key reference for the explorer.
type dbFKInfo struct {
	fromCol, refTable, toCol, onUpdate, onDelete string
}

// dbIdxInfo describes one index for the explorer.
type dbIdxInfo struct {
	name    string
	columns string
	unique  bool
	origin  string
}

// getDBTables returns all user table names.
func (ds *DemoState) getDBTables() []string {
	db := ds.DB()
	if db == nil {
		return nil
	}
	q := `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	if ds.dbIsPostgres {
		q = `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`
	}
	rows, err := db.Query(q)
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

// getTableColumns returns the columns of a table in definition order.
func (ds *DemoState) getTableColumns(name string) []dbColInfo {
	db := ds.DB()
	if db == nil {
		return nil
	}
	var cols []dbColInfo
	if ds.dbIsPostgres {
		rows, err := db.Query(`SELECT c.ordinal_position, c.column_name, c.data_type,
				c.is_nullable = 'NO', COALESCE(c.column_default, ''), COALESCE(pk.is_pk, false)
			FROM information_schema.columns c
			LEFT JOIN (SELECT kcu.column_name, true AS is_pk
				FROM information_schema.table_constraints tc
				JOIN information_schema.key_column_usage kcu
					ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
				WHERE tc.constraint_type = 'PRIMARY KEY'
					AND tc.table_schema = 'public' AND tc.table_name = $1
			) pk ON pk.column_name = c.column_name
			WHERE c.table_schema = 'public' AND c.table_name = $1
			ORDER BY c.ordinal_position`, name)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var c dbColInfo
			if rows.Scan(&c.pos, &c.name, &c.ctype, &c.notNull, &c.dflt, &c.pk) == nil {
				cols = append(cols, c)
			}
		}
		return cols
	}
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, name))
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var c dbColInfo
		var notnull, pk int
		var dflt *string
		if rows.Scan(&c.pos, &c.name, &c.ctype, &notnull, &dflt, &pk) == nil {
			c.notNull = notnull == 1
			c.pk = pk > 0
			if dflt != nil {
				c.dflt = *dflt
			}
			cols = append(cols, c)
		}
	}
	return cols
}

// getTableFKs returns the foreign keys declared on a table.
func (ds *DemoState) getTableFKs(name string) []dbFKInfo {
	db := ds.DB()
	if db == nil {
		return nil
	}
	var fks []dbFKInfo
	if ds.dbIsPostgres {
		rows, err := db.Query(`SELECT kcu.column_name, ccu.table_name, ccu.column_name,
				rc.update_rule, rc.delete_rule
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
			JOIN information_schema.constraint_column_usage ccu
				ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
			JOIN information_schema.referential_constraints rc
				ON rc.constraint_name = tc.constraint_name AND rc.constraint_schema = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY'
				AND tc.table_schema = 'public' AND tc.table_name = $1`, name)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var f dbFKInfo
			if rows.Scan(&f.fromCol, &f.refTable, &f.toCol, &f.onUpdate, &f.onDelete) == nil {
				fks = append(fks, f)
			}
		}
		return fks
	}
	rows, err := db.Query(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, name))
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id, seq int
		var f dbFKInfo
		var match string
		if rows.Scan(&id, &seq, &f.refTable, &f.fromCol, &f.toCol, &f.onUpdate, &f.onDelete, &match) == nil {
			fks = append(fks, f)
		}
	}
	return fks
}

// getTableIndexes returns the indexes on a table.
func (ds *DemoState) getTableIndexes(name string) []dbIdxInfo {
	db := ds.DB()
	if db == nil {
		return nil
	}
	var idxs []dbIdxInfo
	if ds.dbIsPostgres {
		rows, err := db.Query(`SELECT indexname, indexdef FROM pg_indexes
			WHERE schemaname = 'public' AND tablename = $1 ORDER BY indexname`, name)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var ix dbIdxInfo
			var def string
			if rows.Scan(&ix.name, &def) != nil {
				continue
			}
			ix.unique = strings.HasPrefix(def, "CREATE UNIQUE")
			if lp, rp := strings.Index(def, "("), strings.LastIndex(def, ")"); lp >= 0 && rp > lp {
				ix.columns = def[lp+1 : rp]
			}
			if strings.HasSuffix(ix.name, "_pkey") {
				ix.origin = "pk"
			}
			idxs = append(idxs, ix)
		}
		return idxs
	}
	rows, err := db.Query(fmt.Sprintf(`PRAGMA index_list("%s")`, name))
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var unique, partial int
		var ix dbIdxInfo
		if rows.Scan(&seq, &ix.name, &unique, &ix.origin, &partial) != nil {
			continue
		}
		ix.unique = unique == 1
		var cols []string
		ixColRows, err := db.Query(fmt.Sprintf(`PRAGMA index_info("%s")`, ix.name))
		if err == nil {
			for ixColRows.Next() {
				var seqno, cid int
				var colName string
				if ixColRows.Scan(&seqno, &cid, &colName) == nil {
					cols = append(cols, colName)
				}
			}
			ixColRows.Close()
		}
		ix.columns = strings.Join(cols, ", ")
		idxs = append(idxs, ix)
	}
	return idxs
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

		colCount := len(ds.getTableColumns(name))

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
		for _, f := range ds.getTableFKs(name) {
			refs = append(refs, fkRef{from: name, fromCol: f.fromCol, to: f.refTable, toCol: f.toCol})
		}
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

// truncLen is the display length string/ID cell values are cut to when the
// explorer's truncate option is on; the full value stays in the hover tooltip.
const truncLen = 10

// truncCell renders one data cell. With trunc on, string and []byte values
// longer than truncLen display as a prefix plus an ellipsis, keeping the full
// value in the title attribute. Other types (numbers, times) are never cut.
func truncCell(v any, trunc bool) string {
	full := fmt.Sprintf("%v", v)
	if trunc {
		var isText bool
		switch v.(type) {
		case string, []byte:
			isText = true
		}
		if isText && len([]rune(full)) > truncLen {
			short := string([]rune(full)[:truncLen])
			return fmt.Sprintf(`<td title="%s">%s&hellip;</td>`, html.EscapeString(full), html.EscapeString(short))
		}
	}
	return fmt.Sprintf(`<td>%s</td>`, html.EscapeString(full))
}

// BuildExplorerTableHTML renders the detail view for a single table: schema,
// FKs, indexes, and paginated data. trunc shortens ID/text cells to truncLen
// characters and is carried through sort/pagination links as ?trunc=1.
func (ds *DemoState) BuildExplorerTableHTML(name string, page int, sort string, dir string, trunc bool) string {
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
	schemaCols := ds.getTableColumns(name)
	if len(schemaCols) > 0 {
		s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
		s.WriteString(`<thead><tr><th>#</th><th>Name</th><th>Type</th><th>Not Null</th><th>Default</th><th>PK</th></tr></thead>`)
		s.WriteString(`<tbody>`)
		for _, c := range schemaCols {
			colNames = append(colNames, c.name)
			nnStr := ""
			if c.notNull {
				nnStr = "YES"
			}
			pkStr := ""
			if c.pk {
				pkStr = "YES"
			}
			s.WriteString(fmt.Sprintf(`<tr><td>%d</td><td><strong>%s</strong></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				c.pos, c.name, c.ctype, nnStr, html.EscapeString(c.dflt), pkStr))
		}
		s.WriteString(`</tbody></table>`)
	} else {
		s.WriteString(`<p class="has-text-danger">Schema not available.</p>`)
	}
	s.WriteString(`</div>`)

	// --- Foreign Keys ---
	if fks := ds.getTableFKs(name); len(fks) > 0 {
		s.WriteString(`<div class="box">`)
		s.WriteString(`<h3 class="title is-5">Foreign Keys</h3>`)
		s.WriteString(`<table class="table is-fullwidth is-striped is-narrow">`)
		s.WriteString(`<thead><tr><th>Column</th><th>References</th><th>Column</th><th>On Update</th><th>On Delete</th></tr></thead>`)
		s.WriteString(`<tbody>`)
		for _, f := range fks {
			s.WriteString(fmt.Sprintf(`<tr><td>%s</td><td><a href="/internal/explorer/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				f.fromCol, f.refTable, f.refTable, f.toCol, f.onUpdate, f.onDelete))
		}
		s.WriteString(`</tbody></table>`)
		s.WriteString(`</div>`)
	}

	// --- Indexes ---
	if idxs := ds.getTableIndexes(name); len(idxs) > 0 {
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
				html.EscapeString(ix.name), html.EscapeString(ix.columns), uniq, ix.origin))
		}
		s.WriteString(`</tbody></table>`)
		s.WriteString(`</div>`)
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

	truncParam := ""
	if trunc {
		truncParam = "&trunc=1"
	}
	sortParams := ""
	if sort != "" {
		sortParams = fmt.Sprintf("&sort=%s&dir=%s", sort, dir)
	}

	s.WriteString(`<div class="box">`)
	s.WriteString(fmt.Sprintf(`<h3 class="title is-5">Data (%d rows)</h3>`, totalRows))

	// Truncation toggle, preserving page and sort in the link.
	toggleLabel := "Truncate IDs/text to 10 chars"
	toggleParam := "&trunc=1"
	if trunc {
		toggleLabel = "Show full values"
		toggleParam = ""
	}
	s.WriteString(fmt.Sprintf(`<p class="mb-3"><a class="button is-small" href="/internal/explorer/%s?page=%d%s%s">%s</a></p>`,
		name, page, sortParams, toggleParam, toggleLabel))

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
		baseURL := fmt.Sprintf("/internal/explorer/%s?page=%d%s", name, page, truncParam)
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
					s.WriteString(truncCell(v, trunc))
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

	// Pagination — carry sort and trunc options across pages.
	if totalPages > 1 {
		sortParams += truncParam
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
