// Copyright 2026 PGSTY contributors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package subnet

import (
	"errors"
	"os"
	"testing"
)

func TestSiloSubnetIsPermanentlyDisabled(t *testing.T) {
	cfg := Config{License: "legacy-license", APIKey: "legacy-key", BaseURL: "https://example.invalid"}
	if cfg.Registered() {
		t.Fatal("legacy SUBNET credentials must not register a Silo deployment")
	}

	for _, name := range []string{"CONSOLE_SUBNET_LICENSE", "CONSOLE_SUBNET_API_KEY", "CONSOLE_SUBNET_PROXY", "CONSOLE_SUBNET_URL"} {
		t.Setenv(name, "must-be-cleared")
	}
	cfg.ApplyEnv()
	for _, name := range []string{"CONSOLE_SUBNET_LICENSE", "CONSOLE_SUBNET_API_KEY", "CONSOLE_SUBNET_PROXY", "CONSOLE_SUBNET_URL"} {
		if _, ok := os.LookupEnv(name); ok {
			t.Fatalf("%s remains set", name)
		}
	}

	if _, err := cfg.Upload("https://example.invalid", "health.json", nil); !errors.Is(err, errSiloSubnetDisabled) {
		t.Fatalf("Upload error = %v, want %v", err, errSiloSubnetDisabled)
	}
	if _, err := cfg.Post("https://example.invalid", struct{}{}); !errors.Is(err, errSiloSubnetDisabled) {
		t.Fatalf("Post error = %v, want %v", err, errSiloSubnetDisabled)
	}
	if _, err := cfg.submitPost(nil); !errors.Is(err, errSiloSubnetDisabled) {
		t.Fatalf("submitPost error = %v, want %v", err, errSiloSubnetDisabled)
	}
}
