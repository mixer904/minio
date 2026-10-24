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

package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/tinylib/msgp/msgp"
)

// TestReadPartsEmptyPathList covers the trace path the health decorator builds
// before it delegates.
//
// ReadParts took partMetaPaths[0] unconditionally, so a caller passing no paths
// at all indexed an empty slice. xlStorage.ReadParts itself handles an empty
// list perfectly well - it returns an empty result - so the panic came purely
// from the metrics bookkeeping wrapped around it.
func TestReadPartsEmptyPathList(t *testing.T) {
	disk, _, err := newXLStorageTestSetup(t)
	if err != nil {
		t.Fatalf("unable to create test setup: %v", err)
	}
	if err := disk.MakeVol(t.Context(), "foo"); err != nil {
		t.Fatalf("MakeVol: %v", err)
	}

	parts, err := disk.ReadParts(t.Context(), "foo")
	if err != nil {
		t.Fatalf("empty part list returned an error: %v", err)
	}
	if len(parts) != 0 {
		t.Fatalf("empty part list returned %d parts, want 0", len(parts))
	}
}

// TestReadPartsEmptyPathListDoesNotLeakKeepAlive covers what the panic actually
// cost a running node.
//
// ReadPartsHandler calls keepHTTPResponseAlive before it calls ReadParts. That
// helper spawns a goroutine whose only exit is receiving from the channel that
// done() writes. Panicking in between skips both done(err) and done(nil), so
// net/http recovering the handler goroutine still leaves the keep-alive
// goroutine and its 10-second ticker parked forever - one per request, driven
// by a request body an authenticated peer fully controls.
func TestReadPartsEmptyPathListDoesNotLeakKeepAlive(t *testing.T) {
	newStorageRESTHTTPServerClient(t)
	server := &storageRESTServer{endpoint: globalLocalSetDrives[0][0][0].Endpoint()}

	// Recovering here mirrors net/http, which recovers a panicking handler and
	// keeps serving. That recovery is exactly why the defect is a leak rather
	// than a crash: the process survives, and the parked goroutine survives
	// with it.
	call := func() (panicked bool) {
		defer func() { panicked = recover() != nil }()

		var body bytes.Buffer
		if err := msgp.Encode(&body, &ReadPartsReq{}); err != nil {
			t.Fatalf("encoding an empty ReadPartsReq: %v", err)
		}
		u := "/?" + url.Values{storageRESTVolume: []string{"foo"}}.Encode()
		req := httptest.NewRequest(http.MethodPost, u, bytes.NewReader(body.Bytes()))
		req.Header.Set("Authorization", "Bearer "+globalNodeAuthToken)
		req.Header.Set("X-Minio-Time", strconv.FormatInt(time.Now().UnixNano(), 10))
		w := httptest.NewRecorder()
		server.ReadPartsHandler(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("empty-path ReadParts: status %d, body %q", w.Code, w.Body.String())
		}
		return false
	}

	baseline := settleKeepAliveGoroutines(0, time.Second)

	const requests = 20
	panicked := false
	for range requests {
		panicked = call() || panicked
	}
	if panicked {
		t.Error("ReadPartsHandler panicked on an empty path list")
	}

	// One leak per request, so anything above the baseline is the defect.
	if got := settleKeepAliveGoroutines(baseline, 5*time.Second); got > baseline {
		t.Errorf("%d empty-path requests left %d keepHTTPResponseAlive goroutines parked "+
			"(baseline %d) - done() is never reached, so each request strands one "+
			"goroutine and its ticker", requests, got, baseline)
	}
}

// settleKeepAliveGoroutines counts goroutines parked inside
// keepHTTPResponseAlive, sampling until the count falls to at most limit or the
// deadline passes, and returns the final sample.
//
// Counting this one frame rather than runtime.NumGoroutine keeps the assertion
// meaningful when the whole cmd package runs and unrelated background
// goroutines come and go.
func settleKeepAliveGoroutines(limit int, wait time.Duration) int {
	deadline := time.Now().Add(wait)
	for {
		n := countGoroutinesCreatedBy("github.com/minio/minio/cmd.keepHTTPResponseAlive")
		if n <= limit || !time.Now().Before(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// countGoroutinesCreatedBy reports how many live goroutines were started by fn.
// Matching the "created by" line counts each goroutine once; the function name
// on its own also appears in the running frame of every goroutine it started.
func countGoroutinesCreatedBy(fn string) int {
	needle := []byte("created by " + fn)
	buf := make([]byte, 1<<20)
	for {
		if n := runtime.Stack(buf, true); n < len(buf) {
			return bytes.Count(buf[:n], needle)
		}
		buf = make([]byte, 2*len(buf))
	}
}
