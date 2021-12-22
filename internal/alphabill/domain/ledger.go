package domain

type (
	LedgerProof struct {
		PreviousStateHash []byte
	}
)

func (l *LedgerProof) Bytes() []byte {
	return l.PreviousStateHash
}
