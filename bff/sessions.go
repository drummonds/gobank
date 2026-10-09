package bff

import "git.bytestone.uk/hum3/gobank/internal/session"

// The BFF's sessions and login lockout live in internal/session, shared
// with the staff web; these are their names here.
type (
	Sessions       = session.Store
	SessionStore   = session.Memory
	SQLSessions    = session.SQL
	Table          = session.Table
	FailureLimiter = session.Limiter
)

var (
	CustomerSessionsTable = session.Customers
	StaffSessionsTable    = session.Staff
	SessionsSchema        = session.CustomersSchema
	StaffSessionsSchema   = session.StaffSchema
	NewSessionStore       = session.NewMemory
	NewSQLSessions        = session.NewSQL
	NewFailureLimiter     = session.NewLimiter
)
