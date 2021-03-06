/*
Rating system designed to be used in VoIP Carriers World
Copyright (C) 2012-2015 ITsysCOM

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package engine

import (
	"encoding/json"
	"strings"

	"github.com/cgrates/cgrates/cache2go"
	"github.com/cgrates/cgrates/utils"

	"github.com/cgrates/cgrates/history"
)

/*
Structure that gathers multiple destination prefixes under a common id.
*/
type Destination struct {
	Id       string
	Prefixes []string
}

// returns prefix precision
func (d *Destination) containsPrefix(prefix string) int {
	if d == nil {
		return 0
	}
	for _, p := range d.Prefixes {
		if strings.Index(prefix, p) == 0 {
			return len(p)
		}
	}
	return 0
}

func (d *Destination) String() (result string) {
	result = d.Id + ": "
	for _, k := range d.Prefixes {
		result += k + ", "
	}
	result = strings.TrimRight(result, ", ")
	return result
}

func (d *Destination) AddPrefix(pfx string) {
	d.Prefixes = append(d.Prefixes, pfx)
}

// history record method
func (d *Destination) GetHistoryRecord() history.Record {
	js, _ := json.Marshal(d)
	return history.Record{
		Id:       d.Id,
		Filename: history.DESTINATIONS_FN,
		Payload:  js,
	}
}

// Reverse search in cache to see if prefix belongs to destination id
func CachedDestHasPrefix(destId, prefix string) bool {
	if cached, err := cache2go.Get(utils.DESTINATION_PREFIX + prefix); err == nil {
		_, found := cached.(map[interface{}]struct{})[destId]
		return found
	}
	return false
}

func CleanStalePrefixes(destIds []string) {
	prefixMap, err := cache2go.GetAllEntries(utils.DESTINATION_PREFIX)
	if err != nil {
		return
	}
	for prefix, idIDs := range prefixMap {
		dIDs := idIDs.Value().(map[interface{}]struct{})
		changed := false
		for _, searchedDID := range destIds {
			if _, found := dIDs[searchedDID]; found {
				if len(dIDs) == 1 {
					// remove de prefix from cache
					cache2go.RemKey(utils.DESTINATION_PREFIX + prefix)
				} else {
					// delete the destination from list and put the new list in chache
					delete(dIDs, searchedDID)
					changed = true
				}
			}
		}
		if changed {
			cache2go.Cache(utils.DESTINATION_PREFIX+prefix, dIDs)
		}
	}
}
