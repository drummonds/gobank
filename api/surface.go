package api

// The surface as the walker reads it off the code (package api/walk):
// every route each package registers, with where and how it was
// registered and what the handler is. The OpenAPI document is rendered
// from it, so nothing in the document is written twice.

// Surface is every route of the packages walked, and the gates in front
// of their muxes.
type Surface struct {
	Module     string      // the module walked, as "git.bytestone.uk/hum3/gobank": where the source is
	Packages   []Package   // the packages walked, with their doc comments
	Registrars []Registrar // the functions routes are registered in: the sections of a package
	Routes     []Route
	Gates      []Gate
	Types      []Schema // every type the routes' bodies are made of, by name
}

// Registrar is a function that registers routes, with its doc comment:
// a package's routes fall into sections by it.
type Registrar struct {
	Package, Name, Doc string
}

// Package is one package walked: a server, described by its own doc.
type Package struct {
	Path, Name, Doc string
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

	// Registrar is the function the registration is in, as
	// "(*Site).consoleRoutes": the section the route belongs to.
	Registrar string

	// Via is the helper the route was registered through when it was not
	// the mux directly ("control", "(*Site).handle"), and Args the
	// constant arguments of that call besides the pattern, in order.
	Via  string
	Args []string

	// Doc is the handler's doc comment, or the comment above the
	// registration when the handler is a literal.
	Doc string

	RouteFacts
}

// What a handler does, read off its body and the bodies it calls within
// the package: the responses it can write, the request it reads, and what
// it looks at. Branches are not followed, so every fact is a possibility.

// Response is one way the handler answers.
type Response struct {
	Status   int    // 200 when the handler writes a body without setting one
	Media    string // the Content-Type set before it, "" when none was
	Type     string // the Go type encoded as JSON, as "screen.Screen" or "AboutStatus"; "" when the body is not an encoded value
	Note     string // the first constant string passed with the status: an error code, a message
	Redirect string // the target of a redirect when it is constant
}

// Param is a value the handler reads from the request.
type Param struct {
	Name string
	In   string // "path", "query", "form" (FormValue reads either), "file"
}

// Request is a body the handler decodes.
type Request struct {
	Type string // the Go type decoded from JSON
}

// Reads is what the handler and its wrappers look at on the request.
type Reads struct {
	Headers []string // header names read
	Cookies []string // cookie names read
}

// RouteFacts are the facts for one route, held beside it on Route.
type RouteFacts struct {
	Request   *Request
	Responses []Response
	Params    []Param
	Reads     Reads    // what the handler itself looks at
	Guards    Reads    // what the wrappers around it look at to let the request through: its security
	Action    string   // the role action the handler requires (staff's require), "" when none
	Methods   []string // the methods the handler tells apart ("GET", "POST"); none when it serves them all alike
	Unknowns  []string // what the walker could not follow: a handler whose body is outside the walk, a route with no response found
}

// Gate is a wrapper in front of a package's whole mux: what it reads to
// let a request through, and the paths it lets through unread.
type Gate struct {
	Package string
	Wrapper string   // as "(*Site).requireLogin"
	Doc     string   // the wrapper's doc comment
	Reads   Reads    // what it looks at
	Open    []string // paths served without it
}

// Schema is one type a body is made of, read off go/types with the doc
// comments of its declaration and fields.
type Schema struct {
	Name    string  // qualified by package name: "screen.Screen", "bff.errorBody", "[]screen.Component"
	Package string  // import path of a named type's package, "" for a composite
	Kind    string  // object, array, map, string, integer, number, boolean, any
	Doc     string  // the type's doc comment
	Fields  []Field // object: in declaration order
	Elem    string  // array or map: the element's schema name or scalar kind
	Enum    []string
	Format  string // "date-time" for time.Time
}

// Field is one JSON property of an object schema.
type Field struct {
	Name     string // the JSON name
	Type     string // a schema name, or a scalar kind
	Doc      string // the field's doc or line comment
	Optional bool   // omitempty
	Nullable bool   // a pointer
}
