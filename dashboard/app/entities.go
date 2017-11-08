// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package dash

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/google/syzkaller/dashboard/dashapi"
	"github.com/google/syzkaller/pkg/hash"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

// This file contains definitions of entities stored in datastore.

const (
	maxTextLen   = 200
	MaxStringLen = 1024

	maxCrashes = 20
)

type Build struct {
	Namespace       string
	Manager         string
	ID              string // unique ID generated by syz-ci
	OS              string
	Arch            string
	VMArch          string
	SyzkallerCommit string
	CompilerID      string
	KernelRepo      string
	KernelBranch    string
	KernelCommit    string
	KernelConfig    int64 // reference to KernelConfig text entity
}

type Bug struct {
	Namespace  string
	Seq        int64 // sequences of the bug with the same title
	Title      string
	Status     int
	DupOf      string
	NumCrashes int64
	NumRepro   int64
	ReproLevel dashapi.ReproLevel
	HasReport  bool
	FirstTime  time.Time
	LastTime   time.Time
	Closed     time.Time
	Reporting  []BugReporting
	Commits    []string
	PatchedOn  []string
}

type BugReporting struct {
	Name       string // refers to Reporting.Name
	ID         string // unique ID per BUG/BugReporting used in commucation with external systems
	ExtID      string // arbitrary reporting ID that is passed back in dashapi.BugReport
	Link       string
	CC         string // additional emails added to CC list (|-delimited list)
	ReproLevel dashapi.ReproLevel
	Reported   time.Time
	Closed     time.Time
}

type Crash struct {
	Manager     string
	BuildID     string
	Time        time.Time
	Maintainers []string `datastore:",noindex"`
	Log         int64    // reference to CrashLog text entity
	Report      int64    // reference to CrashReport text entity
	ReproOpts   []byte   `datastore:",noindex"`
	ReproSyz    int64    // reference to ReproSyz text entity
	ReproC      int64    // reference to ReproC text entity
	ReportLen   int
}

// ReportingState holds dynamic info associated with reporting.
type ReportingState struct {
	Entries []ReportingStateEntry
}

type ReportingStateEntry struct {
	Namespace string
	Name      string
	// Current reporting quota consumption.
	Sent int
	Date int
}

// Text holds text blobs (crash logs, reports, reproducers, etc).
type Text struct {
	Namespace string
	Text      []byte `datastore:",noindex"` // gzip-compressed text
}

const (
	BugStatusOpen = iota
)

const (
	BugStatusFixed = 1000 + iota
	BugStatusInvalid
	BugStatusDup
)

const (
	ReproLevelNone = dashapi.ReproLevelNone
	ReproLevelSyz  = dashapi.ReproLevelSyz
	ReproLevelC    = dashapi.ReproLevelC
)

func buildKey(c context.Context, ns, id string) *datastore.Key {
	if ns == "" {
		panic("requesting build key outside of namespace")
	}
	h := hash.String([]byte(fmt.Sprintf("%v-%v", ns, id)))
	return datastore.NewKey(c, "Build", h, 0, nil)
}

func loadBuild(c context.Context, ns, id string) (*Build, error) {
	build := new(Build)
	if err := datastore.Get(c, buildKey(c, ns, id), build); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, fmt.Errorf("unknown build %v/%v", ns, id)
		}
		return nil, fmt.Errorf("failed to get build %v/%v: %v", ns, id, err)
	}
	return build, nil
}

func (bug *Bug) displayTitle() string {
	if bug.Seq == 0 {
		return bug.Title
	}
	return fmt.Sprintf("%v (%v)", bug.Title, bug.Seq+1)
}

var displayTitleRe = regexp.MustCompile("^(.*) \\(([0-9]+)\\)$")

func splitDisplayTitle(display string) (string, int64, error) {
	match := displayTitleRe.FindStringSubmatchIndex(display)
	if match == nil {
		return display, 0, nil
	}
	title := display[match[2]:match[3]]
	seqStr := display[match[4]:match[5]]
	seq, err := strconv.ParseInt(seqStr, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse bug title: %v", err)
	}
	if seq <= 0 || seq > 1e6 {
		return "", 0, fmt.Errorf("failed to parse bug title: seq=%v", seq)
	}
	return title, seq - 1, nil
}

func canonicalBug(c context.Context, bug *Bug) (*Bug, error) {
	for {
		if bug.Status != BugStatusDup {
			return bug, nil
		}
		canon := new(Bug)
		bugKey := datastore.NewKey(c, "Bug", bug.DupOf, 0, nil)
		if err := datastore.Get(c, bugKey, canon); err != nil {
			return nil, fmt.Errorf("failed to get dup bug %q for %q: %v",
				bug.DupOf, bugKeyHash(bug.Namespace, bug.Title, bug.Seq), err)
		}
		bug = canon
	}
}

func bugKeyHash(ns, title string, seq int64) string {
	return hash.String([]byte(fmt.Sprintf("%v-%v-%v-%v", config.Namespaces[ns].Key, ns, title, seq)))
}

func bugReportingHash(bugHash, reporting string) string {
	return hash.String([]byte(fmt.Sprintf("%v-%v", bugHash, reporting)))
}

func textLink(tag string, id int64) string {
	if id == 0 {
		return ""
	}
	return fmt.Sprintf("/text?tag=%v&id=%v", tag, id)
}
