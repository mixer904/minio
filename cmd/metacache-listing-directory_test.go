// Copyright (c) 2026 Ruohang Feng
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

type listingDirectoryWalkSnapshot struct {
	StorageAPI
	bucket   string
	snapshot []byte
}

func (d listingDirectoryWalkSnapshot) WalkDir(ctx context.Context, opts WalkDirOptions, out io.Writer) error {
	if opts.Bucket != d.bucket {
		return d.StorageAPI.WalkDir(ctx, opts, out)
	}
	_, err := out.Write(d.snapshot)
	return err
}

// Capture two actual directory-walk generations for a directory object, then
// acknowledge its deletion. A different pool still contains dir/child. The
// stale source walkers disagree, while every current source drive is absent.
func TestListingRecoveryDeletedDirectoryKeepsCommonPrefix(t *testing.T) {
	for _, marker := range []bool{false, true} {
		for _, sourcePool := range []int{0, 1} {
			t.Run(fmt.Sprintf("pool=%d/marker=%t", sourcePool, marker), func(t *testing.T) {
				z, bucket, router := listingPoolsBackend(t)
				ctx := t.Context()
				source := z.serverPools[sourcePool].sets[0]
				target := z.serverPools[1-sourcePool].sets[0]
				childBody := []byte("surviving child")
				put := func(set *erasureObjects, key string, tm time.Time) {
					var body []byte
					if key == "dir/child" {
						body = childBody
					}
					_, err := set.PutObject(ctx, bucket, encodeDirObject(key), mustGetPutObjReader(t, bytes.NewReader(body), int64(len(body)), "", ""), ObjectOptions{MTime: tm, VersionSuspended: marker})
					if err != nil {
						t.Fatal(err)
					}
				}
				tm := time.Now().UTC().Add(-time.Hour)
				put(target, "dir/child", tm)
				disks, switchDisks := installListingDiskSwitch(source)
				snapshots := make([][]byte, len(disks))
				for gen := range 3 {
					if gen < 2 {
						put(source, "dir/", tm.Add(time.Duration(gen)*time.Minute))
					} else {
						if _, err := source.DeleteObject(ctx, bucket, encodeDirObject("dir/"), ObjectOptions{VersionSuspended: marker, DeleteMarker: marker, MTime: tm.Add(2 * time.Minute)}); err != nil {
							t.Fatal(err)
						}
					}
					for i, disk := range disks {
						wanted := 0
						if i >= 6 {
							wanted = 1
						}
						if i >= 12 {
							wanted = 2
						}
						if wanted != gen {
							continue
						}
						var b bytes.Buffer
						if err := disk.WalkDir(ctx, WalkDirOptions{Bucket: bucket, Recursive: false}, &b); err != nil {
							t.Fatal(err)
						}
						snapshots[i] = append([]byte(nil), b.Bytes()...)
					}
				}
				for _, key := range []string{"dir/child", "dir/"} {
					request, err := newTestSignedRequestV4(http.MethodGet, getGetObjectURL("", bucket, key), 0, nil, globalActiveCred.AccessKey, globalActiveCred.SecretKey, nil)
					if err != nil {
						t.Fatal(err)
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request.WithContext(ctx))
					t.Logf("GET %s HTTP %d", key, response.Code)
					want := http.StatusOK
					if key == "dir/" {
						want = http.StatusNotFound
					}
					if response.Code != want {
						t.Fatalf("GET %s: got %d want %d: %s", key, response.Code, want, response.Body.String())
					}
					if key == "dir/child" && !bytes.Equal(response.Body.Bytes(), childBody) {
						t.Fatal("surviving child content changed")
					}
				}
				wrapped := make([]StorageAPI, len(disks))
				for i, disk := range disks {
					wrapped[i] = listingDirectoryWalkSnapshot{disk, bucket, snapshots[i]}
				}
				switchDisks(wrapped)
				t.Cleanup(func() { switchDisks(disks) })
				req, err := newTestSignedRequestV4(http.MethodGet, getListObjectsV2URL("", bucket, "", "1000", "", "", "/"), 0, nil, globalActiveCred.AccessKey, globalActiveCred.SecretKey, nil)
				if err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req.WithContext(ctx))
				t.Logf("LIST HTTP %d body=%s", rec.Code, rec.Body.String())
				var page ListObjectsV2Response
				if rec.Code != http.StatusOK || xml.Unmarshal(rec.Body.Bytes(), &page) != nil {
					t.Fatalf("LIST failed: %d %s", rec.Code, rec.Body.String())
				}
				if len(page.CommonPrefixes) != 1 || page.CommonPrefixes[0].Prefix != "dir/" {
					t.Fatalf("acknowledged child lost its CommonPrefix: %+v", page)
				}
			})
		}
	}
}

// The directory must remain in the sorted input when an earlier object name
// disappears. Emitting it immediately would skip an intervening name, and
// keeping only one colliding directory would leave input order significant.
func TestListingRecoveryDirectoryCollisionOrdering(t *testing.T) {
	for _, object := range []string{"dir", "dir/"} {
		for _, present := range []bool{false, true} {
			if present && object == "dir" {
				continue
			}
			for source := range 3 {
				t.Run(fmt.Sprintf("object=%s/present=%t/source=%d", object, present, source), func(t *testing.T) {
					in := make([]chan metaCacheEntry, 6)
					for i := range in {
						in[i] = make(chan metaCacheEntry, 1)
						entry := metaCacheEntry{name: "dir/"}
						if i == source {
							entry = metaCacheEntry{name: object, listingSet: &erasureObjects{}}
						}
						if i >= 3 {
							entry = metaCacheEntry{name: []string{"dir-", "dir.ext", "dir0"}[i-3], metadata: []byte{1}}
						}
						in[i] <- entry
						close(in[i])
					}
					out := make(chan metaCacheEntry, len(in))
					err := mergeEntryChannelsWithResolver(t.Context(), in, out, 1, nil, func(group []*metaCacheEntry) (*metaCacheEntry, error) {
						if len(group) != 1 || group[0].listingSet == nil {
							t.Fatalf("directory entered the object group: %+v", group)
						}
						if !present {
							return nil, nil
						}
						return &metaCacheEntry{name: object, metadata: []byte{1}}, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					var names []string
					for entry := range out {
						names = append(names, entry.name)
						if entry.name == "dir/" && entry.isObject() != present {
							t.Fatalf("directory collision returned the wrong entry type: %+v", entry)
						}
					}
					want := []string{"dir-", "dir.ext", "dir/", "dir0"}
					if !slices.Equal(names, want) {
						t.Fatalf("names=%v, want %v", names, want)
					}
				})
			}
		}
	}
}
