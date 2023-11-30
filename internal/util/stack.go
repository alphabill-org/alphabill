package util

type Stack[T any] []T

func (s *Stack[T]) IsEmpty() bool {
	return len(*s) == 0
}

func (s *Stack[T]) Push(item T) {
	*s = append(*s, item)
}

func (s *Stack[T]) Pop() T {
	index := len(*s)-1
	item := (*s)[index]
	*s = (*s)[:index]
	return item
}
