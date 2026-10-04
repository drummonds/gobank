// Package screen defines the server-driven screen tree that the BFF sends to
// thin clients.
//
// A Screen is a complete description of one app view: header, body
// components, and the bottom navigation. Every presentation decision (money
// formatting, sign and colour of an amount, icon, grouping, paging) is made by
// the server that builds the tree. Clients only render: the Flutter shell
// turns the JSON into widgets, and HTML renders the same tree for the
// browser and the demo phone frame.
//
// The schema is versioned. Clients must render a fallback for any component
// type they do not know, so the server can add components without breaking
// older apps.
package screen

// SchemaVersion is the version of the screen JSON schema this package emits.
const SchemaVersion = 1

// Screen is one app view.
type Screen struct {
	Schema   int         `json:"schema"`
	ID       string      `json:"id"`
	Path     string      `json:"path"` // GET path that returns this screen
	Title    string      `json:"title"`
	Subtitle string      `json:"subtitle,omitempty"`
	Back     *Action     `json:"back,omitempty"`
	Body     []Component `json:"body"`
	Nav      *Nav        `json:"nav,omitempty"`
}

// Action is what happens when the user taps a component. Exactly one of the
// fields is set.
type Action struct {
	Screen string `json:"screen,omitempty"` // GET this path and show the result
	Submit string `json:"submit,omitempty"` // POST form values to this path
	Logout bool   `json:"logout,omitempty"` // end the session
}

// Nav is the bottom tab bar.
type Nav struct {
	Active string `json:"active"`
	Tabs   []Tab  `json:"tabs"`
}

// Tab is one entry in the bottom tab bar.
type Tab struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Icon   Icon   `json:"icon"`
	Screen string `json:"screen"`
}

// Tone selects a colour treatment. Clients map tones to their theme.
type Tone string

// Tones.
const (
	ToneNeutral  Tone = ""
	ToneSavings  Tone = "savings"
	ToneLending  Tone = "lending"
	TonePositive Tone = "positive"
	ToneNegative Tone = "negative"
	ToneDanger   Tone = "danger"
	ToneMuted    Tone = "muted"
)

// Icon names a glyph. Clients map icons to their icon set.
type Icon string

// Icons.
const (
	IconNone     Icon = ""
	IconHome     Icon = "home"
	IconAccounts Icon = "accounts"
	IconActivity Icon = "activity"
	IconSavings  Icon = "savings"
	IconLending  Icon = "lending"
	IconIn       Icon = "in"
	IconOut      Icon = "out"
	IconInterest Icon = "interest"
	IconLoan     Icon = "loan"
	IconBank     Icon = "bank"
)

// Component types. See the constructor functions for which fields each uses.
const (
	TypeHero    = "hero"    // large balance card: Title (label), Value, Pairs, Tone
	TypeRow     = "row"     // tappable list row: Icon, Title, Subtitle, Value, Note, Tone, Action
	TypeTx      = "tx"      // transaction row: Icon, Title, Subtitle, Value, Note, Tone
	TypeHeading = "heading" // section heading: Text
	TypeText    = "text"    // paragraph or notice: Text, Tone
	TypeDetails = "details" // label/value card: Title, Pairs
	TypeForm    = "form"    // input form: Fields, Title (submit label), Action.Submit
	TypeButton  = "button"  // standalone button or link: Title, Tone, Action
)

// Component is one element of a screen body. Type selects the component and
// which of the other fields are meaningful; unused fields are omitted from
// JSON.
type Component struct {
	Type     string  `json:"type"`
	Tone     Tone    `json:"tone,omitempty"`
	Icon     Icon    `json:"icon,omitempty"`
	Title    string  `json:"title,omitempty"`
	Subtitle string  `json:"subtitle,omitempty"`
	Value    string  `json:"value,omitempty"`
	Note     string  `json:"note,omitempty"`
	Currency string  `json:"currency,omitempty"` // ISO 4217 code of the money a row or tx shows; the renderer picks the glyph by it
	Text     string  `json:"text,omitempty"`
	Pairs    []Pair  `json:"pairs,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
	Action   *Action `json:"action,omitempty"`
}

// Pair is a label/value line inside a hero or details component.
type Pair struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Field is one input in a form.
type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Kind        string `json:"kind"` // "text", "password", "number"
	Placeholder string `json:"placeholder,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// New returns an empty screen with the current schema version.
func New(id, path, title string) Screen {
	return Screen{Schema: SchemaVersion, ID: id, Path: path, Title: title, Body: []Component{}}
}

// Add appends components to the body and returns the screen for chaining.
func (s *Screen) Add(c ...Component) *Screen {
	s.Body = append(s.Body, c...)
	return s
}

// Hero builds a large balance card.
func Hero(label, value string, tone Tone, pairs ...Pair) Component {
	return Component{Type: TypeHero, Title: label, Value: value, Tone: tone, Pairs: pairs}
}

// Row builds a tappable list row.
func Row(icon Icon, title, subtitle, value, note string, tone Tone, action *Action) Component {
	return Component{Type: TypeRow, Icon: icon, Title: title, Subtitle: subtitle, Value: value, Note: note, Tone: tone, Action: action}
}

// Tx builds a transaction row. Value should carry its sign ("+£1.00").
func Tx(icon Icon, title, subtitle, value, note string, tone Tone) Component {
	return Component{Type: TypeTx, Icon: icon, Title: title, Subtitle: subtitle, Value: value, Note: note, Tone: tone}
}

// Heading builds a section heading.
func Heading(text string) Component { return Component{Type: TypeHeading, Text: text} }

// Text builds a paragraph; ToneDanger renders as an error notice.
func Text(text string, tone Tone) Component { return Component{Type: TypeText, Text: text, Tone: tone} }

// Details builds a label/value card.
func Details(title string, pairs ...Pair) Component {
	return Component{Type: TypeDetails, Title: title, Pairs: pairs}
}

// Form builds an input form that POSTs to submit.
func Form(submitLabel, submit string, fields ...Field) Component {
	return Component{Type: TypeForm, Title: submitLabel, Action: &Action{Submit: submit}, Fields: fields}
}

// Button builds a standalone button.
func Button(title string, tone Tone, action *Action) Component {
	return Component{Type: TypeButton, Title: title, Tone: tone, Action: action}
}

// Go returns an action that shows the screen at path.
func Go(path string) *Action { return &Action{Screen: path} }

// Logout returns the logout action.
func Logout() *Action { return &Action{Logout: true} }
