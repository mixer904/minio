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

package cmd

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/minio/minio/internal/auth"
)

// Tests the part number list validation of the CompleteMultipartUpload handler.
// The list must be strictly increasing: duplicate or out-of-order part numbers
// are rejected with InvalidPartOrder, and the rejection must happen before the
// target object is assembled. Only the ordering is constrained - part numbers
// need not start at 1 and need not be consecutive, so [1,3] and [5,9] are both
// legal and must be accepted.
func TestAPICompleteMultipartHandlerPartOrder(t *testing.T) {
	defer DetectTestLeak(t)()
	ExecObjectLayerAPITest(ExecObjectLayerAPITestArgs{
		t:          t,
		objAPITest: testAPICompleteMultipartHandlerPartOrder,
		endpoints:  []string{"NewMultipart", "PutObjectPart", "CompleteMultipart"},
	})
}

func testAPICompleteMultipartHandlerPartOrder(obj ObjectLayer, instanceType, bucketName string, apiRouter http.Handler,
	credentials auth.Credentials, t *testing.T,
) {
	ctx := context.Background()

	// Every uploaded part is >= the minimum part size so that a part is never
	// rejected for being too small when it is not the last one.
	partData := strings.Repeat("a", 5*humanize.MiByte)
	partETag := getMD5Hash([]byte(partData))

	testCases := []struct {
		name string
		// Part numbers uploaded before the completion request.
		upload []int
		// Part numbers listed in the completion request, in this order.
		complete           []int
		expectedRespStatus int
		// Only checked when expectedRespStatus is not http.StatusOK.
		expectedErr APIErrorCode
	}{
		// Defect reproduction. A duplicated part number is the case that
		// sort.SliceIsSorted() used to accept, because its '<' predicate treats
		// equal neighbors as sorted. Each of these assembled the same part into
		// the object more than once, inflating it past what was uploaded.
		{
			name:               "duplicate-part",
			upload:             []int{1},
			complete:           []int{1, 1},
			expectedRespStatus: http.StatusBadRequest,
			expectedErr:        ErrInvalidPartOrder,
		},
		{
			name:               "trailing-duplicate",
			upload:             []int{1, 2},
			complete:           []int{1, 2, 2},
			expectedRespStatus: http.StatusBadRequest,
			expectedErr:        ErrInvalidPartOrder,
		},
		{
			name:               "duplicate-max-part-number",
			upload:             []int{globalMaxPartID},
			complete:           []int{globalMaxPartID, globalMaxPartID},
			expectedRespStatus: http.StatusBadRequest,
			expectedErr:        ErrInvalidPartOrder,
		},

		// Regression guard. Ordering violations that were already rejected
		// before the fix and must stay rejected with the same error.
		{
			name:               "descending",
			upload:             []int{1, 2},
			complete:           []int{2, 1},
			expectedRespStatus: http.StatusBadRequest,
			expectedErr:        ErrInvalidPartOrder,
		},
		{
			name:               "repeat-after-increase",
			upload:             []int{1, 2},
			complete:           []int{1, 2, 1},
			expectedRespStatus: http.StatusBadRequest,
			expectedErr:        ErrInvalidPartOrder,
		},

		// Regression guard. The order loop starts at index 1 and so is a no-op
		// for an empty list; an empty list is only rejected because the handler
		// screens it out first. This pins that dependency down - without the
		// len()==0 check the empty list reaches readParts() and panics the
		// process on partMetaPaths[0] in xl-storage-disk-id-check.go.
		{
			name:               "empty-parts",
			upload:             []int{1},
			complete:           nil,
			expectedRespStatus: http.StatusBadRequest,
			expectedErr:        ErrMissingPart,
		},

		// Regression guard. Legal lists that must keep working. Part numbers
		// only have to increase: they need not start at 1 and need not be
		// consecutive, so none of these may be rejected.
		{
			name:               "single-part",
			upload:             []int{1},
			complete:           []int{1},
			expectedRespStatus: http.StatusOK,
		},
		{
			name:               "strictly-increasing",
			upload:             []int{1, 2},
			complete:           []int{1, 2},
			expectedRespStatus: http.StatusOK,
		},
		{
			name:               "non-consecutive",
			upload:             []int{1, 3},
			complete:           []int{1, 3},
			expectedRespStatus: http.StatusOK,
		},
		{
			name:               "gap-not-starting-at-one",
			upload:             []int{5, 9},
			complete:           []int{5, 9},
			expectedRespStatus: http.StatusOK,
		},
		{
			name:               "single-part-not-one",
			upload:             []int{3},
			complete:           []int{3},
			expectedRespStatus: http.StatusOK,
		},
		{
			name:               "part-number-bounds",
			upload:             []int{1, globalMaxPartID},
			complete:           []int{1, globalMaxPartID},
			expectedRespStatus: http.StatusOK,
		},
	}

	// completeReq issues a CompleteMultipartUpload request listing partNumbers
	// in the given order and returns the recorded response.
	completeReq := func(objectName, uploadID string, partNumbers []int) *httptest.ResponseRecorder {
		t.Helper()

		completeUploads := &CompleteMultipartUpload{}
		for _, partNumber := range partNumbers {
			completeUploads.Parts = append(completeUploads.Parts, CompletePart{
				PartNumber: partNumber,
				ETag:       partETag,
			})
		}
		completeBytes, err := xml.Marshal(completeUploads)
		if err != nil {
			t.Fatalf("%s: error XML encoding of parts: %v", instanceType, err)
		}

		req, err := newTestSignedRequestV4(http.MethodPost,
			getCompleteMultipartUploadURL("", bucketName, objectName, uploadID),
			int64(len(completeBytes)), bytes.NewReader(completeBytes),
			credentials.AccessKey, credentials.SecretKey, nil)
		if err != nil {
			t.Fatalf("%s: failed to create HTTP request for CompleteMultipartUpload: %v", instanceType, err)
		}

		rec := httptest.NewRecorder()
		apiRouter.ServeHTTP(rec, req)
		return rec
	}

	for i, testCase := range testCases {
		objectName := fmt.Sprintf("test-part-order-%d-%s", i, testCase.name)

		res, err := obj.NewMultipartUpload(ctx, bucketName, objectName, ObjectOptions{})
		if err != nil {
			t.Fatalf("%s: %s: failed to initiate multipart upload: %v", instanceType, testCase.name, err)
		}
		for _, partNumber := range testCase.upload {
			_, err = obj.PutObjectPart(ctx, bucketName, objectName, res.UploadID, partNumber,
				mustGetPutObjReader(t, strings.NewReader(partData), int64(len(partData)), partETag, ""), ObjectOptions{})
			if err != nil {
				t.Fatalf("%s: %s: failed to upload part %d: %v", instanceType, testCase.name, partNumber, err)
			}
		}

		rec := completeReq(objectName, res.UploadID, testCase.complete)
		if rec.Code != testCase.expectedRespStatus {
			t.Errorf("%s: %s: expected response status %d, got %d: %s",
				instanceType, testCase.name, testCase.expectedRespStatus, rec.Code, rec.Body.String())
		}

		objInfo, statErr := obj.GetObjectInfo(ctx, bucketName, objectName, ObjectOptions{})

		if testCase.expectedRespStatus != http.StatusOK {
			var errResp APIErrorResponse
			if err = xml.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Errorf("%s: %s: failed parsing error response %s: %v",
					instanceType, testCase.name, rec.Body.String(), err)
			} else if wantCode := getAPIError(testCase.expectedErr).Code; errResp.Code != wantCode {
				t.Errorf("%s: %s: expected error code %s, got %s",
					instanceType, testCase.name, wantCode, errResp.Code)
			}
			// A rejected completion must not have touched the target object.
			if statErr == nil {
				t.Errorf("%s: %s: expected no object to be created, found one of size %d",
					instanceType, testCase.name, objInfo.Size)
			} else if !isErrObjectNotFound(statErr) {
				t.Errorf("%s: %s: expected ObjectNotFound after a rejected completion, got %v",
					instanceType, testCase.name, statErr)
			}

			// The rejection must also leave the upload itself untouched, so
			// that a client can retry with a well formed list. testCase.upload
			// is always strictly increasing, so it is the corrected list.
			retry := completeReq(objectName, res.UploadID, testCase.upload)
			if retry.Code != http.StatusOK {
				t.Errorf("%s: %s: expected the upload to survive a rejected completion, retry got %d: %s",
					instanceType, testCase.name, retry.Code, retry.Body.String())
				continue
			}
			objInfo, statErr = obj.GetObjectInfo(ctx, bucketName, objectName, ObjectOptions{})
			if statErr != nil {
				t.Errorf("%s: %s: expected the object to exist after the retry: %v", instanceType, testCase.name, statErr)
				continue
			}
			if wantSize := int64(len(testCase.upload) * len(partData)); objInfo.Size != wantSize {
				t.Errorf("%s: %s: expected the retried object to be %d bytes, got %d",
					instanceType, testCase.name, wantSize, objInfo.Size)
			}
			continue
		}

		if statErr != nil {
			t.Errorf("%s: %s: expected the object to exist after completion: %v", instanceType, testCase.name, statErr)
			continue
		}
		if wantSize := int64(len(testCase.complete) * len(partData)); objInfo.Size != wantSize {
			t.Errorf("%s: %s: expected the completed object to be %d bytes, got %d",
				instanceType, testCase.name, wantSize, objInfo.Size)
		}
	}
}
