package _s

import (
	"errors"
	"fmt"
)

type stack struct {
	data [][]byte
}

func (s *stack) push(data []byte) {
	s.data = append(s.data, data)
}

func (s *stack) pop() ([]byte, error) {
	if s.size() == 0 {
		return nil, errors.New("cannot pop from empty stack")
	}
	lastIdx := s.size() - 1
	top := s.data[lastIdx]
	s.data = s.data[:lastIdx]
	return top, nil
}

func (s *stack) peek() ([]byte, error) {
	if s.isEmpty() {
		return nil, errors.New("cannot peek into empty stack")
	}
	return s.data[s.size()-1], nil
}

func (s *stack) size() int {
	return len(s.data)
}

func (s *stack) isEmpty() bool {
	return len(s.data) == 0
}

func (s *stack) pushBool(val bool) {
	if val {
		s.data = append(s.data, []byte{0x01})
	} else {
		s.data = append(s.data, []byte{0x00})
	}
}

func (s *stack) popBool() (bool, error) {
	top, err := s.pop()
	if err != nil {
		return false, err
	}
	if top[0] == 0x00 {
		return false, nil
	}
	if top[0] == 0x01 {
		return true, nil
	}
	return false, fmt.Errorf("popped value is not a boolean [0x%x]", top[0])
}
