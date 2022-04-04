package rootchain

import (
	"encoding/json"

	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var ErrSignerIsNil = errors.New("signer is nil")

func GetVerifierAndPublicKey(signer crypto.Signer) ([]byte, crypto.Verifier, error) {
	if signer == nil {
		return nil, nil, ErrSignerIsNil
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, nil, err
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, nil, err
	}
	return pubKey, verifier, nil
}

func WriteDebugJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() == log.DEBUG {
		j, _ := json.MarshalIndent(arg, "", "\t")
		logger.Debug(m+"\n%s", string(j))
	}
}
