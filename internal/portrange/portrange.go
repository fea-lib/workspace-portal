package portrange

import (
	"fmt"
	"strconv"
	"strings"
)

// PortRange is a [lo, hi] port pair that unmarshals from "lo-hi" strings in both
// YAML and environment variables (e.g. "4100-4199"). Implementing
// encoding.TextUnmarshaler means both yaml.v3 and caarlos0/env pick it up
// automatically — no custom parsing code needed at the call site.
type PortRange [2]int

func (p *PortRange) UnmarshalText(text []byte) error {
	parts := strings.SplitN(string(text), "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("port range must be in lo-hi format, got %q", string(text))
	}
	lo, err1 := strconv.Atoi(parts[0])
	hi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return fmt.Errorf("invalid port range %q", string(text))
	}
	*p = PortRange{lo, hi}
	return nil
}
