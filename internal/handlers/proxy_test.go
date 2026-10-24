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

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio/internal/config"
)

type headerTest struct {
	key      string // header key
	val      string // header val
	expected string // expected result
}

func TestGetScheme(t *testing.T) {
	headers := []headerTest{
		{xForwardedProto, "https", "https"},
		{xForwardedProto, "http", "http"},
		{xForwardedProto, "HTTP", "http"},
		{xForwardedScheme, "https", "https"},
		{xForwardedScheme, "http", "http"},
		{xForwardedScheme, "HTTP", "http"},
		{forwarded, `For="[2001:db8:cafe::17]:4711`, ""},                    // No proto
		{forwarded, `for=192.0.2.43, for=198.51.100.17;proto=https`, ""},    // Multiple params, will be empty.
		{forwarded, `for=172.32.10.15; proto=https;by=127.0.0.1;`, "https"}, // Space before proto
		{forwarded, `for=192.0.2.60;proto=http;by=203.0.113.43`, "http"},    // Multiple params
	}
	for _, v := range headers {
		req := &http.Request{
			Header: http.Header{
				v.key: []string{v.val},
			},
		}
		res := GetSourceScheme(req)
		if res != v.expected {
			t.Errorf("wrong header for %s: got %s want %s", v.key, res,
				v.expected)
		}
	}
}

// TestGetSourceIP - check the source ip of a request is parsed correctly.
func TestGetSourceIP(t *testing.T) {
	headers := []headerTest{
		{xForwardedFor, "8.8.8.8", "8.8.8.8"},                                         // Single address
		{xForwardedFor, "8.8.8.8, 8.8.4.4", "8.8.8.8"},                                // Multiple
		{xForwardedFor, "", ""},                                                       // None
		{xRealIP, "8.8.8.8", "8.8.8.8"},                                               // Single address
		{xRealIP, "[2001:db8:cafe::17]:4711", "[2001:db8:cafe::17]"},                  // IPv6 address
		{xRealIP, "", ""},                                                             // None
		{forwarded, `for="_gazonk"`, "_gazonk"},                                       // Hostname
		{forwarded, `For="[2001:db8:cafe::17]:4711`, `[2001:db8:cafe::17]`},           // IPv6 address
		{forwarded, `for=192.0.2.60;proto=http;by=203.0.113.43`, `192.0.2.60`},        // Multiple params
		{forwarded, `for=192.0.2.43, for=198.51.100.17`, "192.0.2.43"},                // Multiple params
		{forwarded, `for="workstation.local",for=198.51.100.17`, "workstation.local"}, // Hostname
	}

	for _, v := range headers {
		req := &http.Request{
			Header: http.Header{
				v.key: []string{v.val},
			},
		}
		res := GetSourceIP(req)
		if res != v.expected {
			t.Errorf("wrong header for %s: got %s want %s", v.key, res,
				v.expected)
		}
	}
}

// withSourceIPTrust installs the trust policy the given EnvTrustedProxies value
// would produce and restores the previous one when the test ends.
func withSourceIPTrust(t *testing.T, proxies string) {
	t.Helper()

	policy, prefixes, err := lookupSourceIPTrust(proxies)
	if err != nil {
		t.Fatalf("%s=%q: unexpected error: %v", EnvTrustedProxies, proxies, err)
	}

	prevPolicy, prevProxies := sourceIPPolicy, trustedProxies
	sourceIPPolicy, trustedProxies = policy, prefixes
	t.Cleanup(func() {
		sourceIPPolicy, trustedProxies = prevPolicy, prevProxies
	})
}

func requestFrom(peer string, header http.Header) *http.Request {
	if header == nil {
		header = http.Header{}
	}
	return &http.Request{RemoteAddr: peer, Header: header}
}

// Upstream's test, unchanged: _MINIO_API_XFF_HEADER keeps its original meaning,
// suppressing X-Forwarded-For alone and leaving X-Real-IP to answer instead.
// That behavior is exactly why it is not the trust switch - see the test below.
func TestXFFDisabled(t *testing.T) {
	req := &http.Request{
		Header: http.Header{
			xForwardedFor: []string{"8.8.8.8"},
			xRealIP:       []string{"1.1.1.1"},
		},
	}
	// When X-Forwarded-For and X-Real-IP headers are both present, X-Forwarded-For takes precedence.
	res := GetSourceIP(req)
	if res != "8.8.8.8" {
		t.Errorf("wrong header, xff takes precedence: got %s, want: 8.8.8.8", res)
	}

	// When explicitly disabled, the XFF header is ignored and X-Real-IP is used.
	enableXFFHeader = false
	defer func() {
		enableXFFHeader = true
	}()
	res = GetSourceIP(req)
	if res != "1.1.1.1" {
		t.Errorf("wrong header, xff is disabled: got %s, want: 1.1.1.1", res)
	}
}

