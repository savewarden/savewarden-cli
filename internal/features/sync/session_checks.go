package sync

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
	"github.com/savewarden/savewarden-cli/internal/entities/game"
)

// sessionPlan is what a plan observed of the target it would write below and
// of the references the target must stay apart from, kept in memory for the
// checks under the session (ADR 0006 §11, plan B5 §4.3). The journal format
// does not change: a recovery observes again in its own plan.
type sessionPlan struct {
	// target is the chain of the target from the root of its volume, root
	// included, down to the target, or to its parent while it is absent.
	target observedChain
	refs   []plannedReference
	// marker names the marker the plan read at the root of the target,
	// relative to it, and markerSum its SHA-256; empty when none was read.
	marker    string
	markerSum [32]byte
	// ancestorMarkers requires that no ancestor of the target hold the
	// marker of a vault or of a restore area: a restore area.
	ancestorMarkers bool
}

// plannedReference is a reference as a plan observed it: its chain from the
// root of its volume, or that its drive had no volume, or why it could not
// be observed.
type plannedReference struct {
	reference
	chain    observedChain
	noVolume bool
	err      error
}

// same reports whether two observations of one reference agree.
func (r plannedReference) same(other plannedReference) bool {
	if r.noVolume != other.noVolume || (r.err == nil) != (other.err == nil) || r.chain.complete != other.chain.complete ||
		len(r.chain.ids) != len(other.chain.ids) {
		return false
	}
	for i, id := range r.chain.ids {
		if other.chain.ids[i] != id {
			return false
		}
	}
	return true
}

// observeTarget reads, without following anything, the chain of the
// directory at volume and segments, through the plan's read-only adapters.
func (s *Service) observeTarget(volume string, segments []string) (observedChain, error) {
	scope, err := s.files.OpenScope(volume)
	if err != nil {
		return observedChain{}, err
	}
	defer func() { _ = scope.Close() }()
	return chainIdentities(scope, segments)
}

// observeReferences reads the chain of every reference, through the plan's
// read-only adapters, one scope per volume.
func (s *Service) observeReferences(refs []reference) []plannedReference {
	scopes := map[string]Scope{}
	missing := map[string]bool{}
	defer func() {
		for _, scope := range scopes {
			_ = scope.Close()
		}
	}()
	planned := make([]plannedReference, 0, len(refs))
	for _, ref := range refs {
		p := plannedReference{reference: ref}
		key := strings.ToUpper(ref.volume)
		scope, opened := scopes[key]
		if !opened && !missing[key] {
			var err error
			scope, err = s.files.OpenScope(ref.volume)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				missing[key] = true
			case err != nil:
				p.err = err
			default:
				scopes[key] = scope
			}
		}
		switch {
		case missing[key]:
			p.noVolume = true
		case p.err == nil:
			p.chain, p.err = chainIdentities(scope, ref.segments)
		}
		planned = append(planned, p)
	}
	return planned
}

// sessionPlanOf observes the target and the references for the checks under
// the session. A target whose chain cannot be observed is refused by the
// caller's plan before this; here it is an error all the same.
func (s *Service) sessionPlanOf(volume string, segments []string, refs []reference) (sessionPlan, error) {
	chain, err := s.observeTarget(volume, segments)
	if err != nil {
		return sessionPlan{}, err
	}
	return sessionPlan{target: chain, refs: s.observeReferences(refs)}, nil
}

// vaultSessionPlan is the sessionPlan of an operation that writes in the
// vault at vaultPath: its references are the installation and the sources
// of manifest, and the marker of an initialized vault is read again.
func (s *Service) vaultSessionPlan(vaultPath, volume string, segments []string, manifest game.Manifest, v *vault) (sessionPlan, error) {
	p, err := s.sessionPlanOf(volume, segments, manifestReferences(manifest))
	if err != nil {
		return sessionPlan{}, errVault(vaultPath, "cannot be observed for the checks under the session: %v", err)
	}
	if v.marker != nil {
		p = p.withMarker(backup.MarkerName, v.marker)
	}
	return p, nil
}

