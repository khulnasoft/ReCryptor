// Package secretsharing provides methods to split secrets into shares.
//
// Let n be the number of parties, and t the number of corrupted parties such
// that 0 <= t < n. A (t,n) secret sharing allows to split a secret into n
// shares, such that the secret can be recovered from any subset of at least t+1
// different shares.
//
// A Shamir secret sharing [1] relies on Lagrange polynomial interpolation.
// A Feldman secret sharing [2] extends Shamir's by committing the secret,
// which allows to verify that a share is part of the committed secret.
//
// New returns a SecretSharing compatible with Shamir secret sharing.
// The SecretSharing can be verifiable (compatible with Feldman secret sharing)
// using the CommitSecret and Verify functions.
//
// In this implementation, secret sharing is defined over the scalar field of
// a prime order group.
//
// References
//
//	[1] Shamir, How to share a secret. https://dl.acm.org/doi/10.1145/359168.359176/
//	[2] Feldman, A practical scheme for non-interactive verifiable secret sharing. https://ieeexplore.ieee.org/document/4568297/
package secretsharing

import (
	"fmt"
	"io"

	"github.com/khulnasoft/recryptor/group"
	"github.com/khulnasoft/recryptor/math/polynomial"
)

// Share represents a share of a secret.
type Share struct {
	// ID uniquely identifies a share in a secret sharing instance. ID is never zero.
	ID group.Scalar
	// Value stores the share generated by a secret sharing instance.
	Value group.Scalar
}

// SecretCommitment is the set of commitments generated by splitting a secret.
type SecretCommitment = []group.Element

// SecretSharing provides a (t,n) Shamir's secret sharing. It allows splitting
// a secret into n shares, such that the secret can be only recovered from
// any subset of t+1 shares.
type SecretSharing struct {
	g    group.Group
	t    uint
	poly polynomial.Polynomial
}

// New returns a SecretSharing providing a (t,n) Shamir's secret sharing.
// It allows splitting a secret into n shares, such that the secret is
// only recovered from any subset of at least t+1 shares.
func New(rnd io.Reader, t uint, secret group.Scalar) SecretSharing {
	c := make([]group.Scalar, t+1)
	c[0] = secret.Copy()
	g := secret.Group()
	for i := 1; i < len(c); i++ {
		c[i] = g.RandomScalar(rnd)
	}

	return SecretSharing{g: g, t: t, poly: polynomial.New(c)}
}

// Share creates n shares with an ID monotonically increasing from 1 to n.
func (ss SecretSharing) Share(n uint) []Share {
	shares := make([]Share, n)
	id := ss.g.NewScalar()
	for i := range shares {
		shares[i] = ss.ShareWithID(id.SetUint64(uint64(i + 1)))
	}

	return shares
}

// ShareWithID creates one share of the secret using the ID as identifier.
// Notice that shares with the same ID are considered equal.
// Panics, if the ID is zero.
func (ss SecretSharing) ShareWithID(id group.Scalar) Share {
	if id.IsZero() {
		panic("secretsharing: id cannot be zero")
	}

	return Share{
		ID:    id.Copy(),
		Value: ss.poly.Evaluate(id),
	}
}

// CommitSecret creates a commitment to the secret for further verifying shares.
func (ss SecretSharing) CommitSecret() SecretCommitment {
	c := make(SecretCommitment, ss.poly.Degree()+1)
	for i := range c {
		c[i] = ss.g.NewElement().MulGen(ss.poly.Coefficient(uint(i)))
	}
	return c
}

// Verify returns true if the share s was produced by sharing a secret with
// threshold t and commitment of the secret c.
func Verify(t uint, s Share, c SecretCommitment) bool {
	if len(c) != int(t+1) {
		return false
	}
	if s.ID.IsZero() {
		return false
	}

	g := s.ID.Group()
	lc := len(c) - 1
	sum := g.NewElement().Set(c[lc])
	for i := lc - 1; i >= 0; i-- {
		sum.Mul(sum, s.ID)
		sum.Add(sum, c[i])
	}
	polI := g.NewElement().MulGen(s.Value)
	return polI.IsEqual(sum)
}

// Recover returns a secret provided more than t different shares are given.
// Returns an error if the number of shares is not above the threshold t.
// Panics if some shares are duplicated, i.e., shares must have different IDs.
func Recover(t uint, shares []Share) (secret group.Scalar, err error) {
	if l := len(shares); l <= int(t) {
		return nil, errThreshold(t, uint(l))
	}

	x := make([]group.Scalar, t+1)
	px := make([]group.Scalar, t+1)
	for i := range shares[:t+1] {
		x[i] = shares[i].ID
		px[i] = shares[i].Value
	}

	l := polynomial.NewLagrangePolynomial(x, px)
	zero := shares[0].ID.Group().NewScalar()

	return l.Evaluate(zero), nil
}

func errThreshold(t, n uint) error {
	return fmt.Errorf("secretsharing: number of shares (n=%v) must be above the threshold (t=%v)", n, t)
}
