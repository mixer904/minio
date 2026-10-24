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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/cli"
	xhttp "github.com/minio/minio/internal/http"
)

// Default probe deadlines. Cluster checks are evaluated server-side under the
// (default 10s) cluster_deadline, so their client deadline must be longer or
// an unhealthy cluster answer would never be received.
const (
	healthcheckLocalTimeout   = 5 * time.Second
	healthcheckClusterTimeout = 15 * time.Second
)

// healthcheckChecks maps the CLI check vocabulary 1:1 onto the server's
// /minio/health/<path> endpoints. The path literals are shared with the
// health router; the semantics live server-side only.
var healthcheckChecks = map[string]string{
	"live":         healthCheckLivenessPath,
	"ready":        healthCheckReadinessPath,
	"cluster":      healthCheckClusterPath,
	"cluster-read": healthCheckClusterReadPath,
}

var healthcheckFlags = []cli.Flag{
	cli.StringFlag{
		Name:   "address",
		Value:  ":" + GlobalMinioDefaultPort,
		Usage:  "probe the server bound to a specific ADDRESS:PORT, an empty ADDRESS is probed as 127.0.0.1",
		EnvVar: "MINIO_ADDRESS",
	},
	cli.StringFlag{
		Name:   "url",
		Usage:  "probe this base URL (http[s]://HOST:PORT) instead of deriving one from --address and the certs directory",
		EnvVar: "MINIO_HEALTHCHECK_URL",
	},
	cli.BoolFlag{
		Name:  "maintenance",
		Usage: "with the cluster check only: ask whether taking this node down would lose quorum (HTTP 412 means it would)",
	},
	cli.DurationFlag{
		Name:  "timeout",
		Usage: "overall probe deadline (default: 5s for live/ready, 15s for cluster checks)",
	},
}

var healthcheckCmd = cli.Command{
	Name:   "healthcheck",
	Usage:  "Probe the health of a Silo server and report it as the exit code",
	Flags:  append(healthcheckFlags, GlobalFlags...),
	Action: healthcheckMain,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} {{if .VisibleFlags}}[FLAGS] {{end}}[CHECK]

CHECK:
  live          the process is serving requests (default); touches no external system
  ready         live, plus KMS and etcd reachability when they are configured
  cluster       cluster-wide write quorum across every erasure set
  cluster-read  cluster-wide read quorum across every erasure set
{{if .VisibleFlags}}
FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
EXIT CODE:
  0 - healthy (with --maintenance: safe to take the node down)
  1 - anything else

EXAMPLES:
  1. Probe local liveness, e.g. as a container HEALTHCHECK:
     {{.Prompt}} {{.HelpName}}
  2. Probe readiness of a server on a non-default port:
     {{.Prompt}} {{.HelpName}} --address :9010 ready
  3. Ask whether this node can be taken down without losing HA:
     {{.Prompt}} {{.HelpName}} --maintenance cluster
