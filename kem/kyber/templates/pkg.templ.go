// +build ignore
// The previous line (and this one up to the warning below) is removed by the
// template generator.

// Code generated from pkg.templ.go. DO NOT EDIT.

// Package {{.Pkg}} implements the IND-CCA2 secure key encapsulation mechanism
{{ if .NIST -}}
// {{.KemName}} as defined in FIPS203.
{{- else -}}
// {{.KemName}} as submitted to round 3 of the NIST PQC competition and
// described in
//
// https://pq-crystals.org/kyber/data/kyber-specification-round3.pdf
{{- end }}
package {{.Pkg}}

import (
	"bytes"
	"crypto/subtle"
	"io"

	"github.com/khulnasoft/recryptor/internal/sha3"
	"github.com/khulnasoft/recryptor/kem"
	cpapke "github.com/khulnasoft/recryptor/pke/kyber/{{.PkePkg}}"
	cryptoRand "crypto/rand"
)

const (
	// Size of seed for NewKeyFromSeed
	KeySeedSize = cpapke.KeySeedSize + 32

	// Size of seed for EncapsulateTo.
	EncapsulationSeedSize = 32

	// Size of the established shared key.
	SharedKeySize = 32

	// Size of the encapsulated shared key.
	CiphertextSize = cpapke.CiphertextSize

	// Size of a packed public key.
	PublicKeySize = cpapke.PublicKeySize

	// Size of a packed private key.
	PrivateKeySize = cpapke.PrivateKeySize + cpapke.PublicKeySize + 64
)

// Type of a {{.KemName}} public key
type PublicKey struct {
	pk *cpapke.PublicKey

	hpk [32]byte // H(pk)
}

// Type of a {{.KemName}} private key
type PrivateKey struct {
	sk  *cpapke.PrivateKey
	pk  *cpapke.PublicKey
	hpk [32]byte // H(pk)
	z   [32]byte
}

// NewKeyFromSeed derives a public/private keypair deterministically
// from the given seed.
//
// Panics if seed is not of length KeySeedSize.
func NewKeyFromSeed(seed []byte) (*PublicKey, *PrivateKey) {
	var sk PrivateKey
	var pk PublicKey

	if len(seed) != KeySeedSize {
		panic("seed must be of length KeySeedSize")
	}

	{{ if .NIST -}}
	pk.pk, sk.sk = cpapke.NewKeyFromSeedMLKEM(seed[:cpapke.KeySeedSize])
	{{- else -}}
	pk.pk, sk.sk = cpapke.NewKeyFromSeed(seed[:cpapke.KeySeedSize])
	{{- end }}
	sk.pk = pk.pk
	copy(sk.z[:], seed[cpapke.KeySeedSize:])

	// Compute H(pk)
	var ppk [cpapke.PublicKeySize]byte
	sk.pk.Pack(ppk[:])
	h := sha3.New256()
	h.Write(ppk[:])
	h.Read(sk.hpk[:])
	copy(pk.hpk[:], sk.hpk[:])

	return &pk, &sk
}

// GenerateKeyPair generates public and private keys using entropy from rand.
// If rand is nil, crypto/rand.Reader will be used.
func GenerateKeyPair(rand io.Reader) (*PublicKey, *PrivateKey, error) {
	var seed [KeySeedSize]byte
	if rand == nil {
		rand = cryptoRand.Reader
	}
	_, err := io.ReadFull(rand, seed[:])
	if err != nil {
		return nil, nil, err
	}
	pk, sk := NewKeyFromSeed(seed[:])
	return pk, sk, nil
}

