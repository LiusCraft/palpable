package id

import "testing"

func TestParticipant(t *testing.T) {
	const mac DeviceID = "aa:bb:cc:dd:ee:ff"
	cases := []struct {
		p     ParticipantID
		kind  Kind
		owner DeviceID
	}{
		{Device(mac), KindDevice, mac},
		{"agt:aa:bb:cc:dd:ee:ff", KindAgent, mac},
		{"aa:bb:cc:dd:ee:ff", KindUnknown, ""},
		{"", KindUnknown, ""},
	}
	for _, c := range cases {
		if got := c.p.Kind(); got != c.kind {
			t.Errorf("%q.Kind() = %v, want %v", c.p, got, c.kind)
		}
		if got := OwnerDevice(c.p); got != c.owner {
			t.Errorf("OwnerDevice(%q) = %q, want %q", c.p, got, c.owner)
		}
	}
	if Device(mac) != "dev:aa:bb:cc:dd:ee:ff" {
		t.Fatalf("Device(mac) = %q", Device(mac))
	}
}
