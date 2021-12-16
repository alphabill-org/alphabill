package domain

type (
	LedgerProof struct {
		PreviousStateHash []byte
	}
)

func (l *LedgerProof) Normalise() []byte {
	return l.PreviousStateHash
}