// EncapsulateTo generates a shared key and ciphertext that contains it
// for the public key using randomness from seed and writes the shared key
// to ss and ciphertext to ct.
//
// Panics if ss, ct or seed are not of length SharedKeySize, CiphertextSize
// and EncapsulationSeedSize respectively.
//
// seed may be nil, in which case crypto/rand.Reader is used to generate one.
func (pk *PublicKey) EncapsulateTo(ct, ss []byte, seed []byte) {
	if seed == nil {
		seed = make([]byte, EncapsulationSeedSize)
		if _, err := cryptoRand.Read(seed[:]); err != nil {
			panic(err)
		}
	} else {
		if len(seed) != EncapsulationSeedSize {
			panic("seed must be of length EncapsulationSeedSize")
		}
	}

	if len(ct) != CiphertextSize {
		panic("ct must be of length CiphertextSize")
	}

	if len(ss) != SharedKeySize {
		panic("ss must be of length SharedKeySize")
	}

	var m [32]byte
	{{ if .NIST -}}
	copy(m[:], seed)
	{{- else -}}
	// m = H(seed), the hash of shame
	h := sha3.New256()
	h.Write(seed)
	h.Read(m[:])
	{{- end }}

	// (K', r) = G(m ‖ H(pk))
	var kr [64]byte
	g := sha3.New512()
	g.Write(m[:])
	g.Write(pk.hpk[:])
	g.Read(kr[:])

	// c = Kyber.CPAPKE.Enc(pk, m, r)
	pk.pk.EncryptTo(ct, m[:], kr[32:])

	{{ if .NIST -}}
	copy(ss, kr[:SharedKeySize])
	{{- else -}}
	// Compute H(c) and put in second slot of kr, which will be (K', H(c)).
	h.Reset()
	h.Write(ct[:CiphertextSize])
	h.Read(kr[32:])

	// K = KDF(K' ‖ H(c))
	kdf := sha3.NewShake256()
	kdf.Write(kr[:])
	kdf.Read(ss[:SharedKeySize])
	{{- end }}
}

// DecapsulateTo computes the shared key which is encapsulated in ct
// for the private key.
//
// Panics if ct or ss are not of length CiphertextSize and SharedKeySize
// respectively.
func (sk *PrivateKey) DecapsulateTo(ss, ct []byte) {
	if len(ct) != CiphertextSize {
		panic("ct must be of length CiphertextSize")
	}

	if len(ss) != SharedKeySize {
		panic("ss must be of length SharedKeySize")
	}

	// m' = Kyber.CPAPKE.Dec(sk, ct)
	var m2 [32]byte
	sk.sk.DecryptTo(m2[:], ct)

	// (K'', r') = G(m' ‖ H(pk))
	var kr2 [64]byte
	g := sha3.New512()
	g.Write(m2[:])
	g.Write(sk.hpk[:])
	g.Read(kr2[:])

	// c' = Kyber.CPAPKE.Enc(pk, m', r')
	var ct2 [CiphertextSize]byte
	sk.pk.EncryptTo(ct2[:], m2[:], kr2[32:])

	{{ if .NIST -}}
	var ss2 [SharedKeySize]byte

	// Compute shared secret in case of rejection: ss₂ = PRF(z ‖ c)
	prf := sha3.NewShake256()
	prf.Write(sk.z[:])
	prf.Write(ct[:CiphertextSize])
	prf.Read(ss2[:])

	// Set ss2 to the real shared secret if c = c'.
	subtle.ConstantTimeCopy(
		subtle.ConstantTimeCompare(ct, ct2[:]),
		ss2[:],
		kr2[:SharedKeySize],
	)

	copy(ss, ss2[:])
	{{- else -}}
	// Compute H(c) and put in second slot of kr2, which will be (K'', H(c)).
	h := sha3.New256()
	h.Write(ct[:CiphertextSize])
	h.Read(kr2[32:])

	// Replace K'' by  z in the first slot of kr2 if c ≠ c'.
	subtle.ConstantTimeCopy(
		1-subtle.ConstantTimeCompare(ct, ct2[:]),
		kr2[:32],
		sk.z[:],
	)

	// K = KDF(K''/z, H(c))
	kdf := sha3.NewShake256()
	kdf.Write(kr2[:])
	kdf.Read(ss)
	{{- end }}
}

