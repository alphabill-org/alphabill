package event

const (
	Error Type = iota
	TransactionProcessed
	TransactionFailed
	NewRoundStarted
	UnicityCertificateHandled
	BlockFinalized
	RecoveryStarted
	RecoveryFinished
	StateReverted
)

type (
	Event struct {
		EventType Type
		Content   any
	}

	Type int

	Handler func(e *Event)
)
