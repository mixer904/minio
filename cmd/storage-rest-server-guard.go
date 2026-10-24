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
	"io"
	"runtime"

	"github.com/minio/madmin-go/v3"
	xioutil "github.com/minio/minio/internal/ioutil"
)

// guardedStorage rejects filesystem paths that arrive from an internode
// payload and would resolve outside the volume they name.
//
// Why here and not in each handler: the global HTTP middleware validates only
// r.URL.Path and r.Form (query arguments), and r.Form is never populated from
// a request body. Paths carried in a msgpack body, or in a grid RPC frame on
// the long-lived /minio/grid/v1 websocket, therefore reach xlStorage
// unvalidated. Every storage-REST and grid handler obtains its StorageAPI from
// storageRESTServer.getStorage(), so wrapping that one call covers all of them
// at once, sees the nested struct fields a per-handler check tends to miss,
// and cannot drift out of sync as handlers are added.
//
// Scope, deliberately narrow:
//
//   - Only *path* arguments are checked here. Volume arguments are validated
//     at the sink by xlStorage.getVolDir, which additionally covers callers
//     that bypass this wrapper entirely (the peer-S3 bucket RPCs). Do not
//     conclude from the absence of a volume check here that volumes are inert.
//
//   - Local callers - the erasure layer talking to its own drives - are not
//     wrapped. Their object names already passed IsValidObjectName at the S3
//     boundary, so wrapping them would add cost and regression risk without
//     adding a control.
type guardedStorage struct {
	StorageAPI
}

// isVolumeRootAlias reports whether p addresses the volume directory itself
// rather than something inside it, i.e. whether pathJoin(volumeDir, p)
// collapses back to volumeDir. That happens only for the empty string and for
// strings made up entirely of separators.
//
// Whitespace must NOT be treated as a separator here, even though
// hasBadPathComponent trims it when comparing a segment against "."/"..".
// path.Clean does not touch spaces, so pathJoin(volumeDir, "  ") is
// "volumeDir/  " - a real directory named two spaces, not the volume root. A
// whitespace-only object key is legal in S3 (IsValidObjectName accepts it) and
// is committed through RenameData on the PutObject path, so rejecting it here
// would fail those writes on every remote drive at once. See
// TestGuardAcceptsEveryLegalObjectName.
//
// Three characters collapse a component on Windows but not on Unix, so the
// rule is platform-split and exercised from either platform through
// isVolumeRootAliasOn:
//
//   - Backslash is a separator on Windows only. path.Clean never treats it as
//     one, so on Unix "\\" names an ordinary file and is a legal S3 object key;
//     refusing it there would make a distributed cluster reject a write that a
//     single-node server accepts.
//
//   - Space and period are stripped from the end of a path component by the
//     Win32 normalisation layer. A component made only of those characters
//     therefore disappears and the path resolves to its parent - the volume
//     root. Go does not add the \\?\ prefix that would suppress this for the
//     short paths used here, so " " and "..." reach the syscall as an empty
//     component. On Unix they are ordinary filenames and legal object keys.
//
// Note the second point is reasoned from documented Win32 behavior, not from a
// Windows test run: CI is Linux-only, so TestIsVolumeRootAliasIsPlatformCorrect
// pins both branches of the predicate rather than the syscall behavior itself.
func isVolumeRootAlias(p string) bool {
	return isVolumeRootAliasOn(p, runtime.GOOS == globalWindowsOSName)
}

func isVolumeRootAliasOn(p string, windows bool) bool {
	for i := range len(p) {
		if p[i] == SlashSeparatorChar {
			continue
		}
		if windows && (p[i] == '\\' || p[i] == ' ' || p[i] == '.') {
			continue
		}
		return false
	}
	return true
}

