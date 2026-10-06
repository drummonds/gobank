package sim

import "testing"

// The console settings are a value behind Get and Update: a reader gets a
// snapshot it can keep, a writer changes the fields it names, and no caller
// sees the lock.
func TestSimSettingsGetIsASnapshot(t *testing.T) {
	var s settings
	s.Update(func(v *Settings) { *v = DefaultSettings() })
	snap := s.Get()
	s.Update(func(v *Settings) { v.MaxCustomers = 7 })
	if snap.MaxCustomers != DefaultSettings().MaxCustomers {
		t.Errorf("snapshot changed under the reader: %+v", snap)
	}
	if got := s.Get(); got.MaxCustomers != 7 || got.DayLength != 0 {
		t.Errorf("Update should change only the field it names, got %+v", got)
	}
}

// Reset clears the bank's run, not what the operator set on the console.
