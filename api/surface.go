package api

// The surface as the walker reads it off the code (package api/walk):
// every route each package registers, with where and how it was
// registered and what the handler is. The OpenAPI document is rendered
// from it, so nothing in the document is written twice.

// Surface is every route of the packages walked.
type Surface struct {
	Routes []Route
}

// Route is one registration on a ServeMux.
type Route struct {
	Package string // import path of the package that registers it
	Method  string // the pattern's method, "" when the pattern has none
	Path    string // the pattern's path, with {name} parameters
	Pos     string // file:line of the registration, relative to the module root

	// Handler is the named function the request finally reaches, as
	// "(*Server).health" or "lofigui.ServeFavicon"; "" for a function
	// literal. Wrappers are the functions it was passed through on the
	// way, outermost first, as "(*Server).authed".
	Handler  string
	Wrappers []string

	// Via is the helper the route was registered through when it was not
	// the mux directly ("control", "(*Site).handle"), and Args the
	// constant arguments of that call besides the pattern, in order.
	Via  string
	Args []string

	// Doc is the handler's doc comment, or the comment above the
	// registration when the handler is a literal.
	Doc string
}
