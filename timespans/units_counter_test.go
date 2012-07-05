/*
Rating system designed to be used in VoIP Carriers World
Copyright (C) 2012  Radu Ioan Fericean

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

package timespans

import (
	"reflect"
	"testing"
)

func TestUnitsCounterStoreRestore(t *testing.T) {
	uc := &UnitsCounter{
		Direction:     OUTBOUND,
		BalanceId:     SMS,
		Units:         100,
		Weight:        10,
		MinuteBuckets: []*MinuteBucket{&MinuteBucket{Weight: 20, Price: 1, DestinationId: "NAT"}, &MinuteBucket{Weight: 10, Price: 10, Percent: 0, DestinationId: "RET"}},
	}
	r := uc.store()
	if string(r) != "OUT/SMS/100/10/0;20;1;0;NAT,0;10;10;0;RET" {
		t.Errorf("Error serializing units counter: %v", string(r))
	}
	o := &UnitsCounter{}
	o.restore(r)
	if !reflect.DeepEqual(o, uc) {
		t.Errorf("Expected %v was  %v", uc, o)
	}
}