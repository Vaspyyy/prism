// Types with braces in them, and declarations without a block.
package types

type extractor func(s *state, i int) int

var extractors = []extractor{
	plain,
}

type Usage int

const (
	Any Usage = iota
	Server
)

func Value() interface{} {
	return nil
}

func (s *Set) Keys() map[string]struct{} {
	return s.keys
}

func Min[T interface{ ~int | ~string }](x, y T) T {
	if x < y {
		return x
	}
	return y
}

func Label[T interface {
	~int
	String() string
}](x T) string {
	return x.String()
}

type Pair[K interface{ comparable }, V any] struct {
	key K
	val V
}

func capacity(c any) int

var barrier struct {
	enabled bool
}
