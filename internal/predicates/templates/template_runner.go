package templates

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/predicates"
)

const TemplateStartByte = 0x00

func init() {
	fmt.Println("Templates init") // TODO: remove
	runner := newTemplateRunner()
	runner.addTemplate(&AlwaysTrue{})
	runner.addTemplate(&AlwaysFalse{})
	runner.addTemplate(&P2pkh256{})

	predicates.RegisterDefaultRunner(runner)
}

type (
	TemplateRunner struct {
		templates map[uint64]PredicateTemplate
	}
)

func (t *TemplateRunner) Execute(p *predicates.Predicate, sig []byte, sigData []byte) error {
	if p.Tag != TemplateStartByte {
		return fmt.Errorf("invalid predicate tag: %d", p.Tag)
	}
	tp, err := t.selectTemplate(p)
	if err != nil {
		return err
	}
	return tp.Execute(p.Body, sig, sigData)
}

func newTemplateRunner() *TemplateRunner {
	return &TemplateRunner{templates: make(map[uint64]PredicateTemplate)}
}

func (t *TemplateRunner) addTemplate(template PredicateTemplate) {
	t.templates[template.ID()] = template
}

func (t *TemplateRunner) selectTemplate(p *predicates.Predicate) (PredicateTemplate, error) {
	pt, found := t.templates[p.ID]
	if !found {
		return nil, fmt.Errorf("unknown predicate template: %d", p.ID)
	}
	return pt, nil
}