// Packs sk to buf.
//
// Panics if buf is not of size PrivateKeySize.
func (sk *PrivateKey) Pack(buf []byte) {
	if len(buf) != PrivateKeySize {
		panic("buf must be of length PrivateKeySize")
	}

	sk.sk.Pack(buf[:cpapke.PrivateKeySize])
	buf = buf[cpapke.PrivateKeySize:]
	sk.pk.Pack(buf[:cpapke.PublicKeySize])
	buf = buf[cpapke.PublicKeySize:]
	copy(buf, sk.hpk[:])
	buf = buf[32:]
	copy(buf, sk.z[:])
}

// Unpacks sk from buf.
//
// Panics if buf is not of size PrivateKeySize.
{{ if .NIST -}}
//
// Returns an error if buf is not of size PrivateKeySize, or private key
// doesn't pass the ML-KEM decapsulation key check.
func (sk *PrivateKey) Unpack(buf []byte) error {
	if len(buf) != PrivateKeySize {
		return kem.ErrPrivKeySize
	}
{{- else -}}
func (sk *PrivateKey) Unpack(buf []byte) {
	if len(buf) != PrivateKeySize {
		panic("buf must be of length PrivateKeySize")
	}
{{- end }}

	sk.sk = new(cpapke.PrivateKey)
	sk.sk.Unpack(buf[:cpapke.PrivateKeySize])
	buf = buf[cpapke.PrivateKeySize:]
	sk.pk = new(cpapke.PublicKey)
	sk.pk.Unpack(buf[:cpapke.PublicKeySize])
{{ if .NIST -}}
	var hpk [32]byte
	h := sha3.New256()
	h.Write(buf[:cpapke.PublicKeySize])
	h.Read(hpk[:])
{{ end -}}
	buf = buf[cpapke.PublicKeySize:]
	copy(sk.hpk[:], buf[:32])
	copy(sk.z[:], buf[32:])
{{ if .NIST -}}
	if !bytes.Equal(hpk[:], sk.hpk[:]) {
		return kem.ErrPrivKey
	}
	return nil
{{ end -}}
}

// Packs pk to buf.
//
// Panics if buf is not of size PublicKeySize.
func (pk *PublicKey) Pack(buf []byte) {
	if len(buf) != PublicKeySize {
		panic("buf must be of length PublicKeySize")
	}

	pk.pk.Pack(buf)
}

// Unpacks pk from buf.
//
{{ if .NIST -}}
// Returns an error if buf is not of size PublicKeySize, or the public key
// is not normalized.
func (pk *PublicKey) Unpack(buf []byte) error {
	if len(buf) != PublicKeySize {
		return kem.ErrPubKeySize
	}
{{- else -}}
// Panics if buf is not of size PublicKeySize.
func (pk *PublicKey) Unpack(buf []byte) {
	if len(buf) != PublicKeySize {
		panic("buf must be of length PublicKeySize")
	}
{{- end }}

	pk.pk = new(cpapke.PublicKey)
	{{ if .NIST -}}
	if err := pk.pk.UnpackMLKEM(buf); err != nil {
		return err
	}
	{{- else -}}
	pk.pk.Unpack(buf)
	{{- end }}

	// Compute cached H(pk)
	h := sha3.New256()
	h.Write(buf)
	h.Read(pk.hpk[:])

	{{ if .NIST -}}
	return nil
	{{- end }}
}

// Boilerplate down below for the KEM scheme API.

type scheme struct{}

var sch kem.Scheme = &scheme{}

// Scheme returns a KEM interface.
func Scheme() kem.Scheme { return sch }

func (*scheme) Name() string               { return "{{.Name}}" }
func (*scheme) PublicKeySize() int         { return PublicKeySize }
func (*scheme) PrivateKeySize() int        { return PrivateKeySize }
func (*scheme) SeedSize() int              { return KeySeedSize }
func (*scheme) SharedKeySize() int         { return SharedKeySize }
func (*scheme) CiphertextSize() int        { return CiphertextSize }
func (*scheme) EncapsulationSeedSize() int { return EncapsulationSeedSize }

func (sk *PrivateKey) Scheme() kem.Scheme { return sch }
func (pk *PublicKey) Scheme() kem.Scheme  { return sch }

func (sk *PrivateKey) MarshalBinary() ([]byte, error) {
	var ret [PrivateKeySize]byte
	sk.Pack(ret[:])
	return ret[:], nil
}

func (sk *PrivateKey) Equal(other kem.PrivateKey) bool {
	oth, ok := other.(*PrivateKey)
	if !ok {
		return false
	}
	if sk.pk == nil && oth.pk == nil {
		return true
	}
	if sk.pk == nil || oth.pk == nil {
		return false
	}
	if !bytes.Equal(sk.hpk[:], oth.hpk[:]) ||
		subtle.ConstantTimeCompare(sk.z[:], oth.z[:]) != 1 {
		return false
	}
	return sk.sk.Equal(oth.sk)
}

func (pk *PublicKey) Equal(other kem.PublicKey) bool {
	oth, ok := other.(*PublicKey)
	if !ok {
		return false
	}
	if pk.pk == nil && oth.pk == nil {
		return true
	}
	if pk.pk == nil || oth.pk == nil {
		return false
	}
	return bytes.Equal(pk.hpk[:], oth.hpk[:])
}

func (sk *PrivateKey) Public() kem.PublicKey {
	pk := new(PublicKey)
	pk.pk = sk.pk
	copy(pk.hpk[:], sk.hpk[:])
	return pk
}

func (pk *PublicKey) MarshalBinary() ([]byte, error) {
	var ret [PublicKeySize]byte
	pk.Pack(ret[:])
	return ret[:], nil
}

func (*scheme) GenerateKeyPair() (kem.PublicKey, kem.PrivateKey, error) {
	return GenerateKeyPair(cryptoRand.Reader)
}

func (*scheme) DeriveKeyPair(seed []byte) (kem.PublicKey, kem.PrivateKey) {
	if len(seed) != KeySeedSize {
		panic(kem.ErrSeedSize)
	}
	return NewKeyFromSeed(seed[:])
}

func (*scheme) Encapsulate(pk kem.PublicKey) (ct, ss []byte, err error) {
	ct = make([]byte, CiphertextSize)
	ss = make([]byte, SharedKeySize)

	pub, ok := pk.(*PublicKey)
	if !ok {
		return nil, nil, kem.ErrTypeMismatch
	}
	pub.EncapsulateTo(ct, ss, nil)
	return
}

func (*scheme) EncapsulateDeterministically(pk kem.PublicKey, seed []byte) (
	ct, ss []byte, err error) {
	if len(seed) != EncapsulationSeedSize {
		return nil, nil, kem.ErrSeedSize
	}

	ct = make([]byte, CiphertextSize)
	ss = make([]byte, SharedKeySize)

	pub, ok := pk.(*PublicKey)
	if !ok {
		return nil, nil, kem.ErrTypeMismatch
	}
	pub.EncapsulateTo(ct, ss, seed)
	return
}

func (*scheme) Decapsulate(sk kem.PrivateKey, ct []byte) ([]byte, error) {
	if len(ct) != CiphertextSize {
		return nil, kem.ErrCiphertextSize
	}

	priv, ok := sk.(*PrivateKey)
	if !ok {
		return nil, kem.ErrTypeMismatch
	}
	ss := make([]byte, SharedKeySize)
	priv.DecapsulateTo(ss, ct)
	return ss, nil
}

func (*scheme) UnmarshalBinaryPublicKey(buf []byte) (kem.PublicKey, error) {
	var ret PublicKey
	{{ if .NIST -}}
	if err := ret.Unpack(buf); err != nil {
		return nil, err
	}
	{{- else -}}
	if len(buf) != PublicKeySize {
		return nil, kem.ErrPubKeySize
	}
	ret.Unpack(buf)
	{{- end }}
	return &ret, nil
}

func (*scheme) UnmarshalBinaryPrivateKey(buf []byte) (kem.PrivateKey, error) {
	if len(buf) != PrivateKeySize {
		return nil, kem.ErrPrivKeySize
	}
	var ret PrivateKey
	{{ if .NIST -}}
	if err := ret.Unpack(buf); err != nil {
		return nil, err
	}
	{{- else -}}
	ret.Unpack(buf)
	{{- end }}
	return &ret, nil
}