// TestTrustNoProxiesIgnoresEveryForwardedHeader pins the property the setting
// exists for: an operator who turns forwarded-header trust off cannot be talked
// out of it by switching to another header. This is the guarantee
// _MINIO_API_XFF_HEADER=off does not provide, since suppressing one of three
// interchangeable headers only moves the answer to the next one.
func TestTrustNoProxiesIgnoresEveryForwardedHeader(t *testing.T) {
	const peer = "203.0.113.9:44321"

	forged := []headerTest{
		{xForwardedFor, "8.8.8.8", "203.0.113.9"},
		{xRealIP, "8.8.8.8", "203.0.113.9"},
		{forwarded, "for=8.8.8.8", "203.0.113.9"},
	}

	// Default mode honors every one of them, which is the behavior being opted
	// out of.
	withSourceIPTrust(t, "")
	for _, v := range forged {
		if res := GetSourceIP(requestFrom(peer, http.Header{v.key: []string{v.val}})); res != "8.8.8.8" {
			t.Errorf("%s: default mode should honor the header: got %s, want 8.8.8.8", v.key, res)
		}
	}

	withSourceIPTrust(t, TrustNoProxies)
	for _, v := range forged {
		res := GetSourceIP(requestFrom(peer, http.Header{v.key: []string{v.val}}))
		if res != v.expected {
			t.Errorf("%s should be ignored under %s: got %s, want %s", v.key, TrustNoProxies, res, v.expected)
		}
	}

	// All three at once, in case one merely shadows another.
	res := GetSourceIP(requestFrom(peer, http.Header{
		xForwardedFor: []string{"8.8.8.8"},
		xRealIP:       []string{"1.1.1.1"},
		forwarded:     []string{"for=9.9.9.9"},
	}))
	if res != "203.0.113.9" {
		t.Errorf("wrong source with all headers set and no proxies trusted: got %s, want 203.0.113.9", res)
	}
}

// TestTrustedProxiesResolution covers the allow-list mode, where the peer decides
// whether the request may speak for anyone else.
func TestTrustedProxiesResolution(t *testing.T) {
	withSourceIPTrust(t, "10.0.0.0/8, 192.0.2.7")

	tests := []struct {
		name   string
		peer   string
		header http.Header
		want   string
	}{{
		name:   "trusted proxy overwrote the header",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		// The stock nginx $proxy_add_x_forwarded_for recipe appends, so a client
		// that sends its own X-Forwarded-For keeps the left-most slot. Reading
		// right-to-left is what steps over it.
		name:   "client injected the left-most entry",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"9.9.9.9, 1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "two proxy hops after the client",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"9.9.9.9, 1.2.3.4, 10.0.0.5"}},
		want:   "1.2.3.4",
	}, {
		name:   "untrusted peer may not speak for anyone",
		peer:   "203.0.113.9:44321",
		header: http.Header{xForwardedFor: []string{"1.2.3.4"}},
		want:   "203.0.113.9",
	}, {
		name:   "untrusted peer cannot fall back to X-Real-IP either",
		peer:   "203.0.113.9:44321",
		header: http.Header{xRealIP: []string{"1.2.3.4"}},
		want:   "203.0.113.9",
	}, {
		name:   "untrusted peer cannot fall back to Forwarded either",
		peer:   "203.0.113.9:44321",
		header: http.Header{forwarded: []string{"for=1.2.3.4"}},
		want:   "203.0.113.9",
	}, {
		name:   "X-Real-IP from a trusted proxy",
		peer:   "192.0.2.7:9000",
		header: http.Header{xRealIP: []string{"1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		// FTP and SFTP reach the S3 layer over loopback and declare the session's
		// client with X-Forwarded-For.
		name:   "loopback front-end is trusted implicitly",
		peer:   "127.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "Forwarded chain is also read right-to-left",
		peer:   "10.0.0.1:9000",
		header: http.Header{forwarded: []string{"for=9.9.9.9, for=1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "unparseable chain entries are skipped",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"_gazonk, workstation.local, 1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "a chain of nothing but trusted hops falls back to the peer",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"10.0.0.2, 10.0.0.3"}},
		want:   "10.0.0.1",
	}, {
		name:   "X-Forwarded-For outranks X-Real-IP, as in the untrusted path",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"1.2.3.4"}, xRealIP: []string{"9.9.9.9"}},
		want:   "1.2.3.4",
	}, {
		name:   "IPv6 client keeps its bracketed form",
		peer:   "10.0.0.1:9000",
		header: http.Header{xForwardedFor: []string{"2001:db8::1"}},
		want:   "[2001:db8::1]",
	}, {
		name: "no headers at all",
		peer: "10.0.0.1:9000",
		want: "10.0.0.1",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := GetSourceIP(requestFrom(tt.peer, tt.header)); res != tt.want {
				t.Errorf("got %s, want %s", res, tt.want)
			}
		})
	}
}

