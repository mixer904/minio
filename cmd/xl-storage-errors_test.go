// Copyright (c) 2015-2021 MinIO, Inc.
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

package cmd

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestSysErrors(t *testing.T) {
	pathErr := &os.PathError{Err: syscall.ENAMETOOLONG}
	ok := isSysErrTooLong(pathErr)
	if !ok {
		t.Fatalf("Unexpected error expecting %s", syscall.ENAMETOOLONG)
	}
	pathErr = &os.PathError{Err: syscall.ENOTDIR}
	ok = isSysErrNotDir(pathErr)
	if !ok {
		t.Fatalf("Unexpected error expecting %s", syscall.ENOTDIR)
	}
	if runtime.GOOS != globalWindowsOSName {
		pathErr = &os.PathError{Err: syscall.ENOTEMPTY}
		ok = isSysErrNotEmpty(pathErr)
		if !ok {
			t.Fatalf("Unexpected error expecting %s", syscall.ENOTEMPTY)
		}
	} else {
		pathErr = &os.PathError{Err: syscall.Errno(0x91)}
		ok = isSysErrNotEmpty(pathErr)
		if !ok {
			t.Fatal("Unexpected error expecting 0x91")
		}
	}
	if runtime.GOOS == globalWindowsOSName {
		pathErr = &os.PathError{Err: syscall.Errno(0x03)}
		ok = isSysErrPathNotFound(pathErr)
		if !ok {
			t.Fatal("Unexpected error expecting 0x03")
		}
	}
}

// TestCommitMetaMissingMapping verifies the RPC and object-layer mapping
// of errCommitMetaMissing, returned when the staged xl.meta disappears
// between the data rename and the meta-file commit in RenameData().
func TestCommitMetaMissingMapping(t *testing.T) {
	// The sentinel must survive a storage REST/grid round-trip; errors
	// cross the wire as strings and are remapped by toStorageErr().
	if got := toStorageErr(errors.New(errCommitMetaMissing.Error())); !errors.Is(got, errCommitMetaMissing) {
		t.Fatalf("expected %s, got %s", errCommitMetaMissing, got)
	}
	if got := toStorageErr(errCommitMetaMissing); got != errCommitMetaMissing {
		t.Fatalf("expected %s, got %s", errCommitMetaMissing, got)
	}

	// toObjectErr() must map the sentinel to InsufficientWriteQuorum
	// (503), never to ObjectNotFound (404).
	err := toObjectErr(errCommitMetaMissing, "bucket", "object")
	qerr, ok := err.(InsufficientWriteQuorum)
	if !ok {
		t.Fatalf("expected InsufficientWriteQuorum, got %s (%T)", err, err)
	}
	if qerr.Bucket != "bucket" || qerr.Object != "object" {
		t.Fatalf("unexpected bucket/object %s/%s", qerr.Bucket, qerr.Object)
	}
}
