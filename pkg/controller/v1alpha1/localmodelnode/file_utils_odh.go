//go:build distro

/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package localmodelnode

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// isModelRootWritable checks if the model root folder is writable.
// Defined as a var for test overriding.
var isModelRootWritable = func() bool {
	file, err := os.CreateTemp(modelsRootFolder, ".write-test-*")
	if err != nil {
		return false
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return true
}

// mcsLevelIsCategorized reports whether an SELinux MCS level carries categories. The level is
// "<sensitivity>[:categories]"; categories, when present, follow a colon (e.g. "s0:c2,c28"),
// while a bare "s0" or an "s0-s0" range carries none.
func mcsLevelIsCategorized(level string) bool {
	return strings.Contains(level, ":")
}

// selinuxMCSLevel reads the MCS level of path from its security.selinux xattr. ok is false when
// the node/filesystem carries no SELinux label (ENODATA / ENOTSUP) so callers do not treat a
// non-SELinux node as needing a relabel.
func selinuxMCSLevel(path string) (level string, ok bool, err error) {
	// Query the required size first (dest == nil returns the length) — a context with many
	// explicitly-listed categories can exceed a small fixed buffer and would otherwise return
	// ERANGE, which is not ENODATA/ENOTSUP and would be misread as an unexpected error.
	size, err := unix.Lgetxattr(path, "security.selinux", nil)
	if err != nil {
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			return "", false, nil
		}
		return "", false, err
	}
	buf := make([]byte, size)
	n, err := unix.Lgetxattr(path, "security.selinux", buf)
	if err != nil {
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			return "", false, nil
		}
		return "", false, err
	}
	return parseMCSLevel(string(buf[:n])), true, nil
}

// folderHasNonSharedMCS reports whether the model directory at path, or any descendant, carries
// SELinux MCS *categories* (i.e. a level other than the shared, category-less floor). Anything
// categorized is unreadable from a consuming namespace whose MCS category set does not dominate
// it, so it must be relabeled to the shared level. Defined as a var so tests can inject behavior
// without a real SELinux filesystem.
//
// The whole tree is walked, not just the directory or its immediate children, because SELinux
// assigns a newly-created file the *creating process's* MCS level, not the parent directory's:
// a re-download into a previously-relabeled (s0) directory writes categorized files under an s0
// dir, and storage-initializer preserves nested object paths, so a categorized file can appear
// arbitrarily deep. The walk exits early (filepath.SkipAll) on the first categorized entry, so
// the common (healthy) cost is a full traversal while a problem is found on the first offender.
//
// Returns (false, nil) when SELinux is disabled or the filesystem does not support labels
// (ENODATA / ENOTSUP), so non-SELinux nodes never trigger a relabel loop.
var folderHasNonSharedMCS = func(path string) (bool, error) {
	categorized := false
	walkErr := filepath.WalkDir(path, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		level, ok, lvlErr := selinuxMCSLevel(p)
		if lvlErr != nil {
			return lvlErr
		}
		if ok && mcsLevelIsCategorized(level) {
			categorized = true
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return false, walkErr
	}
	return categorized, nil
}
