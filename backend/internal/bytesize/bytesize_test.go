package bytesize

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{-5, "0 B"},
		{1023, "1023 B"},
		{1536, "1.5 KiB"},
		{10 * 1 << 30, "10.0 GiB"},
	}
	for _, tc := range cases {
		if got := Format(tc.bytes); got != tc.want {
			t.Fatalf("Format(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}
