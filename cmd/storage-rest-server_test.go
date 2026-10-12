// Copyright (c) 2015-2023 MinIO, Inc.
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

import "testing"

// TestNSScannerRespPooling verifies that recycled NSScanner stream
// responses cannot leak stale fields from a previous scan into new
// responses, which would otherwise produce incorrect bucket usage
// data.
func TestNSScannerRespPooling(t *testing.T) {
	dirty := storageNSScannerRPC.NewResponse()
	dirty.Update = &dataUsageEntry{}
	dirty.Final = &dataUsageCache{}
	storageNSScannerRPC.PutResponse(dirty)

	resp := storageNSScannerRPC.NewResponse()
	if resp.Update != nil || resp.Final != nil {
		t.Fatalf("pooled nsScannerResp leaked stale fields: Update %v, Final %v", resp.Update, resp.Final)
	}
}
