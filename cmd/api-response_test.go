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
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klauspost/compress/gzhttp"
	xhttp "github.com/minio/minio/internal/http"
)

// Tests object location.
func TestObjectLocation(t *testing.T) {
	testCases := []struct {
		request          *http.Request
		bucket, object   string
		domains          []string
		expectedLocation string
	}{
		// Server binding to localhost IP with https.
		{
			request: &http.Request{
				Host: "127.0.0.1:9000",
				Header: map[string][]string{
					"X-Forwarded-Scheme": {httpScheme},
				},
			},
			bucket:           "testbucket1",
			object:           "test/1.txt",
			expectedLocation: "http://127.0.0.1:9000/testbucket1/test/1.txt",
		},
		{
			request: &http.Request{
				Host: "127.0.0.1:9000",
				Header: map[string][]string{
					"X-Forwarded-Scheme": {httpsScheme},
				},
			},
			bucket:           "testbucket1",
			object:           "test/1.txt",
			expectedLocation: "https://127.0.0.1:9000/testbucket1/test/1.txt",
		},
		// Server binding to fqdn.
		{
			request: &http.Request{
				Host: "s3.mybucket.org",
				Header: map[string][]string{
					"X-Forwarded-Scheme": {httpScheme},
				},
			},
			bucket:           "mybucket",
			object:           "test/1.txt",
			expectedLocation: "http://s3.mybucket.org/mybucket/test/1.txt",
		},
		// Server binding to fqdn.
		{
			request: &http.Request{
				Host:   "mys3.mybucket.org",
				Header: map[string][]string{},
			},
			bucket:           "mybucket",
			object:           "test/1.txt",
			expectedLocation: "http://mys3.mybucket.org/mybucket/test/1.txt",
		},
		// Server with virtual domain name.
		{
			request: &http.Request{
				Host:   "mybucket.mys3.bucket.org",
				Header: map[string][]string{},
			},
			domains:          []string{"mys3.bucket.org"},
			bucket:           "mybucket",
			object:           "test/1.txt",
			expectedLocation: "http://mybucket.mys3.bucket.org/test/1.txt",
		},
		{
			request: &http.Request{
				Host: "mybucket.mys3.bucket.org",
				Header: map[string][]string{
					"X-Forwarded-Scheme": {httpsScheme},
				},
			},
			domains:          []string{"mys3.bucket.org"},
			bucket:           "mybucket",
			object:           "test/1.txt",
			expectedLocation: "https://mybucket.mys3.bucket.org/test/1.txt",
		},
	}
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			gotLocation := getObjectLocation(testCase.request, testCase.domains, testCase.bucket, testCase.object)
			if testCase.expectedLocation != gotLocation {
				t.Errorf("expected %s, got %s", testCase.expectedLocation, gotLocation)
			}
		})
	}
}

// Tests getURLScheme function behavior.
func TestGetURLScheme(t *testing.T) {
	tls := false
	gotScheme := getURLScheme(tls)
	if gotScheme != httpScheme {
		t.Errorf("Expected %s, got %s", httpScheme, gotScheme)
	}
	tls = true
	gotScheme = getURLScheme(tls)
	if gotScheme != httpsScheme {
		t.Errorf("Expected %s, got %s", httpsScheme, gotScheme)
	}
}

type writeHeaderSpy struct {
	http.ResponseWriter
	codes []int
}

func (r *writeHeaderSpy) WriteHeader(code int) {
	r.codes = append(r.codes, code)
	r.ResponseWriter.WriteHeader(code)
}

func (r *writeHeaderSpy) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func TestTrackingResponseWriter(t *testing.T) {
	rw := httptest.NewRecorder()
	trw := &trackingResponseWriter{ResponseWriter: rw}
	trw.WriteHeader(299)
	if !trw.headerWritten {
		t.Fatal("headerWritten was not set by WriteHeader call")
	}

	_, err := trw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write unexpectedly failed: %v", err)
	}
	xhttp.Flush(trw)

	// Check that WriteHeader, Write, and Flush were called on the underlying response writer.
	resp := rw.Result()
	if resp.StatusCode != 299 {
		t.Fatalf("unexpected status: %v", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body failed: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("response body incorrect: %v", string(body))
	}
	if !rw.Flushed {
		t.Fatal("underlying ResponseRecorder was not flushed")
	}

	// Check that Unwrap works
	if trw.Unwrap() != rw {
		t.Fatalf("Unwrap returned wrong result: %v", trw.Unwrap())
	}
}

