package handshake

import (
	"github.com/kaspanet/go-secp256k1"
	"github.com/kaspanet/kaspad/util/daghash"
)

var handshakeMessageToSignHash *secp256k1.Hash

func init() {
	var handshakeMessageToSign = []byte("handshake message to sign")
	hash := secp256k1.Hash(daghash.HashH(handshakeMessageToSign))
	handshakeMessageToSignHash = &hash
}