// guardErasureParams rejects a FileInfo from which no meaningful expected shard
// size can be derived, because "no meaningful size" degrades to "everything
// passes" rather than to an error.
//
// checkPart's only integrity test is "st.Size() < expectedSize". Whenever
// ShardFileSize yields 0, that comparison is false for every file that exists -
// including a truncated shard - so the part comes back reported healthy and a
// heal driven by the result skips a shard that actually needs repair. Two
// distinct inputs produce that 0:
//
//   - Unusable erasure parameters with a part of positive size. ShardFileSize
//     returns 0 early so the division cannot panic.
//
//   - A part of NEGATIVE size, with or without usable parameters: numShards
//     floors to 0 and ceilFrac of a negative numerator is 0, so the arithmetic
//     lands on 0 by itself. This one is easy to miss precisely because valid
//     erasure parameters do not save you from it.
//
// Parts of zero length are left alone - ShardFileSize legitimately returns 0
// for them, so they say nothing about whether the metadata is sound.
//
// This is the boundary check; ShardFileSize's own zero-value guard stays as the
// last line of defense against a panic.
func guardErasureParams(fi FileInfo) error {
	// Negative sizes share their rule with the storage layer, which also has to
	// cope with metadata already on disk; keep the two from drifting apart by
	// asking the same predicate.
	if fi.HasNegativePartSize() {
		return errFileCorrupt
	}
	usable := fi.Erasure.BlockSize > 0 && fi.Erasure.DataBlocks > 0
	for _, p := range fi.Parts {
		if p.Size > 0 && !usable {
			return errFileCorrupt
		}
	}
	return nil
}

// guardPaths rejects traversal in any of the supplied paths. The empty string
// is allowed: it legitimately names the volume root for listing operations,
// and an empty FileInfo.DataDir is normal for inline and transitioned objects.
//
// Callers pass the entire batch here before invoking storage, so a payload
// mixing a valid target with a malicious one is rejected whole and performs no
// partial work. (This returns on the first offending path; what matters is that
// it runs to a verdict before any element has been acted on.)
func guardPaths(paths ...string) error {
	for _, p := range paths {
		if hasBadPathComponent(p) {
			return errFileAccessDenied
		}
	}
	return nil
}

// guardObjectPaths is guardPaths plus a rejection of volume-root aliases. It
// applies to the destructive verbs only - rename and bulk delete - because
// those relocate or destroy the volume root when handed one, whereas reads and
// writes of the volume root merely fail on their own.
func guardObjectPaths(paths ...string) error {
	for _, p := range paths {
		if hasBadPathComponent(p) || isVolumeRootAlias(p) {
			return errFileAccessDenied
		}
	}
	return nil
}

