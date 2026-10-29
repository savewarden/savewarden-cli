package backup_test

import (
	"testing"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

func TestFileIDRoundTrip(t *testing.T) {
	for _, id := range []backup.FileID{{}, {Volume: 0x1a2b3c4d, Index: 0x0001000000000abc}, {Volume: ^uint32(0), Index: ^uint64(0)}} {
		parsed, err := backup.ParseFileID(id.String())
		if err != nil || parsed != id {
			t.Fatalf("%v: parsed %v, %v", id, parsed, err)
		}
	}
	if got := (backup.FileID{Volume: 0xab, Index: 0xcd}).String(); got != "000000ab-00000000000000cd" {
		t.Fatalf("String = %q", got)
	}
	for _, bad := range []string{"", "000000ab-cd", "000000AB-00000000000000cd", "000000ab_00000000000000cd",
		" 000000ab-00000000000000cd", "000000ab-00000000000000cd\n", "0000000ab-0000000000000cd"} {
		if _, err := backup.ParseFileID(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestDriveNames(t *testing.T) {
	for drive, want := range map[backup.Drive]string{
		backup.DriveUnknown: "unknown", backup.DriveRemovable: "removable", backup.DriveFixed: "fixed",
		backup.DriveRemote: "network", backup.DriveOptical: "optical", backup.DriveRAM: "RAM disk", backup.Drive(42): "unknown",
	} {
		if got := drive.String(); got != want {
			t.Errorf("%d: %q, want %q", drive, got, want)
		}
	}
}
