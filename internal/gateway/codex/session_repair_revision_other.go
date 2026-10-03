//go:build !linux && !darwin && !windows

package codex

import "os"

func sessionRepairPlatformRevision(os.FileInfo) [3]int64 { return [3]int64{} }
