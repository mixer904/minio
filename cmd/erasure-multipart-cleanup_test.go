// Copyright (c) 2026 clown15
//
// This file is part of Silo Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

func TestExtractModTimeFromUploadIDDir(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	nano := now.UnixNano()
	uuid := "d16ad42a-6fe5-4a3b-8092-69983142238f"

	testCases := []struct {
		name        string
		dirName     string
		expectedMod time.Time
		wantZero    bool
	}{
		{
			name:        "Modern Plaintext Format (<UUID>x<UnixNano>)",
			dirName:     fmt.Sprintf("%sx%d", uuid, nano),
			expectedMod: time.Unix(0, nano),
			wantZero:    false,
		},
		{
			name:        "Legacy Format Without Timestamp (<UUID>)",
			dirName:     uuid,
			expectedMod: time.Time{},
			wantZero:    true,
		},
		{
			name:        "Base64 Encoded Format",
			dirName:     base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%sx%d", uuid, nano))),
			expectedMod: time.Unix(0, nano),
			wantZero:    false,
		},
		{
			name:        "Malformed Suffix (<UUID>xinvalid)",
			dirName:     fmt.Sprintf("%sxnotanumber", uuid),
			expectedMod: time.Time{},
			wantZero:    true,
		},
		{
			name:        "Empty Directory Name",
			dirName:     "",
			expectedMod: time.Time{},
			wantZero:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractModTimeFromUploadIDDir(tc.dirName)
			if tc.wantZero {
				if !got.IsZero() {
					t.Fatalf("expected zero time for %q, got %v", tc.dirName, got)
				}
				return
			}
			if !got.Equal(tc.expectedMod) {
				t.Fatalf("extractModTimeFromUploadIDDir(%q) = %v; want %v", tc.dirName, got, tc.expectedMod)
			}
		})
	}
}
