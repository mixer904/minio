// Copyright (c) 2015-2025 MinIO, Inc.
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

package config

import (
	"net/netip"
	"strings"
)

// TrustedProxies is an allow-list of peer addresses whose forwarded headers may
// be believed. The zero value trusts nobody, which is the safe default: a
// request whose peer is not on the list is attributed to the peer itself rather
// than to whatever address the request claims.
type TrustedProxies []netip.Prefix

// ParseTrustedProxies parses a list of IP addresses and CIDR blocks separated by
// commas, semicolons or whitespace. A bare address is widened to a single-host
// prefix. what names the setting being parsed and is used in error messages.
//
// Catch-all prefixes are rejected rather than accepted, because a list
// containing 0.0.0.0/0 or ::/0 trusts every peer and so silently reinstates the
// behavior the allow-list exists to prevent.
func ParseTrustedProxies(value, what string) (TrustedProxies, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 {
		return nil, nil
	}

	prefixes := make(TrustedProxies, 0, len(fields))
	for _, field := range fields {
		if prefix, err := netip.ParsePrefix(field); err == nil {
			masked := prefix.Masked()
			if masked.Bits() == 0 {
				return nil, Errorf("%s %q is too broad", what, field)
			}
			prefixes = append(prefixes, masked)
			continue
		}

		addr, err := netip.ParseAddr(field)
		if err != nil {
			return nil, Errorf("invalid %s %q", what, field)
		}
		bits := 32
		if addr.Is6() {
			bits = 128
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, bits))
	}

	return prefixes, nil
}

// Contains reports whether ip is covered by the allow-list. ip must already be a
// bare address; anything that does not parse is not trusted.
//
// Matching is deliberately literal. An entry written in IPv4-mapped form
// (::ffff:192.168.1.10) is a 128-bit prefix and will not match a peer presenting
// as 192.168.1.10, because netip.Prefix.Contains is false across differing
// widths - so such an entry is accepted and then matches nothing. Rewriting
// those entries to the IPv4 prefix they denote was tried and reverted: callers
// already reduce the address through net.ParseIP(...).String() before arriving
// here, which collapses the mapped form anyway, so the rewrite reached no real
// request and served only to change what this shared function means for the
// LDAP STS allow-list that was using it first. Write entries in plain form.
func (t TrustedProxies) Contains(ip string) bool {
	if len(t) == 0 {
		return false
	}

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}

	for _, prefix := range t {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
