package sync

import (
	"fmt"
	"math"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// minimumMargin is the smallest free space kept beyond a generation; it
// also covers the journal and the receipt (plan B2 §4.2, V2).
const minimumMargin = 64 << 20

// qualify accepts local NTFS volumes only (D11): ReFS, FAT32, exFAT and
// network, optical or RAM drives are refused before anything is written.
func qualify(vaultPath string, volume backup.Volume) error {
	if volume.FileSystem != "NTFS" {
		return errVault(vaultPath, "volume file system is %q; only NTFS is qualified", volume.FileSystem)
	}
	if volume.Drive != backup.DriveFixed && volume.Drive != backup.DriveRemovable {
		return errVault(vaultPath, "volume is on a %s drive; only local fixed or removable drives are qualified", volume.Drive)
	}
	return nil
}

// spaceNeeded is the full size of the new generation plus max(64 MiB, 5 %),
// computed without overflow.
func spaceNeeded(sizes []int64) (uint64, error) {
	var total uint64
	for _, size := range sizes {
		if size < 0 || uint64(size) > math.MaxUint64-total {
			return 0, fmt.Errorf("generation size overflows 64 bits")
		}
		total += uint64(size)
	}
	margin := total/20 + boolToUint(total%20 != 0)
	if margin < minimumMargin {
		margin = minimumMargin
	}
	if margin > math.MaxUint64-total {
		return 0, fmt.Errorf("generation size overflows 64 bits")
	}
	return total + margin, nil
}

func boolToUint(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}
