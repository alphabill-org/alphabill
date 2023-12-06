package util

// A simple stack implementation. Not thread safe.
type Stack[T any] []T

func (s *Stack[T]) IsEmpty() bool {
	return len(*s) == 0
}

func (s *Stack[T]) Push(item T) {
	*s = append(*s, item)
}

// Pop pops an item from the stack and panics if the stack is
// empty. IsEmpty method can be used to check if the following Pop
// would be successful.
func (s *Stack[T]) Pop() T {
	index := len(*s)-1
	item := (*s)[index]
	*s = (*s)[:index]
	return item
}
