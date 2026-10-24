// Copyright (c) 2015-2022 MinIO, Inc.
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

package subnet

import (
	"errors"
	"net/http"
)

const (
	// LoggerWebhookName - subnet logger webhook target
	LoggerWebhookName = "subnet"
)

var errSiloSubnetDisabled = errors.New("MinIO SUBNET integration is disabled in Silo")

// Upload given file content (payload) to specified URL
func (c Config) Upload(reqURL string, filename string, payload []byte) (string, error) {
	return "", errSiloSubnetDisabled
}

func (c Config) submitPost(_ *http.Request) (string, error) {
	return "", errSiloSubnetDisabled
}

// Post submit 'payload' to specified URL
func (c Config) Post(reqURL string, payload any) (string, error) {
	return "", errSiloSubnetDisabled
}