`,
}

// healthcheckResult is the outcome of a single probe. It doubles as the
// --json output schema, so field changes are compatibility-relevant.
type healthcheckResult struct {
	Check         string `json:"check"`
	Healthy       bool   `json:"healthy"`
	StatusCode    int    `json:"status,omitempty"`
	DurationMS    int64  `json:"durationMs,omitempty"`
	ServerStatus  string `json:"serverStatus,omitempty"`
	WriteQuorum   string `json:"writeQuorum,omitempty"`
	ReadQuorum    string `json:"readQuorum,omitempty"`
	HealingDrives string `json:"healingDrives,omitempty"`
	Err           string `json:"error,omitempty"`
}

// line renders the single human-readable output line. Container runtimes
// store only the first 4096 bytes of probe output, so it stays short.
func (r healthcheckResult) line() string {
	if r.Err != "" {
		return fmt.Sprintf("%s: unreachable (%s)", r.Check, r.Err)
	}
	if r.Healthy {
		return fmt.Sprintf("%s: ok (%d, %dms)", r.Check, r.StatusCode, r.DurationMS)
	}
	label := "unhealthy"
	if r.StatusCode == http.StatusPreconditionFailed {
		label = "not safe for maintenance"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s (%d)", r.Check, label, r.StatusCode)
	for _, kv := range []struct{ k, v string }{
		{"server-status", r.ServerStatus},
		{"write-quorum", r.WriteQuorum},
		{"read-quorum", r.ReadQuorum},
		{"healing-drives", r.HealingDrives},
	} {
		if kv.v != "" {
			fmt.Fprintf(&b, " %s=%s", kv.k, kv.v)
		}
	}
	return b.String()
}

// healthcheckTarget derives the base URL to probe. An explicit rawURL wins;
// otherwise the address' host:port is used, with the scheme decided by the
// same certificate presence check the server performs at startup. URLs are
// serialized via url.URL so IPv6 zone identifiers survive as %25-escapes.
func healthcheckTarget(rawURL, address, certsDir string) (string, error) {
	if rawURL != "" {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("invalid --url %q: %w", rawURL, err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", fmt.Errorf("invalid --url %q: expected http[s]://HOST:PORT", rawURL)
		}
		return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String(), nil
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid --address %q: %w", address, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	scheme := "http"
	if isFile(filepath.Join(certsDir, publicCertFile)) && isFile(filepath.Join(certsDir, privateKeyFile)) {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, port)}).String(), nil
}

// probeHealth performs one bounded, strictly anonymous GET against the
// health endpoint for check. Anonymity is load-bearing: a credentialed
// request is rejected by the reserved-path guard instead of answered.
func probeHealth(baseURL, check string, maintenance bool, timeout time.Duration) healthcheckResult {
	res := healthcheckResult{Check: check}

	probeURL := baseURL + healthCheckPathPrefix + healthcheckChecks[check]
	if maintenance {
		probeURL += "?maintenance=true"
	}

	// Proxy is nil on purpose: a loopback probe must never be routed through
	// an HTTP_PROXY inherited from the container environment. Certificate
	// verification is skipped to match the kubelet's HTTPS probe behavior.
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             nil,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			DisableKeepAlives: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "silo-healthcheck/"+ReleaseTag)

	started := time.Now()
	resp, err := client.Do(req)
	res.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	res.StatusCode = resp.StatusCode
	res.Healthy = resp.StatusCode == http.StatusOK
	res.ServerStatus = resp.Header.Get(xhttp.MinIOServerStatus)
	res.WriteQuorum = resp.Header.Get(xhttp.MinIOWriteQuorum)
	res.ReadQuorum = resp.Header.Get(xhttp.MinIOReadQuorum)
	res.HealingDrives = resp.Header.Get(xhttp.MinIOHealingDrives)
	return res
}

// healthcheckCertsDir mirrors the server's certs-dir resolution without its
// side effects: an explicit --certs-dir wins, an explicit --config-dir
// implies <config-dir>/certs, and the shared default applies otherwise.
func healthcheckCertsDir(ctx *cli.Context) string {
	switch {
	case ctx.IsSet("certs-dir"):
		return ctx.String("certs-dir")
	case ctx.GlobalIsSet("certs-dir"):
		return ctx.GlobalString("certs-dir")
	case ctx.IsSet("config-dir"):
		return filepath.Join(ctx.String("config-dir"), certsDir)
	case ctx.GlobalIsSet("config-dir"):
		return filepath.Join(ctx.GlobalString("config-dir"), certsDir)
	}
	return defaultCertsDir.Get()
}

func healthcheckMain(ctx *cli.Context) {
	fail := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "healthcheck: "+format+"\n", args...)
		os.Exit(1)
	}

	if len(ctx.Args()) > 1 {
		fail("too many arguments, expected at most one CHECK")
	}
	check := "live"
	if arg := ctx.Args().First(); arg != "" {
		check = arg
	}
	if _, ok := healthcheckChecks[check]; !ok {
		fail("unknown check %q, expected one of: live, ready, cluster, cluster-read", check)
	}
	if ctx.Bool("maintenance") && check != "cluster" {
		fail("--maintenance applies to the cluster check only")
	}

	timeout := ctx.Duration("timeout")
	if !ctx.IsSet("timeout") {
		timeout = healthcheckLocalTimeout
		if strings.HasPrefix(check, "cluster") {
			timeout = healthcheckClusterTimeout
		}
	}

	baseURL, err := healthcheckTarget(ctx.String("url"), ctx.String("address"), healthcheckCertsDir(ctx))
	if err != nil {
		fail("%v", err)
	}

	res := probeHealth(baseURL, check, ctx.Bool("maintenance"), timeout)

	quiet := ctx.Bool("quiet") || ctx.GlobalBool("quiet")
	switch {
	case ctx.Bool("json") || ctx.GlobalBool("json"):
		buf, jerr := json.Marshal(res)
		if jerr != nil {
			fail("%v", jerr)
		}
		fmt.Println(string(buf))
	case !res.Healthy:
		fmt.Fprintln(os.Stderr, res.line())
	case !quiet:
		fmt.Println(res.line())
	}

	if !res.Healthy {
		os.Exit(1)
	}
}
