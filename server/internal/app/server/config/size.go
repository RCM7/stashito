package config

import (
	"fmt"
	"strconv"
	"strings"
)

type ByteSize int64

var sizeUnits = map[string]int64{
	"":   1,
	"b":  1,
	"kb": 1 << 10,
	"mb": 1 << 20,
	"gb": 1 << 30,
	"tb": 1 << 40,
}

func ParseByteSize(s string) (ByteSize, error) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	number, unit := s[:i], strings.ToLower(strings.TrimSpace(s[i:]))
	if number == "" {
		return 0, fmt.Errorf("size %q must start with a number", s)
	}
	mult, ok := sizeUnits[unit]
	if !ok {
		return 0, fmt.Errorf("size %q has unknown unit %q: use KB, MB, GB or TB", s, unit)
	}
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q is not a number", s)
	}
	bytes := int64(value * float64(mult))
	if bytes <= 0 {
		return 0, fmt.Errorf("size %q must be greater than zero", s)
	}
	return ByteSize(bytes), nil
}

func (b *ByteSize) EnvDecode(val string) error {
	if strings.TrimSpace(val) == "" {
		*b = 0
		return nil
	}
	parsed, err := ParseByteSize(val)
	if err != nil {
		return err
	}
	*b = parsed
	return nil
}

func (b ByteSize) String() string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(b)
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", int64(b))
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}
