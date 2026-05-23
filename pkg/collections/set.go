package collections

// Set is a small generic comparable-value set.
type Set[T comparable] map[T]struct{}

func NewSet[T comparable](values ...T) Set[T] {
	set := make(Set[T], len(values))
	for _, value := range values {
		set.Add(value)
	}
	return set
}

func (s Set[T]) Add(value T) {
	s[value] = struct{}{}
}

func (s Set[T]) Has(value T) bool {
	_, ok := s[value]
	return ok
}
