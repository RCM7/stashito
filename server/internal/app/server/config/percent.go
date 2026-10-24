package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Percent int

func ParsePercent(s string) (Percent, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	value, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("percent %q must be a whole number, optionally followed by %%", s)
	}
	if value < 0 || value >= 100 {
		return 0, fmt.Errorf("percent %q must be between 0 and 99", s)
	}
	return Percent(value), nil
}

func (p *Percent) EnvDecode(val string) error {
	parsed, err := ParsePercent(val)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

func (p Percent) Of(total int64) int64 {
	return total * int64(p) / 100
}
