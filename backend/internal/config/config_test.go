package config

import "testing"

func TestValidateBasePath(t *testing.T) {
	good := []string{"/", "/secret/", "/a.b_c~d-e/", "/8f3a9c1d2e5b7a04/"}
	for _, b := range good {
		if err := validateBasePath(b); err != nil {
			t.Errorf("validateBasePath(%q) = %v, want nil", b, err)
		}
	}
	// These would make http.ServeMux panic when registering route patterns.
	bad := []string{"/../", "/./", "/a b/", "/a{b}/", "/x/../y/", "/foo bar/", "/a\tb/"}
	for _, b := range bad {
		if err := validateBasePath(b); err == nil {
			t.Errorf("validateBasePath(%q) = nil, want error", b)
		}
	}
}

func TestLoadValidatesBasePath(t *testing.T) {
	if _, err := Load([]string{"-base-path", ".."}); err == nil {
		t.Error("Load with -base-path .. should error, not later panic")
	}
	if _, err := Load([]string{"-base-path", "good-path"}); err != nil {
		t.Errorf("Load with -base-path good-path: %v", err)
	}
	// Root (default) must be accepted.
	if _, err := Load(nil); err != nil {
		t.Errorf("Load with defaults: %v", err)
	}
}
