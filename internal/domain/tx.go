package domain

// TODO AB-33
type (

	Tx interface {
		ID() TxID
	}

	TxID string
)
