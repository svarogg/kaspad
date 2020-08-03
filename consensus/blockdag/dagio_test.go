// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/kaspanet/kaspad/util/daghash"
)

// TestDAGStateSerialization ensures serializing and deserializing the
// DAG state works as expected.
func TestDAGStateSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		state      *dagState
		serialized []byte
	}{
		{
			name: "genesis",
			state: &dagState{
				TipHashes:         []*daghash.Hash{newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")},
				LastFinalityPoint: newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
			},
			serialized: []byte("{\"TipHashes\":[[111,226,140,10,182,241,179,114,193,166,162,70,174,99,247,79,147,30,131,101,225,90,8,156,104,214,25,0,0,0,0,0]],\"LastFinalityPoint\":[111,226,140,10,182,241,179,114,193,166,162,70,174,99,247,79,147,30,131,101,225,90,8,156,104,214,25,0,0,0,0,0],\"LocalSubnetworkID\":null}"),
		},
		{
			name: "block 1",
			state: &dagState{
				TipHashes:         []*daghash.Hash{newHashFromStr("00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048")},
				LastFinalityPoint: newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
			},
			serialized: []byte("{\"TipHashes\":[[72,96,235,24,191,27,22,32,227,126,148,144,252,138,66,117,20,65,111,215,81,89,171,134,104,142,154,131,0,0,0,0]],\"LastFinalityPoint\":[111,226,140,10,182,241,179,114,193,166,162,70,174,99,247,79,147,30,131,101,225,90,8,156,104,214,25,0,0,0,0,0],\"LocalSubnetworkID\":null}"),
		},
	}

	for i, test := range tests {
		gotBytes, err := serializeDAGState(test.state)
		if err != nil {
			t.Errorf("serializeDAGState #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}

		// Ensure the dagState serializes to the expected value.
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeDAGState #%d (%s): mismatched "+
				"bytes - got %s, want %s", i, test.name,
				string(gotBytes), string(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// dagState.
		state, err := deserializeDAGState(test.serialized)
		if err != nil {
			t.Errorf("deserializeDAGState #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(state, test.state) {
			t.Errorf("deserializeDAGState #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, state, test.state)
			continue
		}
	}
}

// newHashFromStr converts the passed big-endian hex string into a
// daghash.Hash. It only differs from the one available in daghash in that
// it panics in case of an error since it will only (and must only) be
// called with hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *daghash.Hash {
	hash, err := daghash.NewHashFromStr(hexStr)
	if err != nil {
		panic(err)
	}
	return hash
}
