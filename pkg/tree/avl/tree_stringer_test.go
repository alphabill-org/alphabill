package avl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintEmptyTree(t *testing.T) {
	require.Equal(t, "────┤ empty", New[IntKey, *Int64Value]().String())
}

func TestPrintTree(t *testing.T) {
	tree := newIntTree()

	for _, i := range []int{15, 8, 18, 2, 12, 17, 19, 7, 10, 13, 20, 9} {
		require.NoError(t, tree.Add(IntKey(i), &Int64Value{value: int64(i)}))
	}
	tree.Commit()
	//	 			┌───┤ key=20, depth=1, value=20, total=20, clean=true
	//			┌───┤ key=19, depth=2, value=19, total=39, clean=true
	//		┌───┤ key=18, depth=3, value=18, total=74, clean=true
	//		│	└───┤ key=17, depth=1, value=17, total=17, clean=true
	//	────┤ key=15, depth=5, value=15, total=150, clean=true
	//		│		┌───┤ key=13, depth=1, value=13, total=13, clean=true
	//		│	┌───┤ key=12, depth=3, value=12, total=44, clean=true
	//		│	│	└───┤ key=10, depth=2, value=10, total=19, clean=true
	//		│	│		└───┤ key=9, depth=1, value=9, total=9, clean=true
	//		└───┤ key=8, depth=4, value=8, total=61, clean=true
	//			│	┌───┤ key=7, depth=1, value=7, total=7, clean=true
	//			└───┤ key=2, depth=2, value=2, total=9, clean=true
	expected := "\t\t\t┌───┤ key=20, depth=1, value=20, total=20, clean=true\n\t\t┌───┤ key=19, depth=2, value=19, total=39, clean=true\n\t┌───┤ key=18, depth=3, value=18, total=74, clean=true\n\t│\t└───┤ key=17, depth=1, value=17, total=17, clean=true\n────┤ key=15, depth=5, value=15, total=150, clean=true\n\t│\t\t┌───┤ key=13, depth=1, value=13, total=13, clean=true\n\t│\t┌───┤ key=12, depth=3, value=12, total=44, clean=true\n\t│\t│\t└───┤ key=10, depth=2, value=10, total=19, clean=true\n\t│\t│\t\t└───┤ key=9, depth=1, value=9, total=9, clean=true\n\t└───┤ key=8, depth=4, value=8, total=61, clean=true\n\t\t│\t┌───┤ key=7, depth=1, value=7, total=7, clean=true\n\t\t└───┤ key=2, depth=2, value=2, total=9, clean=true\n"
	require.Equal(t, expected, tree.String())
}
