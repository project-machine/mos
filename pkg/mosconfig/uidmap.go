package mosconfig

import (
	"fmt"
	"os"

	"github.com/lxc/lxd/shared/idmap"
	"github.com/project-machine/mos/pkg/utils"
)

type uidRangeDefaults struct {
	SubidStart int64
	SubidRange int64
}

var fullRangeDefaults = uidRangeDefaults{
	SubidStart: 100000,
	SubidRange: 65536,
}

// for testing in containers which start with 65536 uids,
// let's just start our container services at 40k and give
// each 2k uids.
var tinyRangeDefaults = uidRangeDefaults{
	SubidStart: 40000,
	SubidRange: 2000,
}

// return true if we are running with the full host uid mapping
func UidmapIsHost() bool {
	bytes, err := os.ReadFile("/proc/self/uid_map")
	if err != nil {
		return false
	}

	return uidmapIsHost(string(bytes))
}

// We could be smarter here, but this is only to accomodate edge
// cases right now, so let's keep it simple:  If /proc/self/uid_map
// is 0 0 4294967295, we use the default range, else we use the tiny
// range.
func chooseRangeDefaults() uidRangeDefaults {
	if UidmapIsHost() {
		return fullRangeDefaults
	}
	return tinyRangeDefaults
}

func firstUnusedUID(uidmaps []IdmapSet) int64 {
	rangedefs := chooseRangeDefaults()
	min := rangedefs.SubidStart
	for _, u := range uidmaps {
		if u.Hostid > min {
			min = u.Hostid + rangedefs.SubidRange
		}
	}
	return min
}

func addUIDMap(old []IdmapSet, uidmaps []IdmapSet, t Target) []IdmapSet {
	if !t.NeedsIdmap() {
		return uidmaps
	}
	for _, u := range uidmaps {
		if u.Name == t.NSGroup {
			// use the nsgroup already defined in system manifest
			return uidmaps
		}
	}

	for _, u := range old {
		if u.Name == t.NSGroup {
			// use the nsgroup already defined in system manifest
			return append(uidmaps, u)
		}
	}

	// Create a new idmap range
	uidmap := IdmapSet{
		Name:   t.NSGroup,
		Hostid: firstUnusedUID(uidmaps),
	}
	uidmaps = append(uidmaps, uidmap)
	return uidmaps
}

// The install/upgrade step should have created an idmap
// already so we return an error if simply not found
func (mos *Mos) GetUIDMapStr(t *Target) (idmap.IdmapSet, []string, error) {
	empty := idmap.IdmapSet{
		Idmap: []idmap.IdmapEntry{},
	}
	manifest, err := mos.CurrentManifest()
	if err != nil {
		return empty, []string{}, fmt.Errorf("Error opening manifest: %w", err)
	}
	rangedefs := chooseRangeDefaults()

	for _, u := range manifest.UidMaps {
		if u.Name == t.NSGroup {
			uidmap := idmap.IdmapEntry{
				Isuid:    true,
				Isgid:    true,
				Hostid:   u.Hostid,
				Nsid:     0,
				Maprange: rangedefs.SubidRange,
			}
			set := idmap.IdmapSet{
				Idmap: []idmap.IdmapEntry{uidmap},
			}
			return set, uidmap.ToLxcString(), nil
		}
	}

	return empty, []string{}, fmt.Errorf("Error finding UID Mapping for %s", t.ServiceName)
}

func addUidMapping(set idmap.IdmapSet) error {
	for _, u := range set.Idmap {
		// In atomix we wrote /etc/subuid by hand.  Here we are
		// calling out to usermod.  Not sure which is the "better"
		// way.
		first := u.Hostid
		last := first + u.Maprange - 1
		r := fmt.Sprintf("%d-%d", first, last)

		cmdStr := []string{"usermod", "-v", r, "root"}
		if err := utils.RunCommand(cmdStr...); err != nil {
			return fmt.Errorf("Error adding subuid allocation: %w", err)
		}
		cmdStr = []string{"usermod", "-w", r, "root"}
		if err := utils.RunCommand(cmdStr...); err != nil {
			return fmt.Errorf("Error adding subgid allocation: %w", err)
		}
	}

	return nil
}
