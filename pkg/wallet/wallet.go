package wallet

type Wallet interface {
	GetBalance() uint64
	Send(addr string, amount uint64)
}

func New() Wallet {
	return nil
}
