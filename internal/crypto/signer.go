/*
 * GUARDTIME CONFIDENTIAL
 *
 * Copyright 2008-2020 Guardtime, Inc.
 * All Rights Reserved.
 *
 * All information contained herein is, and remains, the property
 * of Guardtime, Inc. and its suppliers, if any.
 * The intellectual and technical concepts contained herein are
 * proprietary to Guardtime, Inc. and its suppliers and may be
 * covered by U.S. and foreign patents and patents in process,
 * and/or are protected by trade secret or copyright law.
 * Dissemination of this information or reproduction of this material
 * is strictly forbidden unless prior written permission is obtained
 * from Guardtime, Inc.
 * "Guardtime" and "KSI" are trademarks or registered trademarks of
 * Guardtime, Inc., and no license to trademarks is granted; Guardtime
 * reserves and retains all trademark rights.
 */

package crypto

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"io"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain/canonicalizer"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

type (
	// InMemoryEd25519Signer for using during development
	InMemoryEd25519Signer struct {
		key  ed25519.PrivateKey
		rand io.Reader
	}
)

// NewInMemoryEd25519Signer generates new key and creates a new InMemoryEd25519Signer.
func NewInMemoryEd25519Signer() (*InMemoryEd25519Signer, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return NewInMemoryEd25519SignerFromSeed(privateKey.Seed()), nil
}

// NewInMemoryEd25519SignerFromSeed creates new InMemoryEd25519Signer from private key seed bytes.
func NewInMemoryEd25519SignerFromSeed(seed []byte) *InMemoryEd25519Signer {
	privKey := ed25519.NewKeyFromSeed(seed) // will panic if key is incorrect
	return &InMemoryEd25519Signer{
		key:  privKey,
		rand: rand.Reader,
	}
}

func (s *InMemoryEd25519Signer) SignBytes(data []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "nil argument")
	}
	sig, err := s.key.Sign(s.rand, data, crypto.Hash(0))
	if err != nil {
		return nil, err
	}
	return sig, nil
}

func (s *InMemoryEd25519Signer) SignObject(obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) ([]byte, error) {
	if s == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "nil argument")
	}
	data, err := canonicalizer.Canonicalize(obj, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to canonicalize the object")
	}
	return s.SignBytes(data)
}

func (s *InMemoryEd25519Signer) Verifier() Verifier {
	pk := s.key.Public().(ed25519.PublicKey)
	return NewEd25519Verifier(pk)
}

func (s *InMemoryEd25519Signer) MarshalPrivateKey() ([]byte, error) {
	if s == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "nil argument")
	}
	return s.key.Seed(), nil
}