func TestTrackingResponseWriterWriteImplicitHeader(t *testing.T) {
	testCases := []struct {
		name string
		body []byte
	}{
		{name: "non-empty", body: []byte("hello")},
		{name: "empty", body: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rw := &writeHeaderSpy{ResponseWriter: rec}
			trw := &trackingResponseWriter{ResponseWriter: rw}

			n, err := trw.Write(testCase.body)
			if err != nil {
				t.Fatalf("Write unexpectedly failed: %v", err)
			}
			if n != len(testCase.body) {
				t.Fatalf("unexpected bytes written: got %d, want %d", n, len(testCase.body))
			}
			if !trw.headerWritten {
				t.Fatal("Write did not set headerWritten")
			}
			if len(rw.codes) != 1 || rw.codes[0] != http.StatusOK {
				t.Fatalf("unexpected WriteHeader calls: got %v, want [%d]", rw.codes, http.StatusOK)
			}
			if got := rec.Body.String(); got != string(testCase.body) {
				t.Fatalf("unexpected body: got %q, want %q", got, testCase.body)
			}
		})
	}
}

func TestTrackingResponseWriterFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &writeHeaderSpy{ResponseWriter: rec}
	trw := &trackingResponseWriter{ResponseWriter: rw}

	xhttp.Flush(trw)
	if !trw.headerWritten {
		t.Fatal("Flush did not set headerWritten")
	}
	if len(rw.codes) != 1 || rw.codes[0] != http.StatusOK {
		t.Fatalf("unexpected WriteHeader calls: got %v, want [%d]", rw.codes, http.StatusOK)
	}
	if !rec.Flushed {
		t.Fatal("underlying ResponseRecorder was not flushed")
	}
}

func TestTrackingResponseWriterFlushUnsupported(t *testing.T) {
	rw := struct{ http.ResponseWriter }{ResponseWriter: httptest.NewRecorder()}
	trw := &trackingResponseWriter{ResponseWriter: rw}

	trw.Flush()
	if trw.headerWritten {
		t.Fatal("unsupported Flush set headerWritten")
	}
}

func TestTrackingResponseWriterGzipStreaming(t *testing.T) {
	const (
		eventPayload = "event data"
		sentinel     = "<sentinel-error/>"
	)

	rw := httptest.NewRecorder()
	trw := &trackingResponseWriter{ResponseWriter: rw}
	var (
		committed bool
		writeErr  error
	)
	handler := gzipHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		setEventStreamHeaders(w)
		_, writeErr = w.Write([]byte(eventPayload))
		if writeErr != nil {
			return
		}
		xhttp.Flush(w)
		committed = headersAlreadyWritten(w)
		writeResponse(w, http.StatusInternalServerError, []byte(sentinel), mimeXML)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	handler.ServeHTTP(trw, req)

	if writeErr != nil {
		t.Fatalf("Write unexpectedly failed: %v", writeErr)
	}
	if !committed {
		t.Fatal("headersAlreadyWritten returned false after Write and Flush")
	}
	resp := rw.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("unexpected Content-Encoding: got %q, want %q", got, "gzip")
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("creating gzip reader failed: %v", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading gzip response body failed: %v", err)
	}
	if got := string(body); got != eventPayload {
		t.Fatalf("unexpected response body: got %q, want %q (sentinel %q must be suppressed)", got, eventPayload, sentinel)
	}
}

func TestHeadersAlreadyWritten(t *testing.T) {
	rw := httptest.NewRecorder()
	trw := &trackingResponseWriter{ResponseWriter: rw}

	if headersAlreadyWritten(trw) {
		t.Fatal("headers have not been written yet")
	}

	trw.WriteHeader(299)
	if !headersAlreadyWritten(trw) {
		t.Fatal("headers were written")
	}
}

func TestHeadersAlreadyWrittenWrapped(t *testing.T) {
	rw := httptest.NewRecorder()
	trw := &trackingResponseWriter{ResponseWriter: rw}
	wrap1 := &gzhttp.NoGzipResponseWriter{ResponseWriter: trw}
	wrap2 := &gzhttp.NoGzipResponseWriter{ResponseWriter: wrap1}

	if headersAlreadyWritten(wrap2) {
		t.Fatal("headers have not been written yet")
	}

	// Pin the current stack-wide 1xx limitation documented on trackingResponseWriter.
	wrap2.WriteHeader(http.StatusContinue)
	if !headersAlreadyWritten(wrap2) {
		t.Fatal("headers were written")
	}
}

func TestWriteResponseHeadersNotWritten(t *testing.T) {
	rw := httptest.NewRecorder()
	trw := &trackingResponseWriter{ResponseWriter: rw}

	writeResponse(trw, 299, []byte("hello"), "application/foo")

	resp := rw.Result()
	if resp.StatusCode != 299 {
		t.Fatal("response wasn't written")
	}
}

func TestWriteResponseHeadersWritten(t *testing.T) {
	rw := httptest.NewRecorder()
	rw.Code = -1
	trw := &trackingResponseWriter{ResponseWriter: rw, headerWritten: true}

	writeResponse(trw, 200, []byte("hello"), "application/foo")

	if rw.Code != -1 {
		t.Fatalf("response was written when it shouldn't have been (Code=%v)", rw.Code)
	}
}
