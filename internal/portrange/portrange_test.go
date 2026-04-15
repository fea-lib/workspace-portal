package portrange

import "testing"

func TestUnmarshalText(t *testing.T) {
	tests := []struct {
		input   string
		want    PortRange
		wantErr bool
	}{
		{"4100-4199", PortRange{4100, 4199}, false},
		{"80-80", PortRange{80, 80}, false},
		{"0-65535", PortRange{0, 65535}, false},
		{"4100", PortRange{}, true},            // missing separator
		{"abc-4199", PortRange{}, true},        // non-numeric lo
		{"4100-xyz", PortRange{}, true},        // non-numeric hi
		{"", PortRange{}, true},                // empty string
		{"4100-4199-extra", PortRange{}, true}, // too many parts
	}

	for _, tc := range tests {
		var p PortRange
		err := p.UnmarshalText([]byte(tc.input))
		if tc.wantErr {
			if err == nil {
				t.Errorf("input %q: expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %q: unexpected error: %v", tc.input, err)
			continue
		}
		if p != tc.want {
			t.Errorf("input %q: got %v, want %v", tc.input, p, tc.want)
		}
	}
}