// TestTrustedProxiesRepeatedHeaderLines covers proxies that add a second header
// line instead of extending the first. HAProxy's `option forwardfor` does this,
// and Header.Get would return only the client's line - putting the value the
// right-to-left walk exists to step over back in front of it.
func TestTrustedProxiesRepeatedHeaderLines(t *testing.T) {
	withSourceIPTrust(t, "10.0.0.0/8")

	tests := []struct {
		name   string
		header http.Header
		want   string
	}{{
		name:   "client line then proxy line",
		header: http.Header{xForwardedFor: []string{"9.9.9.9", "1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "client sends several lines",
		header: http.Header{xForwardedFor: []string{"9.9.9.9", "8.8.8.8", "1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "mixed: a comma chain and an added line",
		header: http.Header{xForwardedFor: []string{"9.9.9.9, 8.8.8.8", "1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		name:   "repeated Forwarded lines",
		header: http.Header{forwarded: []string{"for=9.9.9.9", "for=1.2.3.4"}},
		want:   "1.2.3.4",
	}, {
		// The proxy's own line is the later one; a client's cannot displace it.
		name:   "repeated X-Real-IP takes the last line",
		header: http.Header{xRealIP: []string{"9.9.9.9", "1.2.3.4"}},
		want:   "1.2.3.4",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := GetSourceIP(requestFrom("10.0.0.1:9000", tt.header)); res != tt.want {
				t.Errorf("got %s, want %s", res, tt.want)
			}
		})
	}
}

// The chain arrives from the network, so the work it costs has to be bounded and
// must not scale into the heap. A client sitting behind a trusted proxy can make
// this header as long as the server's header limit allows.
func TestTrustedProxiesBoundsChainWork(t *testing.T) {
	withSourceIPTrust(t, "10.0.0.0/8")

	// The budget runs out before reaching the left-hand entry, so the request
	// falls back to the peer - the same safe direction as an all-trusted chain.
	hops := []string{"1.2.3.4"}
	for range maxForwardedHops + 5 {
		hops = append(hops, "10.0.0.9")
	}
	res := GetSourceIP(requestFrom("10.0.0.1:9000", http.Header{
		xForwardedFor: []string{strings.Join(hops, ",")},
	}))
	if res != "10.0.0.1" {
		t.Errorf("an over-long chain should fall back to the peer: got %s, want 10.0.0.1", res)
	}

	// A chain just inside the budget must still resolve normally.
	shortEnough := []string{"1.2.3.4"}
	for range 10 {
		shortEnough = append(shortEnough, "10.0.0.9")
	}
	res = GetSourceIP(requestFrom("10.0.0.1:9000", http.Header{
		xForwardedFor: []string{strings.Join(shortEnough, ",")},
	}))
	if res != "1.2.3.4" {
		t.Errorf("a normal chain regressed: got %s, want 1.2.3.4", res)
	}

	// Scanning must happen in place. Splitting a megabyte of separators would
	// turn one request's header into tens of megabytes of slice headers.
	huge := []string{strings.Repeat(",", 1<<20)}
	if allocs := testing.AllocsPerRun(2, func() {
		untrustedHop(huge, canonicalSourceIP)
	}); allocs > 0 {
		t.Errorf("walking a chain allocated %v times; it must scan the header in place", allocs)
	}
}

// TestTrustedProxiesDoNotChangeUntrustedMode guards the release-window promise:
// with neither setting present, resolution is exactly what it was before.
func TestTrustedProxiesDoNotChangeUntrustedMode(t *testing.T) {
	withSourceIPTrust(t, "")

	tests := []struct {
		header http.Header
		want   string
	}{
		{http.Header{xForwardedFor: []string{"9.9.9.9, 1.2.3.4"}}, "9.9.9.9"},
		{http.Header{xForwardedFor: []string{"_gazonk"}}, "_gazonk"},
		{http.Header{xRealIP: []string{"workstation.local"}}, "workstation.local"},
		{http.Header{forwarded: []string{`for="[2001:db8:cafe::17]:4711`}}, "[2001:db8:cafe::17]"},
	}

	for _, tt := range tests {
		if res := GetSourceIP(requestFrom("203.0.113.9:44321", tt.header)); res != tt.want {
			t.Errorf("untrusted mode changed for %v: got %s, want %s", tt.header, res, tt.want)
		}
	}
}

// A chain entry naming loopback is an ordinary address, not a hop to step over.
// Stepping over it would discard the real answer for whatever a client put to
// its left - the same failure a too-broad allow-list produces.
func TestTrustedProxiesDoesNotSkipLoopbackChainEntries(t *testing.T) {
	withSourceIPTrust(t, "10.0.0.0/8")

	res := GetSourceIP(requestFrom("10.0.0.1:9000", http.Header{
		xForwardedFor: []string{"8.8.8.8, 127.0.0.1"},
	}))
	if res != "127.0.0.1" {
		t.Errorf("loopback chain entry was skipped: got %s, want 127.0.0.1", res)
	}
}

// A link-local peer presents with a zone. net.ParseIP rejects those outright, so
// without stripping it the peer resolves to nothing and could never be trusted.
func TestTrustedProxiesAcceptsZonedIPv6Peer(t *testing.T) {
	withSourceIPTrust(t, "fe80::1")

	res := GetSourceIP(requestFrom("[fe80::1%eth0]:9000", http.Header{
		xForwardedFor: []string{"1.2.3.4"},
	}))
	if res != "1.2.3.4" {
		t.Errorf("zoned IPv6 peer was not trusted: got %s, want 1.2.3.4", res)
	}
}

// An allow-list entry written in IPv4-mapped form grants no trust: it is a
// 128-bit prefix, and the peer address has already been reduced to its plain
// form by the time it is matched. This is a wart, and it is deliberately left
// alone - rewriting such entries was tried and reverted, because the rewrite
// reached no real request and changed what the shared parser means for the LDAP
// STS allow-list that was using it first. The direction is fail-closed: the
// peer is simply not trusted, and the request is attributed to it.
func TestTrustedProxiesIgnoreMappedIPv4AllowListEntries(t *testing.T) {
	withSourceIPTrust(t, "::ffff:192.168.1.10")

	res := GetSourceIP(requestFrom("192.168.1.10:9000", http.Header{
		xForwardedFor: []string{"1.2.3.4"},
	}))
	if res != "192.168.1.10" {
		t.Errorf("a mapped entry granted trust: got %s, want the peer itself (192.168.1.10)", res)
	}
}

// The policy has to be re-read after the server loads MINIO_CONFIG_ENV_FILE.
// Reading it only at package initialisation left every environment-file
// deployment on the historical trust-any-peer mode with nothing reported.
func TestConfigureSourceIPTrustReadsEnvironmentLate(t *testing.T) {
	prevPolicy, prevProxies := sourceIPPolicy, trustedProxies
	t.Cleanup(func() {
		sourceIPPolicy, trustedProxies = prevPolicy, prevProxies
	})

	t.Setenv(EnvTrustedProxies, "10.0.0.0/8")
	if err := ConfigureSourceIPTrust(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sourceIPPolicy != trustListedPeers {
		t.Fatalf("policy = %v, want trustListedPeers", sourceIPPolicy)
	}
	if res := GetSourceIP(requestFrom("203.0.113.9:44321", http.Header{
		xForwardedFor: []string{"8.8.8.8"},
	})); res != "203.0.113.9" {
		t.Errorf("late-configured allow-list not in effect: got %s, want 203.0.113.9", res)
	}

	// A malformed value must be reported, not silently ignored.
	t.Setenv(EnvTrustedProxies, "nonsense")
	if err := ConfigureSourceIPTrust(); err == nil {
		t.Error("malformed allow-list was accepted")
	}
	if sourceIPPolicy != trustNoPeer {
		t.Errorf("policy = %v after a malformed value, want trustNoPeer", sourceIPPolicy)
	}
}

// A value written as a remote env:// reference whose fetch fails must not be
// mistaken for an unset variable. env.Get drops that error and returns "", which
// would land on the permissive default at exactly the moment the operator's
// intent could not be read.
func TestConfigureSourceIPTrustRejectsUnreadableValue(t *testing.T) {
	prevPolicy, prevProxies := sourceIPPolicy, trustedProxies
	t.Cleanup(func() {
		sourceIPPolicy, trustedProxies = prevPolicy, prevProxies
	})

	// Port 1 on loopback refuses immediately, and no cached _-prefixed fallback
	// exists for this key.
	t.Setenv(EnvTrustedProxies, "env://user:pass@127.0.0.1:1/webhook/v1/getenv")
	if err := ConfigureSourceIPTrust(); err == nil {
		t.Fatal("an unreadable value was accepted")
	}
	if sourceIPPolicy != trustAnyPeer {
		return // failed closed, which is the point
	}
	t.Error("an unreadable value resolved to trustAnyPeer; any peer may forge the source address")
}

// _MINIO_API_XFF_HEADER must keep upstream's read timing. Upstream takes it at
// package initialisation, before environment files are loaded, so a deployment
// that writes it into MINIO_CONFIG_ENV_FILE has it silently ignored. Picking it
// up in ConfigureSourceIPTrust would make that already-deployed setting start
// taking effect, which is the one kind of change this rework exists to avoid.
func TestConfigureSourceIPTrustLeavesXFFSwitchAlone(t *testing.T) {
	prevXFF := enableXFFHeader
	t.Cleanup(func() { enableXFFHeader = prevXFF })

	enableXFFHeader = true
	t.Setenv(EnvXFFHeader, config.EnableOff)
	if err := ConfigureSourceIPTrust(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enableXFFHeader {
		t.Error("the X-Forwarded-For switch was re-read from the environment")
	}
}

// forwardThroughNode sends a client request through the node-to-node forwarder
// and reports what the receiving node resolves as the source address. The
// receiving node's peer is the forwarding node, not the client, which is where
// the trust policy has its least obvious consequence.
func forwardThroughNode(t *testing.T, proxies, clientPeer, forwardingNode string) string {
	t.Helper()
	withSourceIPTrust(t, proxies)

	var resolved string
	receiving := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// The test client's real peer is loopback, which is trusted
		// unconditionally; substitute the address the forwarding node would
		// present so the allow-list decision is the one under test.
		r.RemoteAddr = forwardingNode
		resolved = GetSourceIPRaw(r)
	}))
	defer receiving.Close()

	in := httptest.NewRequest(http.MethodGet, receiving.URL+"/probe", nil)
	in.RemoteAddr = clientPeer
	in.RequestURI = "/probe"
	in.URL.Scheme = "http"
	in.URL.Host = strings.TrimPrefix(receiving.URL, "http://")

	NewForwarder(&Forwarder{PassHost: true}).ServeHTTP(httptest.NewRecorder(), in)
	return resolved
}

func TestForwardedBetweenNodesAttribution(t *testing.T) {
	const (
		client         = "203.0.113.77:44321"
		forwardingNode = "10.10.0.2:36000"
	)

	tests := []struct {
		name    string
		proxies string
		want    string
	}{{
		name: "default mode carries the client through",
		want: "203.0.113.77",
	}, {
		// Believing no header means believing nothing about the forwarding node
		// either, so an internally forwarded request is attributed to it. There
		// is no configuration that corrects this, which is why the documentation
		// steers multi-node clusters to the allow-list instead.
		name:    "trusting nobody attributes to the forwarding node",
		proxies: TrustNoProxies,
		want:    "10.10.0.2",
	}, {
		name:    "allow-list omitting the cluster's own nodes",
		proxies: "192.168.1.0/24",
		want:    "10.10.0.2",
	}, {
		name:    "allow-list including the cluster's own nodes",
		proxies: "192.168.1.0/24,10.10.0.2",
		want:    "203.0.113.77",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forwardThroughNode(t, tt.proxies, client, forwardingNode); got != tt.want {
				t.Errorf("receiving node resolved %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLookupSourceIPTrust(t *testing.T) {
	tests := []struct {
		name    string
		proxies string
		want    sourceIPTrust
		wantErr bool
	}{
		{name: "unset is the historical default", want: trustAnyPeer},
		{name: "none", proxies: TrustNoProxies, want: trustNoPeer},
		{name: "none is case-insensitive", proxies: "None", want: trustNoPeer},
		{name: "off is accepted as a synonym", proxies: "off", want: trustNoPeer},
		{name: "surrounding space is ignored", proxies: "  none  ", want: trustNoPeer},
		{name: "allow-list", proxies: "10.0.0.0/8", want: trustListedPeers},
		{name: "whitespace only is indistinguishable from unset", proxies: "   ", want: trustAnyPeer},
		// Written on purpose, but naming nobody. Falling back to the permissive
		// default would be the one answer the operator cannot have wanted.
		{name: "separators only rejected", proxies: ",", want: trustNoPeer, wantErr: true},
		{name: "assorted separators rejected", proxies: "  ,; ", want: trustNoPeer, wantErr: true},
		// A catch-all would trust every peer and quietly undo the allow-list.
		{name: "catch-all v4 rejected", proxies: "0.0.0.0/0", want: trustNoPeer, wantErr: true},
		{name: "catch-all v6 rejected", proxies: "::/0", want: trustNoPeer, wantErr: true},
		{name: "catch-all hidden in a list", proxies: "10.0.0.0/8,0.0.0.0/0", want: trustNoPeer, wantErr: true},
		// A mapped spelling of the whole IPv4 space stays a /96 here, and so is
		// accepted rather than rejected. It matches nothing, because a peer never
		// reaches Contains in mapped form - the breadth guard covers what an
		// operator would actually write, not every way to write it.
		{name: "mapped whole-IPv4 space is a /96, not a catch-all", proxies: "::ffff:0:0/96", want: trustListedPeers},
		{name: "garbage rejected", proxies: "10.0.0.0/8,nonsense", want: trustNoPeer, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := lookupSourceIPTrust(tt.proxies)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			// A rejected list must fail closed, not fall back to trusting everyone.
			if got != tt.want {
				t.Fatalf("policy = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanonicalSourceIP(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1.2.3.4", "1.2.3.4"},
		{" 1.2.3.4 ", "1.2.3.4"},
		{"1.2.3.4:443", "1.2.3.4"},
		{"2001:db8::1", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"::ffff:1.2.3.4", "1.2.3.4"},
		{"_gazonk", ""},
		{"workstation.local", ""},
		{"", ""},
	}

	for _, tt := range tests {
		if got := canonicalSourceIP(tt.in); got != tt.want {
			t.Errorf("canonicalSourceIP(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRewriteDropsUnvouchedClaims covers the node-to-node forwarder. Relaying a
// header the peer was not entitled to set would hand the client this node's
// authority at the next hop, which does trust it.
func TestRewriteDropsUnvouchedClaims(t *testing.T) {
	rw := &headerRewriter{}

	withSourceIPTrust(t, "10.0.0.0/8")

	untrusted := requestFrom("203.0.113.9:44321", http.Header{
		xRealIP:   []string{"1.2.3.4"},
		forwarded: []string{"for=1.2.3.4"},
	})
	rw.Rewrite(untrusted)
	if got := untrusted.Header.Get(xRealIP); got != "203.0.113.9" {
		t.Errorf("X-Real-IP from an untrusted peer should be replaced: got %s, want 203.0.113.9", got)
	}
	if got := untrusted.Header.Get(forwarded); got != "" {
		t.Errorf("Forwarded from an untrusted peer should be dropped: got %s", got)
	}

	// A trusted proxy's own chain must survive, or a two-hop deployment loses the
	// client it correctly identified.
	trusted := requestFrom("10.0.0.1:9000", http.Header{
		xRealIP:   []string{"1.2.3.4"},
		forwarded: []string{"for=1.2.3.4"},
	})
	rw.Rewrite(trusted)
	if got := trusted.Header.Get(xRealIP); got != "1.2.3.4" {
		t.Errorf("X-Real-IP from a trusted peer should be relayed: got %s, want 1.2.3.4", got)
	}
	if got := trusted.Header.Get(forwarded); got != "for=1.2.3.4" {
		t.Errorf("Forwarded from a trusted peer should be relayed: got %s", got)
	}

	// Default mode must behave exactly as it did: fill in only what is missing.
	withSourceIPTrust(t, "")
	legacy := requestFrom("203.0.113.9:44321", http.Header{xRealIP: []string{"1.2.3.4"}})
	rw.Rewrite(legacy)
	if got := legacy.Header.Get(xRealIP); got != "1.2.3.4" {
		t.Errorf("default mode should relay X-Real-IP untouched: got %s, want 1.2.3.4", got)
	}
}
