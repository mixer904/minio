// Copyright 2026 PGSTY contributors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectDefaultConfigDir(t *testing.T) {
	tests := []struct {
		name        string
		createSilo  bool
		createMinio bool
		wantDir     string
		wantState   defaultConfigDirState
	}{
		{name: "new install", wantDir: defaultSiloConfigDir, wantState: defaultConfigDirNew},
		{name: "silo exists", createSilo: true, wantDir: defaultSiloConfigDir, wantState: defaultConfigDirNew},
		{name: "legacy only", createMinio: true, wantDir: legacyMinioConfigDir, wantState: defaultConfigDirLegacy},
		{name: "both exist", createSilo: true, createMinio: true, wantDir: defaultSiloConfigDir, wantState: defaultConfigDirAmbiguous},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for name, create := range map[string]bool{
				defaultSiloConfigDir: tt.createSilo,
				legacyMinioConfigDir: tt.createMinio,
			} {
				if create {
					if err := os.Mkdir(filepath.Join(home, name), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}

			gotDir, gotState := selectDefaultConfigDir(home)
			if want := filepath.Join(home, tt.wantDir); gotDir != want {
				t.Fatalf("directory = %q, want %q", gotDir, want)
			}
			if gotState != tt.wantState {
				t.Fatalf("state = %d, want %d", gotState, tt.wantState)
			}
		})
	}
}