// areaSessionPlan is the sessionPlan of an operation that writes in the
// restore area at path, separated from refs: no ancestor of the area may be
// marked, and the marker of a marked area is read again.
func (s *Service) areaSessionPlan(path, volume string, segments []string, refs []reference, a *area) (sessionPlan, error) {
	p, err := s.sessionPlanOf(volume, segments, refs)
	if err != nil {
		return sessionPlan{}, errDestination(path, "cannot be observed for the checks under the session: %v", err)
	}
	p.ancestorMarkers = true
	if a != nil && a.markerData != nil {
		p = p.withMarker(backup.RestoreAreaMarkerName, a.markerData)
	}
	return p, nil
}

// withMarker records the marker the plan read at the root of the target.
func (p sessionPlan) withMarker(name string, data []byte) sessionPlan {
	p.marker, p.markerSum = name, sha256.Sum256(data)
	return p
}

// checkUnderSession applies the checks of ADR 0006 §11 once the session is
// open and before it writes anything (plan B5 §4.3), then keeps what they
// validated as the trusted observations of the operation:
//
//   - the held chain is read on the session's own handles; if it cannot be
//     read, the checks stop at once, and nothing else is read or compared;
//   - step 2 compares it with the chain of the plan, without any read;
//   - step 3 reads every reference again, through the plan's adapters;
//   - step 4 searches the overlaps, without any read, on the held chain and
//     on the references as read now; a reference that cannot be observed, or
//     whose drive has no volume, is never searched: step 3 refuses it;
//   - steps 2 to 4 all run, in that order. When several refuse, the overlap
//     is reported first, then the held chain, then the reference: the
//     overlap names the reference at stake and keeps the message of B4
//     (plan B5 §7.2). Only the report is ordered so: any refusal stops the
//     operation, code 1, before it writes anything;
//   - step 5, the decision reads through the session, runs only when steps
//     2 to 4 refused nothing.
func (o *operation) checkUnderSession(p sessionPlan) error {
	held, err := o.vault.Chain()
	if err != nil {
		return o.target.errorf("cannot be checked under the session: %v", err)
	}
	if err := o.checkHeld(p, p.target, held); err != nil {
		return err
	}
	if err := o.decisionReads(p); err != nil {
		return err
	}
	o.trusted, o.checked = p, true
	return nil
}

// checkHeld runs steps 2 to 4 of plan B5 §4.3: the held chain against want,
// the references of p read again, the overlaps on the held chain. The
// overlap is reported first, as checkUnderSession says.
func (o *operation) checkHeld(p sessionPlan, want observedChain, held []backup.FileID) error {
	chainErr := o.sameHeldChain(want, held)
	refs, refErr := o.referencesAgain(p.refs)
	if err := o.separated(refs, held, want.complete && chainErr == nil); err != nil {
		return err
	}
	if chainErr != nil {
		return chainErr
	}
	return refErr
}

// sameHeldChain requires the held chain to be the plan's: the same
// identities from the volume root down to the target, or down to its parent
// while it is absent. A target absent from the plan and held by the session
// appeared meanwhile.
func (o *operation) sameHeldChain(planned observedChain, held []backup.FileID) error {
	changed := func(why string) error {
		return o.target.errorf("%s-changed: %s; run %s again", o.target.changedCode(), why, o.domain.command)
	}
	switch {
	case !planned.complete && len(held) == len(planned.ids)+1:
		return changed("it appeared since the plan")
	case planned.complete && len(held) == len(planned.ids)-1:
		return changed("it disappeared since the plan")
	case len(held) != len(planned.ids):
		return changed("the session holds other directories than the plan read")
	}
	for i, id := range planned.ids {
		if held[i] != id {
			return changed("the session holds other directories than the plan read")
		}
	}
	return nil
}

