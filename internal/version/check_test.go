package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", +1},
		{"v0.1.0", "v0.1.0", 0},
		{"0.1.0", "v0.1.0", 0}, // leading "v" is optional
		{"v0.1.1", "v0.1.0", +1},
		{"v0.10.0", "v0.9.0", +1}, // not just string compare
		{"v1.0.0", "v0.99.99", +1},
		{"v0.1.0-rc1", "v0.1.0", 0}, // pre-release suffix ignored
		{"", "v0.1.0", -1},
		{"v0.1.0", "", +1},
		{"dev", "v0.1.0", -1}, // unparseable -> 0 components -> "older"
	}
	for _, c := range cases {
		got := Compare(c.a, c.b)
		if got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSplitVersion(t *testing.T) {
	got := splitVersion("v1.2.3")
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("splitVersion(v1.2.3) len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("part %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestInstallHint(t *testing.T) {
	// Just sanity-check that it returns something non-empty for the
	// current platform so users always see *some* upgrade guidance.
	if got := InstallHint(); got == "" {
		t.Error("InstallHint() returned empty string")
	}
}

func TestAssetURL(t *testing.T) {
	if got := AssetURL(""); got != "" {
		t.Errorf("AssetURL(\"\") = %q, want empty", got)
	}
	if got := AssetURL("v0.1.0"); got == "" {
		t.Error("AssetURL(v0.1.0) returned empty")
	}
}