// guardVersions validates every path-bearing field reachable through a
// DeleteVersions payload: the per-object name, and the DataDir of each version.
// DataDir is joined under the object directory, and may legitimately be empty
// for inline and transitioned objects, so it gets guardPaths - never
// guardObjectPaths.
func guardVersions(versions []FileInfoVersions, opts DeleteOptions) error {
	if err := guardPaths(opts.OldDataDir); err != nil {
		return err
	}
	for _, v := range versions {
		if err := guardPaths(v.Name); err != nil {
			return err
		}
		for _, fi := range v.Versions {
			if err := guardPaths(fi.DataDir); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Metadata operations
// ---------------------------------------------------------------------------

func (g guardedStorage) DeleteVersion(ctx context.Context, volume, path string, fi FileInfo, forceDelMarker bool, opts DeleteOptions) error {
	if err := guardPaths(path, fi.DataDir, opts.OldDataDir); err != nil {
		return err
	}
	return g.StorageAPI.DeleteVersion(ctx, volume, path, fi, forceDelMarker, opts)
}

func (g guardedStorage) DeleteVersions(ctx context.Context, volume string, versions []FileInfoVersions, opts DeleteOptions) []error {
	if err := guardVersions(versions, opts); err != nil {
		// The whole batch is refused: validating every element before acting on
		// any of it is what stops a payload mixing a valid target with a
		// malicious one from performing a partial delete.
		errs := make([]error, len(versions))
		for i := range errs {
			errs[i] = err
		}
		return errs
	}
	return g.StorageAPI.DeleteVersions(ctx, volume, versions, opts)
}

func (g guardedStorage) DeleteBulk(ctx context.Context, volume string, paths ...string) error {
	if err := guardObjectPaths(paths...); err != nil {
		return err
	}
	return g.StorageAPI.DeleteBulk(ctx, volume, paths...)
}

func (g guardedStorage) WriteMetadata(ctx context.Context, origvolume, volume, path string, fi FileInfo) error {
	if err := guardPaths(path, fi.DataDir); err != nil {
		return err
	}
	return g.StorageAPI.WriteMetadata(ctx, origvolume, volume, path, fi)
}

func (g guardedStorage) UpdateMetadata(ctx context.Context, volume, path string, fi FileInfo, opts UpdateMetadataOpts) error {
	if err := guardPaths(path, fi.DataDir); err != nil {
		return err
	}
	return g.StorageAPI.UpdateMetadata(ctx, volume, path, fi, opts)
}

func (g guardedStorage) ReadVersion(ctx context.Context, origvolume, volume, path, versionID string, opts ReadOptions) (FileInfo, error) {
	if err := guardPaths(path); err != nil {
		return FileInfo{}, err
	}
	return g.StorageAPI.ReadVersion(ctx, origvolume, volume, path, versionID, opts)
}

func (g guardedStorage) ReadXL(ctx context.Context, volume, path string, readData bool) (RawFileInfo, error) {
	if err := guardPaths(path); err != nil {
		return RawFileInfo{}, err
	}
	return g.StorageAPI.ReadXL(ctx, volume, path, readData)
}

func (g guardedStorage) RenameData(ctx context.Context, srcVolume, srcPath string, fi FileInfo, dstVolume, dstPath string, opts RenameOptions) (RenameDataResp, error) {
	if err := guardObjectPaths(srcPath, dstPath); err != nil {
		return RenameDataResp{}, err
	}
	// DataDir is empty for inline and transitioned objects, so it must not be
	// held to the non-empty rule above.
	if err := guardPaths(fi.DataDir); err != nil {
		return RenameDataResp{}, err
	}
	return g.StorageAPI.RenameData(ctx, srcVolume, srcPath, fi, dstVolume, dstPath, opts)
}

// ---------------------------------------------------------------------------
// File operations
// ---------------------------------------------------------------------------

func (g guardedStorage) ListDir(ctx context.Context, origvolume, volume, dirPath string, count int) ([]string, error) {
	if err := guardPaths(dirPath); err != nil {
		return nil, err
	}
	return g.StorageAPI.ListDir(ctx, origvolume, volume, dirPath, count)
}

func (g guardedStorage) ReadFile(ctx context.Context, volume, path string, offset int64, buf []byte, verifier *BitrotVerifier) (int64, error) {
	if err := guardPaths(path); err != nil {
		return 0, err
	}
	return g.StorageAPI.ReadFile(ctx, volume, path, offset, buf, verifier)
}

func (g guardedStorage) AppendFile(ctx context.Context, volume, path string, buf []byte) error {
	if err := guardPaths(path); err != nil {
		return err
	}
	return g.StorageAPI.AppendFile(ctx, volume, path, buf)
}

func (g guardedStorage) CreateFile(ctx context.Context, origvolume, volume, path string, size int64, reader io.Reader) error {
	if err := guardPaths(path); err != nil {
		return err
	}
	return g.StorageAPI.CreateFile(ctx, origvolume, volume, path, size, reader)
}

func (g guardedStorage) ReadFileStream(ctx context.Context, volume, path string, offset, length int64) (io.ReadCloser, error) {
	if err := guardPaths(path); err != nil {
		return nil, err
	}
	return g.StorageAPI.ReadFileStream(ctx, volume, path, offset, length)
}

func (g guardedStorage) RenameFile(ctx context.Context, srcVolume, srcPath, dstVolume, dstPath string) error {
	if err := guardObjectPaths(srcPath, dstPath); err != nil {
		return err
	}
	return g.StorageAPI.RenameFile(ctx, srcVolume, srcPath, dstVolume, dstPath)
}

func (g guardedStorage) RenamePart(ctx context.Context, srcVolume, srcPath, dstVolume, dstPath string, meta []byte, skipParent string) error {
	if err := guardObjectPaths(srcPath, dstPath); err != nil {
		return err
	}
	if err := guardPaths(skipParent); err != nil {
		return err
	}
	return g.StorageAPI.RenamePart(ctx, srcVolume, srcPath, dstVolume, dstPath, meta, skipParent)
}

func (g guardedStorage) CheckParts(ctx context.Context, volume, path string, fi FileInfo) (*CheckPartsResp, error) {
	if err := guardPaths(path, fi.DataDir); err != nil {
		return nil, err
	}
	if err := guardErasureParams(fi); err != nil {
		return nil, err
	}
	return g.StorageAPI.CheckParts(ctx, volume, path, fi)
}

func (g guardedStorage) Delete(ctx context.Context, volume, path string, opts DeleteOptions) error {
	if err := guardPaths(path, opts.OldDataDir); err != nil {
		return err
	}
	return g.StorageAPI.Delete(ctx, volume, path, opts)
}

func (g guardedStorage) VerifyFile(ctx context.Context, volume, path string, fi FileInfo) (*CheckPartsResp, error) {
	if err := guardPaths(path, fi.DataDir); err != nil {
		return nil, err
	}
	if err := guardErasureParams(fi); err != nil {
		return nil, err
	}
	return g.StorageAPI.VerifyFile(ctx, volume, path, fi)
}

func (g guardedStorage) StatInfoFile(ctx context.Context, volume, path string, glob bool) ([]StatInfo, error) {
	if err := guardPaths(path); err != nil {
		return nil, err
	}
	return g.StorageAPI.StatInfoFile(ctx, volume, path, glob)
}

// ReadParts is a read, so it gets guardPaths rather than guardObjectPaths: the
// volume-root alias rule exists because rename and delete *relocate or destroy*
// the root, whereas a read of it merely fails to find part.N. Applying the
// stricter rule here would widen the false-positive surface for no gain.
func (g guardedStorage) ReadParts(ctx context.Context, bucket string, partMetaPaths ...string) ([]*ObjectPartInfo, error) {
	if err := guardPaths(partMetaPaths...); err != nil {
		return nil, err
	}
	return g.StorageAPI.ReadParts(ctx, bucket, partMetaPaths...)
}

func (g guardedStorage) CleanAbandonedData(ctx context.Context, volume, path string) error {
	if err := guardPaths(path); err != nil {
		return err
	}
	return g.StorageAPI.CleanAbandonedData(ctx, volume, path)
}

func (g guardedStorage) WriteAll(ctx context.Context, volume, path string, b []byte) error {
	if err := guardPaths(path); err != nil {
		return err
	}
	return g.StorageAPI.WriteAll(ctx, volume, path, b)
}

func (g guardedStorage) ReadAll(ctx context.Context, volume, path string) ([]byte, error) {
	if err := guardPaths(path); err != nil {
		return nil, err
	}
	return g.StorageAPI.ReadAll(ctx, volume, path)
}

// ---------------------------------------------------------------------------
// Directory walks and scanning
// ---------------------------------------------------------------------------

func (g guardedStorage) WalkDir(ctx context.Context, opts WalkDirOptions, wr io.Writer) error {
	// FilterPrefix and ForwardTo are only string-compared against readDir
	// output, they never reach a path join.
	if err := guardPaths(opts.Bucket, opts.BaseDir); err != nil {
		return err
	}
	return g.StorageAPI.WalkDir(ctx, opts, wr)
}

// NSScanner is the one StorageAPI method that reaches the filesystem without
// passing through getVolDir: the scanner joins cache.Info.Name onto drivePath
// directly (see scanFolder). It is therefore guarded here rather than at the
// sink. Today an unregistered bucket name is also rejected further down by
// globalBucketObjectLockSys, but that is a lookup in an unrelated subsystem,
// not a containment boundary.
func (g guardedStorage) NSScanner(ctx context.Context, cache dataUsageCache, updates chan<- dataUsageEntry, scanMode madmin.HealScanMode, shouldSleep func() bool) (dataUsageCache, error) {
	if err := guardPaths(cache.Info.Name); err != nil {
		// The caller blocks until updates is closed; NSScanner owns closing it.
		xioutil.SafeClose(updates)
		return cache, err
	}
	return g.StorageAPI.NSScanner(ctx, cache, updates, scanMode, shouldSleep)
}
