// Copyright (c) 2015-2026 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
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

package hash

import (
	"bytes"
	"testing"
)

// TestChecksumAddPart checks that merging part checksums reproduces the
// checksum of the concatenated content, including when parts are empty.
// A zero length first part used to leave the accumulator unseeded, which left
// multipart objects with an empty checksum.
func TestChecksumAddPart(t *testing.T) {
	empty := []byte{}
	a := bytes.Repeat([]byte("a"), 1024)
	b := bytes.Repeat([]byte("b"), 7)

	layouts := []struct {
		name  string
		parts [][]byte
	}{
		{"single-empty", [][]byte{empty}},
		{"empty-first", [][]byte{empty, a, b}},
		{"empty-middle", [][]byte{a, empty, b}},
		{"empty-last", [][]byte{a, b, empty}},
		{"all-empty", [][]byte{empty, empty}},
		{"no-empty", [][]byte{a, b}},
		{"single", [][]byte{a}},
	}

	for _, typ := range []ChecksumType{ChecksumCRC32, ChecksumCRC32C, ChecksumCRC64NVME} {
		for _, l := range layouts {
			t.Run(typ.String()+"/"+l.name, func(t *testing.T) {
				var merged Checksum
				merged.Type = typ | ChecksumMultipart | ChecksumIncludesMultipart

				var full []byte
				for i, p := range l.parts {
					part := NewChecksumFromData(typ, p)
					if part == nil {
						t.Fatalf("unable to compute part %d checksum", i+1)
					}
					if err := merged.AddPart(*part, int64(len(p))); err != nil {
						t.Fatalf("AddPart(%d): %v", i+1, err)
					}
					full = append(full, p...)
				}

				want := NewChecksumFromData(typ, full)
				if want == nil {
					t.Fatal("unable to compute expected checksum")
				}
				if merged.Encoded != want.Encoded {
					t.Fatalf("merged checksum = %q, want %q", merged.Encoded, want.Encoded)
				}
				if !merged.Valid() {
					t.Fatalf("merged checksum is not valid: %+v", merged)
				}
			})
		}
	}
}

// TestChecksumAddPartTypeMismatch checks that a mismatched part type is
// reported even when the part is empty. The zero size shortcut used to hide it.
func TestChecksumAddPartTypeMismatch(t *testing.T) {
	var merged Checksum
	merged.Type = ChecksumCRC32 | ChecksumMultipart

	other := NewChecksumFromData(ChecksumCRC32C, []byte("x"))
	if other == nil {
		t.Fatal("unable to compute part checksum")
	}
	if err := merged.AddPart(*other, 0); err == nil {
		t.Fatal("expected a type mismatch error for a zero sized part of the wrong type")
	}
	if err := merged.AddPart(*other, 1); err == nil {
		t.Fatal("expected a type mismatch error for a non-empty part of the wrong type")
	}
}

// TestChecksumAddPartUnmergeable checks that non-CRC types are still refused.
func TestChecksumAddPartUnmergeable(t *testing.T) {
	var merged Checksum
	merged.Type = ChecksumSHA256 | ChecksumMultipart

	other := NewChecksumFromData(ChecksumSHA256, []byte("x"))
	if other == nil {
		t.Fatal("unable to compute part checksum")
	}
	if err := merged.AddPart(*other, 1); err == nil {
		t.Fatal("expected SHA256 to be refused as unmergeable")
	}
}
