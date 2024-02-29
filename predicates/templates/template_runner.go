package templates

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
)

const TemplateStartByte = 0x00

type TemplateRunner struct{}

func New() TemplateRunner {
	return TemplateRunner{}
}

func (TemplateRunner) ID() uint64 {
	return TemplateStartByte
}

func (TemplateRunner) Execute(ctx context.Context, p *predicates.Predicate, args []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error) {
	if p.Tag != TemplateStartByte {
		return false, fmt.Errorf("expected predicate template tag %d but got %d", TemplateStartByte, p.Tag)
	}
	if len(p.Code) != 1 {
		return false, fmt.Errorf("expected predicate template code length to be 1, got %d", len(p.Code))
	}

	switch p.Code[0] {
	case P2pkh256ID:
		return p2pkh256_Execute(p.Params, args, txo, env)
	case AlwaysTrueID:
		return alwaysTrue_Execute(p.Params, args)
	case AlwaysFalseID:
		return alwaysFalse_Execute(p.Params, args)
	default:
		return false, fmt.Errorf("unknown predicate template with id %d", p.Code[0])
	}
}
