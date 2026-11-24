package envprofile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/relux-works/curator/internal/envregistry"
	"github.com/relux-works/curator/internal/stateread"
)

type sharedFileAuthState struct {
	live bool
	err  error
}

// inspectSharedFileAuth inspects a file-linked credential entry without ever
// reading credential bytes. A fork is a conflict, an observation failure is
// unreadable, and an absent native target fails closed with absentHint.
func (req *ResolveRequest) inspectSharedFileAuth(full, target, absentHint string) sharedFileAuthState {
	unreadable := func(err error) sharedFileAuthState {
		return sharedFileAuthState{err: fmt.Errorf("%s: %s: %v", envregistry.DiagPassthroughUnreadable, full, err)}
	}
	conflict := func(reason string) sharedFileAuthState {
		return sharedFileAuthState{err: fmt.Errorf("%s: %s: %s", envregistry.DiagCredentialConflict, full, reason)}
	}
	root := EnvRoot(req.Home)
	parentRel, err := managedRelative(root, filepath.Dir(full))
	if err != nil {
		return unreadable(err)
	}
	if err := checkManagedPrivateTarget(root, parentRel); err != nil {
		return unreadable(err)
	}
	var entry stateread.Metadata
	if req.passthroughLstat == nil {
		entry, err = stateread.Lstat(full)
	} else {
		entry, err = stateread.LstatWith(full, req.passthroughLstat)
	}
	if err != nil {
		return unreadable(err)
	}
	live := false
	if entry.Kind == stateread.KindPresent {
		if entry.Info == nil {
			return unreadable(fmt.Errorf("missing metadata"))
		}
		switch {
		case entry.Info.Mode()&os.ModeSymlink != 0:
			var link stateread.Link
			if req.passthroughReadlink == nil {
				link, err = stateread.Readlink(full)
			} else {
				link, err = stateread.ReadlinkWith(full, req.passthroughReadlink)
			}
			if err != nil {
				return unreadable(err)
			}
			if link.Kind != stateread.KindPresent {
				return unreadable(fmt.Errorf("link changed during inspection"))
			}
			got := link.Target
			if !filepath.IsAbs(got) {
				got = filepath.Join(filepath.Dir(full), got)
			}
			if filepath.Clean(got) != filepath.Clean(target) {
				return conflict("unexpected credential link target")
			}
			live = true
		case entry.Info.IsDir():
			directory, err := stateread.ReadDir(full)
			if err != nil {
				return unreadable(err)
			}
			if len(directory.Entries) != 0 {
				return conflict("non-empty credential directory")
			}
		default:
			return conflict("credential link replaced by a file; preserve both stores")
		}
	} else if entry.Kind != stateread.KindAbsent {
		return unreadable(fmt.Errorf("unknown entry state %q", entry.Kind))
	}
	metadata, err := stateread.StatWith(target, req.museTargetStat)
	if err != nil {
		return unreadable(err)
	}
	if metadata.Kind == stateread.KindAbsent {
		if absentHint == "" {
			absentHint = "native credential target is absent; authenticate in the native home before resolving shared credentials"
		}
		return conflict(absentHint)
	}
	if metadata.Kind != stateread.KindPresent || metadata.Info == nil {
		return unreadable(fmt.Errorf("native target metadata unavailable"))
	}
	if !metadata.Info.Mode().IsRegular() {
		return conflict("native credential target is not a regular file")
	}
	if err := stateread.CheckReadableRegularFile(target); err != nil {
		return unreadable(err)
	}
	return sharedFileAuthState{live: live}
}
