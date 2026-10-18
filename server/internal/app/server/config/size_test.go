package config

import "testing"

func TestParseByteSize(t *testing.T) {
	cases := map[string]int64{
		"1024":  1024,
		"500MB": 500 << 20,
		"50GB":  50 << 30,
		"50 gb": 50 << 30,
		"1.5TB": 3 << 39,
		"10kb":  10 << 10,
		"7b":    7,
	}
	for in, want := range cases {
		got, err := ParseByteSize(in)
		if err != nil {
			t.Errorf("ParseByteSize(%q): %v", in, err)
			continue
		}
		if int64(got) != want {
			t.Errorf("ParseByteSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseByteSizeRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "0", "GB", "10XB", "-5GB", "abc", "2g", "50GiB"} {
		if _, err := ParseByteSize(in); err == nil {
			t.Errorf("ParseByteSize(%q) = nil error, want error", in)
		}
	}
}

func TestByteSizeString(t *testing.T) {
	cases := map[ByteSize]string{
		512:      "512 B",
		50 << 30: "50.0 GB",
		1536:     "1.5 KB",
		3 << 39:  "1.5 TB",
	}
	for in, want := range cases {
		if got := in.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int64(in), got, want)
		}
	}
}