// referencesAgain reads every reference again, through the plan's
// read-only adapters, and requires each to be as the plan observed it. It
// returns the observations made now, for the overlaps, and the first
// difference.
func (o *operation) referencesAgain(planned []plannedReference) ([]plannedReference, error) {
	refs := make([]reference, len(planned))
	for i, p := range planned {
		refs[i] = p.reference
	}
	again := o.s.observeReferences(refs)
	var first error
	for i, now := range again {
		var why error
		switch {
		case planned[i].err != nil || now.err != nil:
			cause := now.err
			if cause == nil {
				cause = planned[i].err
			}
			why = o.target.errorf("separation from the %s %s cannot be proven under the session: %v", now.label, now.path, cause)
		case !planned[i].same(now):
			why = o.target.errorf("the %s %s changed since the plan; run %s again", now.label, now.path, o.domain.command)
		}
		if first == nil {
			first = why
		}
	}
	return again, first
}

// separated applies step 4 of plan B5 §4.3 to the held chain: the identity
// of no existing reference lies in it, whether it ends at the target or at
// its parent; and, when targetHeld says it ends at the target, the target
// lies in no reference chain. The parent of an absent target only anchors
// it: it is never searched in a reference, being the common ancestor of
// sibling references. A reference that cannot be observed, or whose drive
// has no volume, is skipped: nothing is concluded from it here.
func (o *operation) separated(refs []plannedReference, held []backup.FileID, targetHeld bool) error {
	contains := func(ids []backup.FileID, id backup.FileID) bool {
		for _, known := range ids {
			if known == id {
				return true
			}
		}
		return false
	}
	for _, ref := range refs {
		if ref.err != nil || ref.noVolume {
			continue
		}
		if ref.chain.complete && contains(held, ref.chain.ids[len(ref.chain.ids)-1]) {
			return o.target.errorf("%s", o.target.overlap(ref.reference))
		}
		if targetHeld && len(held) > 0 && contains(ref.chain.ids, held[len(held)-1]) {
			return o.target.errorf("%s", o.target.overlap(ref.reference))
		}
	}
	return nil
}

// decisionReads makes, through the session and without following anything,
// the reads the decision rests on (plan B5 §4.3, step 5): an absent target
// still absent below its held parent, no ancestor marked as a vault or a
// restore area, and the marker read again.
func (o *operation) decisionReads(p sessionPlan) error {
	if !p.target.complete {
		_, err := o.vault.Stat(".")
		switch {
		case err == nil:
			return o.target.errorf("%s-changed: it appeared since the plan; run %s again", o.target.changedCode(), o.domain.command)
		case !errors.Is(err, fs.ErrNotExist):
			return o.target.errorf("cannot be checked under the session: %v", err)
		}
	}
	if p.ancestorMarkers {
		depth, err := o.vault.MarkedAncestor(backup.MarkerName, backup.RestoreAreaMarkerName)
		switch {
		case err != nil:
			return o.target.errorf("cannot be checked under the session: %v", err)
		case depth >= 0:
			return o.target.errorf("lies in a Savewarden vault or restore area, %s; choose a destination outside it",
				windowsPath(o.target.volume, o.target.segments[:depth]))
		}
	}
	if p.marker != "" {
		data, err := o.vault.ReadFile(p.marker, markerReadLimit)
		switch {
		case err != nil:
			return o.target.errorf("its marker cannot be read again under the session: %v", err)
		case sha256.Sum256(data) != p.markerSum:
			return o.target.errorf("%s-changed: its marker changed since the plan; run %s again", o.target.changedCode(), o.domain.command)
		}
	}
	return nil
}

// checkCreatedRoot resumes the checks once the session created the absent
// root of its target, after the first checks succeeded and before any other
// mutation: the held chain must be the trusted one, extended by the root
// just created; the references, read again, are still the plan's; neither
// the new root lies in a reference nor a reference in it; and the decision
// reads that still apply, the marked ancestors among them, are made again.
// The extended chain becomes the trusted one. A refusal leaves the root in
// place, named, and Savewarden writes nothing else in it (plan B5 §4.1,
// precision A); what another process may have added is not judged.
func (o *operation) checkCreatedRoot() error {
	stays := func(err error) error {
		return fmt.Errorf("%w; the directory %s this run created stays in place, nothing else was written", err, o.target.path)
	}
	if !o.checked {
		return stays(o.target.errorf("the checks under the session did not run before the creation"))
	}
	held, err := o.vault.Chain()
	if err != nil {
		return stays(o.target.errorf("cannot be checked under the session: %v", err))
	}
	if len(held) == 0 {
		return stays(o.target.errorf("cannot be checked under the session: the session holds nothing"))
	}
	extended := observedChain{ids: append(append([]backup.FileID(nil), o.trusted.target.ids...), held[len(held)-1]), complete: true}
	if err := o.checkHeld(o.trusted, extended, held); err != nil {
		return stays(err)
	}
	// The decision reads that still apply to a target that now exists: the
	// marked ancestors, before any marker, lock or other mutation. Its
	// absence is not read again: this run just created it.
	resumed := o.trusted
	resumed.target = extended
	if err := o.decisionReads(resumed); err != nil {
		return stays(err)
	}
	o.trusted = resumed
	return nil
}

