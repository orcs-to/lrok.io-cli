package main

import "testing"

// TestSplitBasicAuthArg covers the client-side validation we run before we
// even dial the edge — gives users a fast, local error for fat-fingered creds.
func TestSplitBasicAuthArg(t *testing.T) {
	cases := []struct {
		in       string
		user     string
		pass     string
		ok       bool
	}{
		{"alice:s3cret", "alice", "s3cret", true},
		{"alice:", "alice", "", true},        // ok shape; main() rejects empty pass separately
		{":s3cret", "", "s3cret", true},      // ok shape; main() rejects empty user separately
		{"no-colon", "", "", false},
		{"", "", "", false},
		{"user:pass:with:colons", "user", "pass:with:colons", true}, // RFC 7617: split first colon
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			u, p, ok := splitBasicAuthArg(tc.in)
			if ok != tc.ok || u != tc.user || p != tc.pass {
				t.Errorf("splitBasicAuthArg(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tc.in, u, p, ok, tc.user, tc.pass, tc.ok)
			}
		})
	}
}
