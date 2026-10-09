package migrations

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/catalog"
)

// RealDataFile is the embedded catalogue of real restaurants around Mons, in
// the admin import format (a JSON array of RestaurantImport). Unknown keys
// such as "source_urls" or "menu_checked_at" are ignored.
const RealDataFile = "data/mons_restaurants.json"

// UberEatsSnapshotFile is the Uber Eats snapshot read by the sync source
// "ubereats-snapshot" (docs/ARCHITECTURE.md, feedsync.SnapshotEntry[]). The
// lead fills it from a Claude session (Uber Eats connector); "[]" = empty.
const UberEatsSnapshotFile = "data/mons_ubereats.json"

//go:embed data/mons_restaurants.json data/mons_ubereats.json
var dataFS embed.FS

var (
	snapshotMu  sync.RWMutex
	snapshotSet bool
	snapshot    []byte
)

// SetUberEatsSnapshotForTesting replaces the embedded Uber Eats snapshot and
// returns a function restoring the embedded file. Tests only.
func SetUberEatsSnapshotForTesting(data []byte) (restore func()) {
	snapshotMu.Lock()
	prevSet, prev := snapshotSet, snapshot
	snapshotSet, snapshot = true, data
	snapshotMu.Unlock()
	return func() {
		snapshotMu.Lock()
		snapshotSet, snapshot = prevSet, prev
		snapshotMu.Unlock()
	}
}

// UberEatsSnapshot returns the embedded Uber Eats snapshot (nil when the
// file is missing).
func UberEatsSnapshot() ([]byte, error) {
	snapshotMu.RLock()
	defer snapshotMu.RUnlock()
	if snapshotSet {
		return snapshot, nil
	}
	b, err := fs.ReadFile(dataFS, UberEatsSnapshotFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

var (
	overrideMu  sync.RWMutex
	overrideSet bool
	override    []byte
)

// SetRealDataForTesting replaces the embedded real data (nil = file missing)
// and returns a function restoring the embedded file. Tests only.
func SetRealDataForTesting(data []byte) (restore func()) {
	overrideMu.Lock()
	prevSet, prev := overrideSet, override
	overrideSet, override = true, data
	overrideMu.Unlock()
	return func() {
		overrideMu.Lock()
		overrideSet, override = prevSet, prev
		overrideMu.Unlock()
	}
}

func realDataBytes() ([]byte, error) {
	overrideMu.RLock()
	defer overrideMu.RUnlock()
	if overrideSet {
		return override, nil
	}
	b, err := fs.ReadFile(dataFS, RealDataFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// RealRestaurants decodes the real data file. Entries that cannot be decoded
// or do not validate are skipped and reported in problems, so that one bad
// entry never blocks the server start. A missing or empty file yields nothing.
func RealRestaurants() (list []catalog.RestaurantImport, problems []string) {
	b, err := realDataBytes()
	if err != nil {
		return nil, []string{err.Error()}
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, nil
	}
	var raws []json.RawMessage
	if b[0] == '{' {
		// tolerate { "restaurants": [...] }
		var wrapper struct {
			Restaurants []json.RawMessage `json:"restaurants"`
		}
		if err := json.Unmarshal(b, &wrapper); err != nil {
			return nil, []string{RealDataFile + " : JSON invalide : " + err.Error()}
		}
		raws = wrapper.Restaurants
	} else if err := json.Unmarshal(b, &raws); err != nil {
		return nil, []string{RealDataFile + " : JSON invalide : " + err.Error()}
	}
	seen := map[string]bool{}
	for i, raw := range raws {
		var in catalog.RestaurantImport
		if err := json.Unmarshal(raw, &in); err != nil {
			problems = append(problems, fmt.Sprintf("restaurant n° %d : %v", i+1, err))
			continue
		}
		if p := in.Problems(); len(p) > 0 {
			problems = append(problems, fmt.Sprintf("restaurant n° %d (%s) : %v", i+1, in.Slug, p))
			continue
		}
		if seen[in.Slug] {
			problems = append(problems, fmt.Sprintf("restaurant n° %d : slug %q en double", i+1, in.Slug))
			continue
		}
		seen[in.Slug] = true
		list = append(list, in)
	}
	return list, problems
}

// HasRealData reports whether the real data file holds at least one valid restaurant.
func HasRealData() bool {
	list, _ := RealRestaurants()
	return len(list) > 0
}