// trustMarker records the marker this run validated at the root of its
// target: the checks under the lock read it again, and a marker this run
// wrote is never taken for a change.
func (o *operation) trustMarker(name string, data []byte) {
	o.trusted = o.trusted.withMarker(name, data)
}

// recheckUnderLock repeats steps 2 to 4 and the decision reads under the
// lock, on the trusted observations only: the chain the checks validated
// before the first write, extended by the root this run created, the
// references as the plan observed them, and the marker the plan read or
// this run validated. A plan read again under the lock never replaces
// them: a letter remapped meanwhile is refused, never adopted as a new
// reference. The plan requires it at Z6 of a restore and for the recovery
// of a restore operation (plan B5 §4.3: the checks of Z2 to Z6 pass on the
// held chains). A refusal writes nothing more; the marker or the lock this
// run wrote before stays.
func (o *operation) recheckUnderLock() error {
	underLock := func(err error) error {
		return fmt.Errorf("%w; refused under the lock, nothing more is written", err)
	}
	if !o.checked {
		return underLock(o.target.errorf("the checks under the session did not run before the lock"))
	}
	held, err := o.vault.Chain()
	if err != nil {
		return underLock(o.target.errorf("cannot be checked under the session: %v", err))
	}
	if err := o.checkHeld(o.trusted, o.trusted.target, held); err != nil {
		return underLock(err)
	}
	if err := o.decisionReads(o.trusted); err != nil {
		return underLock(err)
	}
	return nil
}

// heldEntries lists the held directory dir through the session, without
// following anything, each entry described by Stat as list describes the
// entries a plan reads, links, reparse points and placeholders being of
// no plain kind, and sorted by name as list sorts them.
func (o *operation) heldEntries(dir string) ([]backup.NamedEntry, error) {
	dirEntries, err := o.vault.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]backup.NamedEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		mode, err := o.vault.Stat(join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		kind := backup.KindOther
		switch {
		case isLink(mode):
		case mode.IsDir():
			kind = backup.KindDirectory
		case mode.IsRegular():
			kind = backup.KindRegular
		}
		entries = append(entries, backup.NamedEntry{Name: entry.Name(), Kind: kind})
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name < entries[b].Name })
	return entries, nil
}

// sameReferences reports whether two lists hold the same references, each
// as many times, whatever their order: a list built from a map has none.
func sameReferences(a, b []reference) bool {
	keys := func(refs []reference) []string {
		out := make([]string, len(refs))
		for i, ref := range refs {
			out[i] = ref.label + "\x00" + ref.path
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(keys(a), keys(b))
}

// sameTaken reports whether two sets of attempt numbers are the same, every
// number included, not only as many.
func sameTaken(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, taken := range a {
		if taken != b[k] {
			return false
		}
	}
	return true
}

// changedCode is the code a changed target reports, as the plans of its
// command already name it.
func (t target) changedCode() string {
	if t.label == "destination" {
		return "destination"
	}
	return "vault"
}

// overlap words a target that coincides with, contains or lies in ref.
func (t target) overlap(ref reference) string {
	if t.label == "destination" {
		return fmt.Sprintf("overlaps the %s %s; a restore never lands in the vault, the installation or a source", ref.label, ref.path)
	}
	return fmt.Sprintf("overlaps the %s %s; the vault must be distinct from the installation and every source", ref.label, ref.path)
}
