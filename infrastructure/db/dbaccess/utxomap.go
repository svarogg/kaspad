package dbaccess

import (
	"github.com/kaspanet/kaspad/infrastructure/db/database"
)

var (
	utxoMapBucket = database.MakeBucket([]byte("utxo-map"))
)

func utxoMapKey(addressKey []byte) *database.Key {
	return utxoBucket.Key(addressKey)
}

// AddToUTXOMap adds the given address-utxoSet pair to
// the database's UTXOMap.
func AddToUTXOMap(context Context, addressKey []byte, utxoSet []byte) error {
	accessor, err := context.accessor()
	if err != nil {
		return err
	}

	key := utxoMapKey(addressKey)
	return accessor.Put(key, utxoSet)
}

// RemoveFromUTXOMap removes the given address from the
// database's UTXOMap.
func RemoveFromUTXOMap(context Context, addressKey []byte) error {
	accessor, err := context.accessor()
	if err != nil {
		return err
	}

	key := utxoMapKey(addressKey)
	return accessor.Delete(key)
}

// GetFromUTXOMap return the given address from the
// database's UTXOMap.
func GetFromUTXOMap(context Context, addressKey []byte) ([]byte, error) {
	accessor, err := context.accessor()
	if err != nil {
		return nil, err
	}

	key := utxoMapKey(addressKey)
	return accessor.Get(key)
}

// UTXOMapCursor opens a cursor over all the UTXOMap entries
// that have been previously added to the database.
func UTXOMapCursor(context Context) (database.Cursor, error) {
	accessor, err := context.accessor()
	if err != nil {
		return nil, err
	}

	return accessor.Cursor(utxoBucket)
}
