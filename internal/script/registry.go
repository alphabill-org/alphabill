package _s

import (
	"errors"
)

type PredicateRunnerFactory interface {
	IsApplicable([]byte) bool
	Create([]byte) (PredicateRunner, error)
}

type Registry struct {
	factories []PredicateRunnerFactory
}

func NewRegistry() *Registry {
	return &Registry{}
}

var defaultRegistry = NewRegistry()

func (r *Registry) Register(f PredicateRunnerFactory) {
	r.factories = append(r.factories, f)
}

func (r *Registry) FindRunner(predicate []byte) (PredicateRunner, error) {
	for _, f := range r.factories {
		if f.IsApplicable(predicate) {
			return f.Create(predicate)
		}
	}
	return nil, errors.New("no predicate runner found")
}

func Register(f PredicateRunnerFactory) bool {
	defaultRegistry.Register(f)
	return true
}

func FindRunner(predicate []byte) (PredicateRunner, error) {
	return defaultRegistry.FindRunner(predicate)
}
