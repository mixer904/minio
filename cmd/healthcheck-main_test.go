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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	xhttp "github.com/minio/minio/internal/http"
)

func TestHealthcheckTarget(t *testing.T) {
	plainDir := t.TempDir()

	tlsDir := t.TempDir()
	for _, name := range []string{publicCertFile, privateKeyFile} {
		if err := os.WriteFile(filepath.Join(tlsDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A lone public.crt without its key must not flip the scheme.
	halfDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(halfDir, publicCertFile), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		rawURL   string
		address  string
		certsDir string
		want     string
		wantErr  bool
	}{
		{name: "default address", address: ":9000", certsDir: plainDir, want: "http://127.0.0.1:9000"},
		{name: "explicit host", address: "10.0.0.7:9010", certsDir: plainDir, want: "http://10.0.0.7:9010"},
		{name: "ipv6 address", address: "[::1]:9000", certsDir: plainDir, want: "http://[::1]:9000"},
		{name: "ipv6 zone is escaped", address: "[fe80::1%eth0]:9000", certsDir: plainDir, want: "http://[fe80::1%25eth0]:9000"},
		{name: "ipv6 zone in url", rawURL: "http://[fe80::1%25eth0]:9000", certsDir: plainDir, want: "http://[fe80::1%25eth0]:9000"},
		{name: "tls certs present", address: ":9000", certsDir: tlsDir, want: "https://127.0.0.1:9000"},
		{name: "cert without key stays http", address: ":9000", certsDir: halfDir, want: "http://127.0.0.1:9000"},
		{name: "url override wins", rawURL: "https://silo.internal:9000", address: ":9000", certsDir: plainDir, want: "https://silo.internal:9000"},
		{name: "url path is dropped", rawURL: "http://silo.internal:9000/minio/health/live", address: ":9000", certsDir: plainDir, want: "http://silo.internal:9000"},
		{name: "address without port", address: "localhost", certsDir: plainDir, wantErr: true},
		{name: "url without scheme", rawURL: "silo.internal:9000", certsDir: plainDir, wantErr: true},
		{name: "url with bad scheme", rawURL: "ftp://silo.internal:9000", certsDir: plainDir, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := healthcheckTarget(test.rawURL, test.address, test.certsDir)
			if (err != nil) != test.wantErr {
				t.Fatalf("healthcheckTarget() error = %v, wantErr = %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("healthcheckTarget() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProbeHealthChecksAndVerdicts(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/minio/health/live", "/minio/health/ready":
			w.WriteHeader(http.StatusOK)
		case "/minio/health/cluster":
			if r.URL.Query().Get("maintenance") == "true" {
				w.Header().Set(xhttp.MinIOWriteQuorum, "3")
				w.Header().Set(xhttp.MinIOHealingDrives, "2")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			w.Header().Set(xhttp.MinIOServerStatus, "iam-offline")
			w.Header().Set(xhttp.MinIOWriteQuorum, "3")
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/minio/health/cluster/read":
			w.Header().Set(xhttp.MinIOReadQuorum, "2")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res := probeHealth(srv.URL, "live", false, time.Second)
	if !res.Healthy || res.StatusCode != http.StatusOK {
		t.Fatalf("live: expected healthy 200, got %+v", res)
	}
	if gotPath != "/minio/health/live" {
		t.Fatalf("live: probed %q", gotPath)
	}
	if gotAuth != "" {
		t.Fatalf("probe must be anonymous, sent Authorization %q", gotAuth)
	}

	res = probeHealth(srv.URL, "cluster", false, time.Second)
	if res.Healthy || res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("cluster: expected unhealthy 503, got %+v", res)
	}
	if res.ServerStatus != "iam-offline" || res.WriteQuorum != "3" {
		t.Fatalf("cluster: headers not decoded, got %+v", res)
	}
	if gotQuery != "" {
		t.Fatalf("cluster without --maintenance sent query %q", gotQuery)
	}

	res = probeHealth(srv.URL, "cluster", true, time.Second)
	if res.Healthy || res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("cluster maintenance: expected 412, got %+v", res)
	}
	if res.HealingDrives != "2" {
		t.Fatalf("cluster maintenance: headers not decoded, got %+v", res)
	}
	if gotQuery != "maintenance=true" {
		t.Fatalf("cluster --maintenance sent query %q", gotQuery)
	}

	res = probeHealth(srv.URL, "cluster-read", false, time.Second)
	if !res.Healthy || res.ReadQuorum != "2" {
		t.Fatalf("cluster-read: expected healthy with read quorum, got %+v", res)
	}
	if gotPath != "/minio/health/cluster/read" {
		t.Fatalf("cluster-read: probed %q", gotPath)
	}
}

func TestProbeHealthTLSSkipsVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := probeHealth(srv.URL, "live", false, time.Second)
	if !res.Healthy {
		t.Fatalf("self-signed TLS probe must succeed, got %+v", res)
	}
}

func TestProbeHealthUnreachableAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	deadURL := srv.URL
	srv.Close()

	res := probeHealth(deadURL, "live", false, time.Second)
	if res.Healthy || res.Err == "" {
		t.Fatalf("probe of a closed server must report unreachable, got %+v", res)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	res = probeHealth(slow.URL, "live", false, 50*time.Millisecond)
	if res.Healthy || res.Err == "" {
		t.Fatalf("probe past its deadline must fail, got %+v", res)
	}
}

func TestHealthcheckResultLine(t *testing.T) {
	tests := []struct {
		res  healthcheckResult
		want string
	}{
		{healthcheckResult{Check: "live", Healthy: true, StatusCode: 200, DurationMS: 2}, "live: ok (200, 2ms)"},
		{healthcheckResult{Check: "cluster", StatusCode: 503, ServerStatus: "iam-offline", WriteQuorum: "3", HealingDrives: "2"}, "cluster: unhealthy (503) server-status=iam-offline write-quorum=3 healing-drives=2"},
		{healthcheckResult{Check: "cluster", StatusCode: 412, WriteQuorum: "3"}, "cluster: not safe for maintenance (412) write-quorum=3"},
		{healthcheckResult{Check: "ready", Err: "connection refused"}, "ready: unreachable (connection refused)"},
	}
	for _, test := range tests {
		if got := test.res.line(); got != test.want {
			t.Fatalf("line() = %q, want %q", got, test.want)
		}
	}
}
