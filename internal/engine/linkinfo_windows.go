//go:build windows

package engine

import "io/fs"

// linkInfo: the NTFS file record is not exposed through FileInfo, so link
// identity stays unknown and nothing is deduped.
func linkInfo(fs.FileInfo) (uint64, uint64) { return 0, 1 }

// canFingerprintLinks records that limitation for callers that promise
// du-exact sizes.
const canFingerprintLinks = false
