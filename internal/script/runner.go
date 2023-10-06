package _s

type PredicateRunner interface {
	Execute(sig []byte, sigData []byte) error
}
