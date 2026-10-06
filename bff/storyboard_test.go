package bff_test

import (
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/bff"
	"git.bytestone.uk/hum3/gobank/bff/stubbank"
	"git.bytestone.uk/hum3/gobank/screen"
)

// The storyboard is the designed app over the fixture customer: a logged-in
// customer lands on the summary, log out leads to a signed-out screen, and
// onboarding is a journey of its own; every screen sits in a group and the
// screens not served in this form are proposed.
func TestStoryboardIsTheDesignedApp(t *testing.T) {
	j, err := bff.Storyboard(t.Context(), stubbank.New(), "cust-001")
	if err != nil {
		t.Fatal(err)
	}
	if j.Endpoints[bff.PathLogin] != bff.PathScreenHome2 || j.Endpoints[bff.PathLogout] != bff.PathScreenSignedOut {
		t.Errorf("login lands on %s and logout on %s; want the summary and the signed-out screen", j.Endpoints[bff.PathLogin], j.Endpoints[bff.PathLogout])
	}
	for _, s := range j.Screens {
		n := screen.NodeID(s.Path)
		if j.Groups[n] == "" {
			t.Errorf("screen %s is in no group", n)
		}
		if s.Nav != nil && s.Nav.Tabs[0].Screen != bff.PathScreenHome2 {
			t.Errorf("screen %s: the first tab is %s, want Home as the summary", n, s.Nav.Tabs[0].Screen)
		}
	}
	d2 := j.Wireframe()
	for _, want := range []string{"logged_out: Logged out", "logged_in: Logged in", "onboarding: Onboarding", "Net Position", "Identity check", "v1_login -> logged_in.v1_screen_home: ok"} {
		if !strings.Contains(d2, want) {
			t.Errorf("wireframe missing %q", want)
		}
	}
}
