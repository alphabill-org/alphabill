package templates

import (
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
)

const TemplateStartByte = 0x00

type (
	TemplateRunner struct {
		templates map[byte]PredicateTemplate
	}
)

var runner = newTemplateRunner()

func init() {
	runner.addTemplate(&AlwaysTrue{})
	runner.addTemplate(&AlwaysFalse{})
	runner.addTemplate(&P2pkh256{})

	predicates.RegisterDefaultRunner(runner)
}

func (t *TemplateRunner) Execute(p *predicates.Predicate, ctx *predicates.PredicateContext) error {
	if p == nil {
		return fmt.Errorf("predicate is nil")
	}
	if p.Tag != TemplateStartByte {
		return fmt.Errorf("invalid predicate tag: %d", p.Tag)
	}
	tp, err := t.selectTemplate(p)
	if err != nil {
		return err
	}
	return tp.Execute(p.Params, ctx.Input, ctx.PayloadBytes)
}

func newTemplateRunner() *TemplateRunner {
	return &TemplateRunner{templates: make(map[byte]PredicateTemplate)}
}

func (t *TemplateRunner) addTemplate(template PredicateTemplate) {
	t.templates[template.ID()] = template
}

func (t *TemplateRunner) selectTemplate(p *predicates.Predicate) (PredicateTemplate, error) {
	if len(p.Code) != 1 {
		return nil, fmt.Errorf("expected predicate code length to be 1, got: %d (%X)", len(p.Code), p.Code)
	}
	pt, found := t.templates[p.Code[0]]
	if !found {
		return nil, fmt.Errorf("unknown predicate template: %X", p.Code)
	}
	return pt, nil
}
