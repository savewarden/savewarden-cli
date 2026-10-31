package sync

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

// copyBuffer bounds the memory of one copy.
const copyBuffer = 1 << 20

// stage creates staging\<op> after its intent, copies every planned
// source into it and records staged (plan B2 §4.3, phase 4).
func (o *operation) stage() error {
	if err := o.record(backup.StepStagingIntent, backup.StepValues{}); err != nil {
		return err
	}
	if err := o.ensureDir(o.game + "/" + o.domain.stagings); err != nil {
		return err
	}
	if err := o.vault.CreateDir(o.stagingDir()); err != nil {
		return fmt.Errorf("staging: %w", err)
	}
	for i, source := range o.plan.Sources {
		if source.Status != SourcePlanned {
			continue
		}
		if err := o.copySource(i, source); err != nil {
			return err
		}
	}
	id, err := o.vault.Identity(o.stagingDir())
	if err != nil {
		return fmt.Errorf("staging identity: %w", err)
	}
	digest, err := backup.ContentDigest(o.content)
	if err != nil {
		return err
	}
	o.staging = id
	return o.record(backup.StepStaged, backup.StepValues{Staging: id, Digest: digest})
}

// copySource recreates the directories of one source in staging, then
// copies its files.
func (o *operation) copySource(index int, source SourcePlan) error {
	volume, segments, err := o.plan.manifest.SourceSegments(index)
	if err != nil {
		return err
	}
	scope, err := o.s.files.OpenScope(volume)
	if err != nil {
		return fmt.Errorf("source %s: open volume %s: %w", source.Source.ID, volume, err)
	}
	defer func() { _ = scope.Close() }()
	base := strings.Join(segments, "/")
	target := o.stagingDir() + "/" + source.Source.ID
	if err := o.vault.CreateDir(target); err != nil {
		return fmt.Errorf("staging: %w", err)
	}
	for _, dir := range source.inventory.Directories {
		if err := o.vault.CreateDir(target + "/" + dir); err != nil {
			return fmt.Errorf("staging: %w", err)
		}
	}
	for _, entry := range source.inventory.Entries {
		if err := o.ctx.Err(); err != nil {
			return fmt.Errorf("interrupted: %w", err)
		}
		if err := o.copyFile(scope, source.Source.ID, join(base, entry.Path), target+"/"+entry.Path, entry); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies one source file to a new staging file. The file must be
// the one planned at its opening, before and after reading, and its bytes
// must hash to the planned SHA-256 (plan B2 §4.3).
func (o *operation) copyFile(scope Scope, sourceID, from, to string, want backup.Entry) error {
	changed := func(why string) error {
		return &BlockedError{Code: o.domain.changed, Detail: fmt.Sprintf("source %s, %s: %s since the plan", sourceID, want.Path, why)}
	}
	same := func(info fs.FileInfo) bool {
		return info.Mode().IsRegular() && info.Size() == want.Size && info.ModTime().Equal(want.ModTime)
	}
	before, err := scope.Lstat(from)
	if errors.Is(err, fs.ErrNotExist) {
		return changed("removed")
	}
	if err != nil {
		return fmt.Errorf("source %s, %s: %w", sourceID, want.Path, err)
	}
	if !same(before) {
		return changed("changed")
	}
	file, err := scope.OpenRegular(from)
	if err != nil {
		return fmt.Errorf("source %s, %s: %w", sourceID, want.Path, err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("source %s, %s: %w", sourceID, want.Path, err)
	}
	if !scope.SameFile(before, opened) || !same(opened) {
		return changed("replaced")
	}
	out, err := o.vault.NewFile(to, want.ModTime)
	if err != nil {
		return fmt.Errorf("staging %s: %w", to, err)
	}
	hash := sha256.New()
	read, copyErr := copyHashed(out, hash, file, want.Size)
	if closeErr := out.Close(); copyErr == nil && closeErr != nil {
		copyErr = &writeError{closeErr}
	}
	var write *writeError
	switch {
	case errors.As(copyErr, &write):
		return fmt.Errorf("staging %s: %w", to, write.err)
	case copyErr != nil:
		return fmt.Errorf("source %s, %s: %w", sourceID, want.Path, copyErr)
	}
	after, err := file.Stat()
	if err != nil {
		return fmt.Errorf("source %s, %s: %w", sourceID, want.Path, err)
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	if read != want.Size || !same(after) || sum != want.SHA256 {
		return changed("changed during the copy")
	}
	return nil
}

// writeError marks a failure to write the vault, as opposed to reading
// the source.
type writeError struct{ err error }

func (e *writeError) Error() string { return e.err.Error() }

// copyHashed copies at most limit+1 bytes from src to dst and hash, and
// returns the number of bytes read.
func copyHashed(dst, hash io.Writer, src io.Reader, limit int64) (int64, error) {
	buffer := make([]byte, min(copyBuffer, limit+1))
	var total int64
	limited := io.LimitReader(src, limit+1)
	for {
		n, err := limited.Read(buffer)
		if n > 0 {
			total += int64(n)
			_, _ = hash.Write(buffer[:n])
			if _, werr := dst.Write(buffer[:n]); werr != nil {
				return total, &writeError{werr}
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// recheck inventories every source again: it must be exactly the plan
// (plan B2 §4.3, phase 5), which covers files added, removed or changed
// elsewhere during the copy.
func (o *operation) recheck() error {
	for i, source := range o.plan.Sources {
		volume, segments, err := o.plan.manifest.SourceSegments(i)
		if err != nil {
			return err
		}
		dir, err := o.s.inventories.Directory(o.ctx, volume, segments)
		if err != nil {
			return err
		}
		if source.Status == SourceMissing {
			if dir.Status != backup.StatusMissing {
				return &BlockedError{Code: CodeChangedDuringSync, Detail: fmt.Sprintf("source %s appeared since the plan", source.Source.ID)}
			}
			continue
		}
		if !sameInventory(dir, source.inventory) {
			return &BlockedError{Code: CodeChangedDuringSync, Detail: fmt.Sprintf("source %s changed during the copy", source.Source.ID)}
		}
	}
	return nil
}

func sameInventory(a, b backup.Directory) bool {
	return a.Status == b.Status && len(a.Findings) == 0 && len(b.Findings) == 0 &&
		len(a.Encrypted) == 0 && len(b.Encrypted) == 0 &&
		slices.Equal(a.Directories, b.Directories) &&
		slices.EqualFunc(a.Entries, b.Entries, func(x, y backup.Entry) bool {
			return x.Path == y.Path && x.Size == y.Size && x.ModTime.Equal(y.ModTime) && x.SHA256 == y.SHA256
		})
}

// verify reads the staging back from disk through a read-only, no-follow
// walk, closed before returning: exactly the planned paths and
// directories, sizes, dates and SHA-256 (plan B2 §4.3, phase 6).
func (o *operation) verify() error {
	segments := append(append([]string{}, o.plan.vaultSegments...), strings.Split(o.stagingDir(), "/")...)
	dir, err := o.s.inventories.Directory(o.ctx, o.plan.vaultVolume, segments)
	if err != nil {
		return err
	}
	receipt := backup.Receipt{Sources: o.content}
	if differences := compareWithReceipt(dir, receipt); len(differences) > 0 {
		first := differences[0]
		return &BlockedError{Code: CodeVerificationMismatch,
			Detail: fmt.Sprintf("staging %s %s: %s; %d difference(s) in all", orDash(first.source), orDash(first.path),
				strings.ReplaceAll(strings.ReplaceAll(first.detail, "its receipt", "the plan"), "current", "staging"), len(differences))}
	}
	digest, err := backup.ContentDigest(o.content)
	if err != nil {
		return err
	}
	return o.record(backup.StepVerified, backup.StepValues{Digest: digest})
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
