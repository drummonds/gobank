package staff

import (
	"context"
	"sync"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// Role is what a member of staff may do on the staff web. The role comes
// from the signed-in user (story 1.7.1); the permissions stay a code
// table here until story 1.7.3 moves them into the users component.
type Role string

const (
	RoleAdmin           Role = "admin"
	RoleAuditor         Role = "auditor"
	RoleCustomerService Role = "cs"
	RoleReadOnly        Role = "readonly"
)

// staffRoles is every staff role, in order of precedence: a user holding
// several has the first.
var staffRoles = []Role{RoleAdmin, RoleAuditor, RoleCustomerService, RoleReadOnly}

// RoleOf is the role a user has on the staff web, and whether it has one:
// a user holding none of the staff roles (a customer) cannot sign in here.
func RoleOf(u core.User) (Role, bool) {
	for _, r := range staffRoles {
		if u.HasRole(string(r)) {
			return r, true
		}
	}
	return "", false
}

// Label returns a human-readable label for the role.
func (r Role) Label() string {
	switch r {
	case RoleAdmin:
		return "Admin"
	case RoleAuditor:
		return "Auditor"
	case RoleCustomerService:
		return "Customer Service"
	case RoleReadOnly:
		return "Read Only"
	default:
		return "Admin"
	}
}

// Can returns whether the role has permission for the given action.
func (r Role) Can(action string) bool {
	switch action {
	case "sim_controls":
		return r == RoleAdmin
	case "settings":
		return r == RoleAdmin
	case "export":
		return r == RoleAdmin
	case "send_payment":
		return r == RoleAdmin || r == RoleCustomerService
	case "view_pii":
		return r != RoleReadOnly
	case "buy_gilt":
		return r == RoleAdmin
	default:
		return false
	}
}

// CanViewComponent reports whether the role may browse a component's tables
// in the DB explorer. The customers component holds PII, so it needs
// view_pii; the rest, and unowned tables (""), are open to every role.
func (r Role) CanViewComponent(component string) bool {
	if component == "customers" {
		return r.Can("view_pii")
	}
	return true
}

// roleKey carries the viewer's role in a context.
type roleKey struct{}

// WithRole returns ctx carrying the viewer's role.
func WithRole(ctx context.Context, r Role) context.Context {
	return context.WithValue(ctx, roleKey{}, r)
}

// RoleFrom is the viewer's role in ctx; admin when none is set, as for a
// page built outside a request.
func RoleFrom(ctx context.Context) Role {
	if r, ok := ctx.Value(roleKey{}).(Role); ok {
		return r
	}
	return RoleAdmin
}

// piiStore keeps each session's PII authorisation: an admin sees personal
// data for a while after authorising, and the authorisation goes with
// the process.
type piiStore struct {
	mu     sync.Mutex
	expiry map[string]time.Time
	ttl    time.Duration
}

func newPIIStore(ttl time.Duration) *piiStore {
	return &piiStore{expiry: map[string]time.Time{}, ttl: ttl}
}

func (p *piiStore) authorise(session string) {
	p.mu.Lock()
	p.expiry[session] = time.Now().Add(p.ttl)
	p.mu.Unlock()
}

func (p *piiStore) revoke(session string) {
	p.mu.Lock()
	delete(p.expiry, session)
	p.mu.Unlock()
}

// effective says whether a session of the given role sees personal data
// now. Auditor and customer service always do, read-only never, an admin
// while authorised.
func (p *piiStore) effective(session string, role Role) bool {
	switch role {
	case RoleAuditor, RoleCustomerService:
		return true
	case RoleReadOnly:
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.expiry[session]
	return ok && time.Now().Before(until)
}
