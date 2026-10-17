// Copyright (c) 2026 Ruohang Feng
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
)

// Run explicitly with -benchtime=1x. Fixture creation is excluded from timing;
// each request still discovers real on-disk metadata. The 16-drive/10k case
// deliberately exceeds the existing aggregate directory-entry budget.
func BenchmarkMultipartListingCapacity(b *testing.B) {
	for _, tc := range []struct {
		drives, uploads int
		mixed, limited  bool
	}{
		{4, 1_000, false, false},
		{4, 10_000, false, false},
		{4, 10_000, true, false},
		{16, 1_000, false, false},
		{16, 10_000, false, true},
	} {
		b.Run(fmt.Sprintf("drives=%d/uploads=%d/mixed=%t", tc.drives, tc.uploads, tc.mixed), func(b *testing.B) {
			obj, dirs, err := prepareErasure(b.Context(), tc.drives)
			if err != nil {
				b.Fatal(err)
			}
			z := obj.(*erasureServerPools)
			b.Cleanup(func() { z.Shutdown(context.Background()); removeRoots(dirs) })
			const bucket, other = "multipart-capacity", "multipart-unrelated"
			for _, name := range []string{bucket, other} {
				if err := z.MakeBucket(b.Context(), name, MakeBucketOptions{}); err != nil {
					b.Fatal(err)
				}
			}
			for n := range tc.uploads {
				target := bucket
				if tc.mixed && n%2 != 0 {
					target = other
				}
				key := fmt.Sprintf("dir/%05d", n)
				mp, err := z.NewMultipartUpload(b.Context(), target, key, ObjectOptions{})
				if err != nil {
					b.Fatal(err)
				}
				if tc.mixed && n%10 == 0 {
					rewriteMultipartIdentity(b, z.serverPools[0].getHashedSet(key), target, key, mp.UploadID, removeMultipartIdentity)
				}
			}
			z.mpCache.Clear()
			var directoryCalls, metadataCalls atomic.Int64
			for _, set := range z.serverPools[0].sets {
				original := set.getDisks
				set.getDisks = func() []StorageAPI {
					disks := append([]StorageAPI(nil), original()...)
					for i, disk := range disks {
						disks[i] = multipartListingCountingDisk{disk, &directoryCalls, &metadataCalls}
					}
					return disks
				}
				b.Cleanup(func() { set.getDisks = original })
			}
			b.ResetTimer()
			for range b.N {
				got, err := z.ListMultipartUploads(b.Context(), bucket, "dir/", "", "", "", 1000)
				if tc.limited {
					var limited SlowDown
					if !errors.As(err, &limited) || len(got.Uploads) != 0 {
						b.Fatalf("overflow must return SlowDown without a partial page: %+v %v", got, err)
					}
				} else if err != nil || len(got.Uploads) != 1000 {
					b.Fatalf("capacity listing: count=%d err=%v", len(got.Uploads), err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(directoryCalls.Load())/float64(b.N), "ListDir/op")
			b.ReportMetric(float64(metadataCalls.Load())/float64(b.N), "ReadVersion/op")
		})
	}
}
