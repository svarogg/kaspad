package flowcontext

import "github.com/kaspanet/go-secp256k1"

func (f *FlowContext) SelfPublicKey() *secp256k1.SchnorrPublicKey {
	return f.selfPublicKey
}

func (f *FlowContext) SelfPrivateKey() *secp256k1.PrivateKey {
	return f.selfPrivateKey
}
