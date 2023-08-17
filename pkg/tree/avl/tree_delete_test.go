package avl

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type value struct {
	key   IntKey
	value int64
}

type order struct {
	value
	clean bool
	depth int64
}

func TestDelete_NotFound(t *testing.T) {
	tree := initTreeAndAddValues(t)
	require.ErrorContains(t, tree.Delete(100), "key 100 does not exist")
	require.ErrorContains(t, tree.Delete(-1), "key -1 does not exist")
}

func TestDelete_TreeIsEmpty(t *testing.T) {
	require.ErrorContains(t, New[IntKey, *Int64Value]().Delete(100), "key 100 does not exist")
}

func TestDelete_Ok(t *testing.T) {
	tests := []struct {
		name          string
		valuesToAdd   []value
		removeNodeKey IntKey
		expectedTotal int64
		expectedOrder []order // NB! in post-order
	}{
		{
			name: "add two nodes; remove right child",
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 100, value: 100},
			},
			removeNodeKey: 100,
			expectedTotal: 50,
			expectedOrder: []order{
				{value: value{key: 50, value: 50}, clean: false, depth: 1},
			},
		},
		{
			name: "add two nodes; remove left child",
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 25, value: 25},
			},
			removeNodeKey: 25,
			expectedTotal: 50,
			expectedOrder: []order{
				{value: value{key: 50, value: 50}, clean: false, depth: 1},
			},
		},
		{
			name: "add two nodes; remove root",
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 100, value: 100},
			},
			removeNodeKey: 50,
			expectedTotal: 100,
			expectedOrder: []order{
				{value: value{key: 100, value: 100}, clean: true, depth: 1},
			},
		},
		{
			name: "add 3 nodes; remove root",
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 25, value: 25},
				{key: 100, value: 100},
			},
			removeNodeKey: 50,
			expectedTotal: 125,
			expectedOrder: []order{
				{value: value{key: 100, value: 100}, clean: true, depth: 1},
				{value: value{key: 25, value: 25}, clean: false, depth: 2},
			},
		},
		{
			name: "add 3 nodes; remove left child",
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 25, value: 25},
				{key: 100, value: 100},
			},
			removeNodeKey: 25,
			expectedTotal: 150,
			expectedOrder: []order{
				{value: value{key: 100, value: 100}, clean: true, depth: 1},
				{value: value{key: 50, value: 50}, clean: false, depth: 2},
			},
		},
		{
			name: "add 3 nodes; remove right child",
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 25, value: 25},
				{key: 100, value: 100},
			},
			removeNodeKey: 100,
			expectedTotal: 75,
			expectedOrder: []order{
				{value: value{key: 25, value: 25}, clean: true, depth: 1},
				{value: value{key: 50, value: 50}, clean: false, depth: 2},
			},
		},
		{
			name: "add 4 nodes; removing a root causes left rotate",
			valuesToAdd: []value{
				//		┌───┤ key=7, depth=2, value=7, total=12, clean=true
				//		│	└───┤ key=5, depth=1, value=5, total=5, clean=true
				//	────┤ key=3, depth=3, value=3, total=16, clean=true
				//		└───┤ key=1, depth=1, value=1, total=1, clean=true
				{3, 3},
				{1, 1},
				{7, 7},
				{5, 5},
			},
			removeNodeKey: 3,
			expectedTotal: 13,
			expectedOrder: []order{
				{value: value{key: 1, value: 1}, clean: false, depth: 1},
				{value: value{key: 7, value: 7}, clean: false, depth: 1},
				{value: value{key: 5, value: 5}, clean: false, depth: 2},
			},
		},
		{
			name: "add 7 nodes; remove root",
			valuesToAdd: []value{
				//	 		┌───┤ key=9, depth=1, value=9, total=9, clean=true
				//		┌───┤ key=8, depth=2, value=8, total=24, clean=true
				//		│	└───┤ key=7, depth=1, value=7, total=7, clean=true
				//	────┤ key=6, depth=3, value=6, total=41, clean=true
				//		│	┌───┤ key=5, depth=1, value=5, total=5, clean=true
				//		└───┤ key=4, depth=2, value=4, total=11, clean=true
				//			└───┤ key=2, depth=1, value=2, total=2, clean=true
				{key: 6, value: 6},
				{key: 4, value: 4},
				{key: 8, value: 8},
				{key: 2, value: 2},
				{key: 5, value: 5},
				{key: 7, value: 7},
				{key: 9, value: 9},
			},
			removeNodeKey: 6,
			expectedTotal: 35,
			expectedOrder: []order{
				//	 		┌───┤ key=9, depth=1, value=9, total=9, clean=true
				//		┌───┤ key=8, depth=2, value=8, total=24, clean=true
				//		│	└───┤ key=7, depth=1, value=7, total=7, clean=true
				//	────┤ key=5, depth=3, value=5, total=41, clean=false
				//		└───┤ key=4, depth=2, value=4, total=11, clean=false
				//			└───┤ key=2, depth=1, value=2, total=2, clean=true
				{value: value{key: 2, value: 2}, clean: true, depth: 1},
				{value: value{key: 4, value: 4}, clean: false, depth: 2},
				{value: value{key: 7, value: 7}, clean: true, depth: 1},
				{value: value{key: 9, value: 9}, clean: true, depth: 1},
				{value: value{key: 8, value: 8}, clean: true, depth: 2},
				{value: value{key: 5, value: 5}, clean: false, depth: 3},
			},
		},
		{
			name: "add 7 nodes; removing a child causes right rotate",
			// 		┌───┤ key=75, depth=2, value=75, total=135, clean=true
			//		│	└───┤ key=60, depth=1, value=60, total=60, clean=true
			//	────┤ key=50, depth=4, value=50, total=251, clean=true
			//		│	┌───┤ key=30, depth=1, value=30, total=30, clean=true
			//		└───┤ key=25, depth=3, value=25, total=66, clean=true
			//			└───┤ key=10, depth=2, value=10, total=11, clean=true
			//				└───┤ key=1, depth=1, value=1, total=1, clean=true
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 25, value: 25},
				{key: 75, value: 75},
				{key: 10, value: 10},
				{key: 30, value: 30},
				{key: 60, value: 60},
				{key: 1, value: 1},
			},
			removeNodeKey: 60,
			expectedTotal: 191,
			expectedOrder: []order{
				// 			┌───┤ key=75, depth=1, value=75, total=135, clean=false
				//		┌───┤ key=50, depth=2, value=50, total=251, clean=false
				//		│	└───┤ key=30, depth=1, value=30, total=30, clean=true
				//	────┤ key=25, depth=3, value=25, total=66, clean=false
				//		└───┤ key=10, depth=2, value=10, total=11, clean=true
				//			└───┤ key=1, depth=1, value=1, total=1, clean=true
				{value: value{key: 1, value: 1}, clean: true, depth: 1},
				{value: value{key: 10, value: 10}, clean: true, depth: 2},
				{value: value{key: 30, value: 30}, clean: true, depth: 1},
				{value: value{key: 75, value: 75}, clean: false, depth: 1},
				{value: value{key: 50, value: 50}, clean: false, depth: 2},
				{value: value{key: 25, value: 25}, clean: false, depth: 3},
			},
		},
		{
			name: "add 7 nodes; removing a child causes left rotate",
			//				┌───┤ key=90, depth=1, value=90,
			//			┌───┤ key=80, depth=2, value=80,
			//		┌───┤ key=75, depth=3, value=75,
			//		│	└───┤ key=70, depth=1, value=70,
			//	────┤ key=50, depth=4, value=50,
			//		│	┌───┤ key=26, depth=1, value=26,
			//		└───┤ key=25, depth=2, value=25,
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 25, value: 25},
				{key: 75, value: 75},
				{key: 26, value: 26},
				{key: 70, value: 70},
				{key: 80, value: 80},
				{key: 90, value: 90},
			},
			removeNodeKey: 26,
			expectedTotal: 390,
			expectedOrder: []order{
				// 			┌───┤ key=90, depth=1, value=90, total=90, clean=true
				//		┌───┤ key=80, depth=2, value=80, total=170, clean=true
				//	────┤ key=75, depth=3, value=75, total=315, clean=false
				//		│	┌───┤ key=70, depth=1, value=70, total=70, clean=true
				//		└───┤ key=50, depth=2, value=50, total=416, clean=false
				//			└───┤ key=25, depth=1, value=25, total=51, clean=false
				{value: value{key: 25, value: 25}, clean: false, depth: 1},
				{value: value{key: 70, value: 70}, clean: true, depth: 1},
				{value: value{key: 50, value: 50}, clean: false, depth: 2},
				{value: value{key: 90, value: 90}, clean: true, depth: 1},
				{value: value{key: 80, value: 80}, clean: true, depth: 2},
				{value: value{key: 75, value: 75}, clean: false, depth: 3},
			},
		},
		{
			name: "add 12 nodes; removing a child causes double right rotate",
			//			┌───┤ key=90, depth=1, value=90, total=90, clean=true
			//		┌───┤ key=84, depth=3, value=84, total=330, clean=true
			//		│	└───┤ key=80, depth=2, value=80, total=156, clean=true
			//		│		└───┤ key=76, depth=1, value=76, total=76, clean=true
			//	────┤ key=75, depth=5, value=75, total=658, clean=true
			//		│		┌───┤ key=72, depth=1, value=72, total=72, clean=true
			//		│	┌───┤ key=70, depth=2, value=70, total=142, clean=true
			//		└───┤ key=50, depth=4, value=50, total=253, clean=true
			//			│	┌───┤ key=30, depth=1, value=30, total=30, clean=true
			//			└───┤ key=25, depth=3, value=25, total=61, clean=true
			//				└───┤ key=5, depth=2, value=5, total=6, clean=true
			//					└───┤ key=1, depth=1, value=1, total=1, clean=true
			valuesToAdd: []value{
				{key: 75, value: 75},
				{key: 50, value: 50},
				{key: 84, value: 84},
				{key: 25, value: 25},
				{key: 70, value: 70},
				{key: 80, value: 80},
				{key: 90, value: 90},
				{key: 5, value: 5},
				{key: 30, value: 30},
				{key: 72, value: 72},
				{key: 76, value: 76},
				{key: 1, value: 1},
			},
			removeNodeKey: 90,
			expectedTotal: 568,
			expectedOrder: []order{
				//				┌───┤ key=84, depth=1, value=84, total=330, clean=false
				//			┌───┤ key=80, depth=2, value=80, total=156, clean=false
				//			│	└───┤ key=76, depth=1, value=76, total=76, clean=true
				//		┌───┤ key=75, depth=3, value=75, total=658, clean=false
				//		│	│	┌───┤ key=72, depth=1, value=72, total=72, clean=true
				//		│	└───┤ key=70, depth=2, value=70, total=142, clean=true
				//	────┤ key=50, depth=4, value=50, total=253, clean=false
				//		│	┌───┤ key=30, depth=1, value=30, total=30, clean=true
				//		└───┤ key=25, depth=3, value=25, total=61, clean=true
				//			└───┤ key=5, depth=2, value=5, total=6, clean=true
				//				└───┤ key=1, depth=1, value=1, total=1, clean=true
				{value: value{key: 1, value: 1}, clean: true, depth: 1},
				{value: value{key: 5, value: 5}, clean: true, depth: 2},
				{value: value{key: 30, value: 30}, clean: true, depth: 1},
				{value: value{key: 25, value: 25}, clean: true, depth: 3},
				{value: value{key: 72, value: 72}, clean: true, depth: 1},
				{value: value{key: 70, value: 70}, clean: true, depth: 2},
				{value: value{key: 76, value: 76}, clean: true, depth: 1},
				{value: value{key: 84, value: 84}, clean: false, depth: 1},
				{value: value{key: 80, value: 80}, clean: false, depth: 2},
				{value: value{key: 75, value: 75}, clean: false, depth: 3},
				{value: value{key: 50, value: 50}, clean: false, depth: 4},
			},
		},
		{
			name: "add 12 nodes; removing a child causes double left rotate",
			//					┌───┤ key=100, depth=1, value=100, total=100, clean=true
			//				┌───┤ key=84, depth=2, value=84, total=184, clean=true
			//			┌───┤ key=80, depth=3, value=80, total=340, clean=true
			//			│	└───┤ key=76, depth=1, value=76, total=76, clean=true
			//		┌───┤ key=75, depth=4, value=75, total=557, clean=true
			//		│	│	┌───┤ key=72, depth=1, value=72, total=72, clean=true
			//		│	└───┤ key=70, depth=2, value=70, total=142, clean=true
			//	────┤ key=50, depth=5, value=50, total=707, clean=true
			//		│		┌───┤ key=40, depth=1, value=40, total=40, clean=true
			//		│	┌───┤ key=30, depth=2, value=30, total=70, clean=true
			//		└───┤ key=25, depth=3, value=25, total=100, clean=true
			//			└───┤ key=5, depth=1, value=5, total=5, clean=true
			valuesToAdd: []value{
				{key: 50, value: 50},
				{key: 75, value: 75},
				{key: 25, value: 25},
				{key: 5, value: 5},
				{key: 30, value: 30},
				{key: 70, value: 70},
				{key: 80, value: 80},
				{key: 40, value: 40},
				{key: 72, value: 72},
				{key: 76, value: 76},
				{key: 84, value: 84},
				{key: 100, value: 100},
			},
			removeNodeKey: 5,
			expectedTotal: 702,
			expectedOrder: []order{
				//				┌───┤ key=100, depth=1, value=100, total=100, clean=true
				//			┌───┤ key=84, depth=2, value=84, total=184, clean=true
				//		┌───┤ key=80, depth=3, value=80, total=340, clean=true
				//		│	└───┤ key=76, depth=1, value=76, total=76, clean=true
				//	────┤ key=75, depth=4, value=75, total=557, clean=false
				//		│		┌───┤ key=72, depth=1, value=72, total=72, clean=true
				//		│	┌───┤ key=70, depth=2, value=70, total=142, clean=true
				//		└───┤ key=50, depth=3, value=50, total=707, clean=false
				//			│	┌───┤ key=40, depth=1, value=40, total=40, clean=true
				//			└───┤ key=30, depth=2, value=30, total=70, clean=false
				//				└───┤ key=25, depth=1, value=25, total=100, clean=false
				{value: value{key: 25, value: 25}, clean: false, depth: 1},
				{value: value{key: 40, value: 40}, clean: true, depth: 1},
				{value: value{key: 30, value: 30}, clean: false, depth: 2},
				{value: value{key: 72, value: 72}, clean: true, depth: 1},
				{value: value{key: 70, value: 70}, clean: true, depth: 2},
				{value: value{key: 50, value: 50}, clean: false, depth: 3},
				{value: value{key: 76, value: 76}, clean: true, depth: 1},
				{value: value{key: 100, value: 100}, clean: true, depth: 1},
				{value: value{key: 84, value: 84}, clean: true, depth: 2},
				{value: value{key: 80, value: 80}, clean: true, depth: 3},
				{value: value{key: 75, value: 75}, clean: false, depth: 4},
			},
		},
		{
			name: "add 8 nodes; removing a child causes left right rotate",
			// 		┌───┤ key=10, depth=2, value=10, total=19, clean=true
			//		│	└───┤ key=9, depth=1, value=9, total=9, clean=true
			//	────┤ key=8, depth=4, value=8, total=51, clean=true
			//		│		┌───┤ key=7, depth=1, value=7, total=7, clean=true
			//		│	┌───┤ key=6, depth=2, value=6, total=18, clean=true
			//		│	│	└───┤ key=5, depth=1, value=5, total=5, clean=true
			//		└───┤ key=4, depth=3, value=4, total=24, clean=true
			//			└───┤ key=2, depth=1, value=2, total=2, clean=true
			valuesToAdd: []value{
				{key: 8, value: 8},
				{key: 4, value: 4},
				{key: 10, value: 10},
				{key: 2, value: 2},
				{key: 6, value: 6},
				{key: 9, value: 9},
				{key: 5, value: 5},
				{key: 7, value: 7},
			},
			removeNodeKey: 9,
			expectedTotal: 42,
			expectedOrder: []order{
				// 			┌───┤ key=10, depth=1, value=10, total=19, clean=false
				//		┌───┤ key=8, depth=2, value=8, total=51, clean=false
				//		│	└───┤ key=7, depth=1, value=7, total=7, clean=true
				//	────┤ key=6, depth=3, value=6, total=18, clean=false
				//		│	┌───┤ key=5, depth=1, value=5, total=5, clean=true
				//		└───┤ key=4, depth=2, value=4, total=24, clean=false
				//			└───┤ key=2, depth=1, value=2, total=2, clean=true
				{value: value{key: 2, value: 2}, clean: true, depth: 1},
				{value: value{key: 5, value: 5}, clean: true, depth: 1},
				{value: value{key: 4, value: 4}, clean: false, depth: 2},
				{value: value{key: 7, value: 7}, clean: true, depth: 1},
				{value: value{key: 10, value: 10}, clean: false, depth: 1},
				{value: value{key: 8, value: 8}, clean: false, depth: 2},
				{value: value{key: 6, value: 6}, clean: false, depth: 3},
			},
		},
		{
			name: "add 8 nodes; removing a child causes right left rotate",
			// 			┌───┤ key=9, depth=1, value=9, total=9, clean=true
			//		┌───┤ key=8, depth=3, value=8, total=35, clean=true
			//		│	│	┌───┤ key=7, depth=1, value=7, total=7, clean=true
			//		│	└───┤ key=6, depth=2, value=6, total=18, clean=true
			//		│		└───┤ key=5, depth=1, value=5, total=5, clean=true
			//	────┤ key=4, depth=4, value=4, total=44, clean=true
			//		│	┌───┤ key=3, depth=1, value=3, total=3, clean=true
			//		└───┤ key=2, depth=2, value=2, total=5, clean=true
			valuesToAdd: []value{
				{key: 4, value: 4},
				{key: 2, value: 2},
				{key: 8, value: 8},
				{key: 3, value: 3},
				{key: 6, value: 6},
				{key: 9, value: 9},
				{key: 5, value: 5},
				{key: 7, value: 7},
			},
			removeNodeKey: 3,
			expectedTotal: 41,
			expectedOrder: []order{
				// 			┌───┤ key=9, depth=1, value=9, total=9, clean=true
				//		┌───┤ key=8, depth=2, value=8, total=35, clean=false
				//		│	└───┤ key=7, depth=1, value=7, total=7, clean=true
				//	────┤ key=6, depth=3, value=6, total=18, clean=false
				//		│	┌───┤ key=5, depth=1, value=5, total=5, clean=true
				//		└───┤ key=4, depth=2, value=4, total=44, clean=false
				//			└───┤ key=2, depth=1, value=2, total=5, clean=false
				{value: value{key: 2, value: 2}, clean: false, depth: 1},
				{value: value{key: 5, value: 5}, clean: true, depth: 1},
				{value: value{key: 4, value: 4}, clean: false, depth: 2},
				{value: value{key: 7, value: 7}, clean: true, depth: 1},
				{value: value{key: 9, value: 9}, clean: true, depth: 1},
				{value: value{key: 8, value: 8}, clean: false, depth: 2},
				{value: value{key: 6, value: 6}, clean: false, depth: 3},
			},
		},
		{
			name: "removing the root node causes single right and double left rotate",
			valuesToAdd: []value{
				//	 			┌───┤ key=19, depth=1, value=19, total=19, clean=true
				//			┌───┤ key=18, depth=2, value=18, total=54, clean=true
				//			│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//		┌───┤ key=16, depth=4, value=16, total=118, clean=true
				//		│	│		┌───┤ key=15, depth=1, value=15, total=15, clean=true
				//		│	│	┌───┤ key=14, depth=2, value=14, total=29, clean=true
				//		│	└───┤ key=10, depth=3, value=10, total=48, clean=true
				//		│		└───┤ key=9, depth=1, value=9, total=9, clean=true
				//	────┤ key=8, depth=5, value=8, total=139, clean=true
				//		│	┌───┤ key=7, depth=1, value=7, total=7, clean=true
				//		└───┤ key=3, depth=3, value=3, total=13, clean=true
				//			└───┤ key=2, depth=2, value=2, total=3, clean=true
				//				└───┤ key=1, depth=1, value=1, total=1, clean=true
				{key: 8, value: 8},
				{key: 3, value: 3},
				{key: 16, value: 16},
				{key: 2, value: 2},
				{key: 7, value: 7},
				{key: 10, value: 10},
				{key: 18, value: 18},
				{key: 1, value: 1},
				{key: 9, value: 9},
				{key: 14, value: 14},
				{key: 17, value: 17},
				{key: 19, value: 19},
				{key: 15, value: 15},
				//{key: 20, value: 20},
			},
			removeNodeKey: 8,
			expectedTotal: 131,
			expectedOrder: []order{
				// 				┌───┤ key=19, depth=1, value=19, total=19, clean=true
				//			┌───┤ key=18, depth=2, value=18, total=54, clean=true
				//			│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//		┌───┤ key=16, depth=3, value=16, total=118, clean=false
				//		│	│	┌───┤ key=15, depth=1, value=15, total=15, clean=true
				//		│	└───┤ key=14, depth=2, value=14, total=29, clean=true
				//	────┤ key=10, depth=4, value=10, total=48, clean=false
				//		│	┌───┤ key=9, depth=1, value=9, total=9, clean=true
				//		└───┤ key=7, depth=3, value=7, total=139, clean=false
				//			│	┌───┤ key=3, depth=1, value=3, total=13, clean=false
				//			└───┤ key=2, depth=2, value=2, total=3, clean=false
				//				└───┤ key=1, depth=1, value=1, total=1, clean=true
				{value: value{key: 1, value: 1}, clean: true, depth: 1},
				{value: value{key: 3, value: 3}, clean: false, depth: 1},
				{value: value{key: 2, value: 2}, clean: false, depth: 2},
				{value: value{key: 9, value: 9}, clean: true, depth: 1},
				{value: value{key: 7, value: 7}, clean: false, depth: 3},
				{value: value{key: 15, value: 15}, clean: true, depth: 1},
				{value: value{key: 14, value: 14}, clean: true, depth: 2},
				{value: value{key: 17, value: 17}, clean: true, depth: 1},
				{value: value{key: 19, value: 19}, clean: true, depth: 1},
				{value: value{key: 18, value: 18}, clean: true, depth: 2},
				{value: value{key: 16, value: 16}, clean: false, depth: 3},
				{value: value{key: 10, value: 10}, clean: false, depth: 4},
			},
		},
		{
			name: "removing the root node causes single right and single left rotate",
			valuesToAdd: []value{
				// 					┌───┤ key=20, depth=1, value=20, total=20, clean=true
				//				┌───┤ key=19, depth=2, value=19, total=39, clean=true
				//			┌───┤ key=18, depth=3, value=18, total=74, clean=true
				//			│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//		┌───┤ key=16, depth=4, value=16, total=138, clean=true
				//		│	│		┌───┤ key=15, depth=1, value=15, total=15, clean=true
				//		│	│	┌───┤ key=14, depth=2, value=14, total=29, clean=true
				//		│	└───┤ key=10, depth=3, value=10, total=48, clean=true
				//		│		└───┤ key=9, depth=1, value=9, total=9, clean=true
				//	────┤ key=8, depth=5, value=8, total=159, clean=true
				//		│	┌───┤ key=7, depth=1, value=7, total=7, clean=true
				//		└───┤ key=3, depth=3, value=3, total=13, clean=true
				//			└───┤ key=2, depth=2, value=2, total=3, clean=true
				//				└───┤ key=1, depth=1, value=1, total=1, clean=true
				{key: 8, value: 8},
				{key: 3, value: 3},
				{key: 16, value: 16},
				{key: 2, value: 2},
				{key: 7, value: 7},
				{key: 10, value: 10},
				{key: 18, value: 18},
				{key: 1, value: 1},
				{key: 9, value: 9},
				{key: 14, value: 14},
				{key: 17, value: 17},
				{key: 19, value: 19},
				{key: 15, value: 15},
				{key: 20, value: 20},
			},
			removeNodeKey: 8,
			expectedTotal: 151,
			expectedOrder: []order{
				//				┌───┤ key=20, depth=1, value=20, total=20, clean=true
				//			┌───┤ key=19, depth=2, value=19, total=39, clean=true
				//		┌───┤ key=18, depth=3, value=18, total=74, clean=true
				//		│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//	────┤ key=16, depth=5, value=16, total=138, clean=false
				//		│			┌───┤ key=15, depth=1, value=15, total=15, clean=true
				//		│		┌───┤ key=14, depth=2, value=14, total=29, clean=true
				//		│	┌───┤ key=10, depth=3, value=10, total=48, clean=true
				//		│	│	└───┤ key=9, depth=1, value=9, total=9, clean=true
				//		└───┤ key=7, depth=4, value=7, total=159, clean=false
				//			│	┌───┤ key=3, depth=1, value=3, total=13, clean=false
				//			└───┤ key=2, depth=2, value=2, total=3, clean=false
				//				└───┤ key=1, depth=1, value=1, total=1, clean=true
				{value: value{key: 1, value: 1}, clean: true, depth: 1},
				{value: value{key: 3, value: 3}, clean: false, depth: 1},
				{value: value{key: 2, value: 2}, clean: false, depth: 2},
				{value: value{key: 9, value: 9}, clean: true, depth: 1},
				{value: value{key: 15, value: 15}, clean: true, depth: 1},
				{value: value{key: 14, value: 14}, clean: true, depth: 2},
				{value: value{key: 10, value: 10}, clean: true, depth: 3},
				{value: value{key: 7, value: 7}, clean: false, depth: 4},
				{value: value{key: 17, value: 17}, clean: true, depth: 1},
				{value: value{key: 20, value: 20}, clean: true, depth: 1},
				{value: value{key: 19, value: 19}, clean: true, depth: 2},
				{value: value{key: 18, value: 18}, clean: true, depth: 3},
				{value: value{key: 16, value: 16}, clean: false, depth: 5},
			},
		},
		{
			name: "removing root causes single left rotate",
			valuesToAdd: []value{
				//	 			┌───┤ key=20, depth=1, value=20, total=20, clean=true
				//				┌───┤ key=19, depth=2, value=19, total=39, clean=true
				//			┌───┤ key=18, depth=3, value=18, total=74, clean=true
				//			│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//		┌───┤ key=16, depth=4, value=16, total=139, clean=true
				//		│	│	┌───┤ key=14, depth=2, value=14, total=27, clean=true
				//		│	│	│	└───┤ key=13, depth=1, value=13, total=13, clean=true
				//		│	└───┤ key=12, depth=3, value=12, total=49, clean=true
				//		│		└───┤ key=10, depth=1, value=10, total=10, clean=true
				//	────┤ key=8, depth=5, value=8, total=166, clean=true
				//		│		┌───┤ key=7, depth=1, value=7, total=7, clean=true
				//		│	┌───┤ key=6, depth=2, value=6, total=13, clean=true
				//		└───┤ key=4, depth=3, value=4, total=19, clean=true
				//			└───┤ key=2, depth=1, value=2, total=2, clean=true
				{key: 8, value: 8},
				{key: 4, value: 4},
				{key: 16, value: 16},
				{key: 2, value: 2},
				{key: 6, value: 6},
				{key: 12, value: 12},
				{key: 18, value: 18},
				{key: 7, value: 7},
				{key: 10, value: 10},
				{key: 14, value: 14},
				{key: 17, value: 17},
				{key: 19, value: 19},
				{key: 13, value: 13},
				{key: 20, value: 20},
			},
			removeNodeKey: 8,
			expectedTotal: 158,
			expectedOrder: []order{
				// 				┌───┤ key=20, depth=1, value=20, total=20, clean=true
				//			┌───┤ key=19, depth=2, value=19, total=39, clean=true
				//		┌───┤ key=18, depth=3, value=18, total=74, clean=true
				//		│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//	────┤ key=16, depth=5, value=16, total=139, clean=false
				//		│		┌───┤ key=14, depth=2, value=14, total=27, clean=true
				//		│		│	└───┤ key=13, depth=1, value=13, total=13, clean=true
				//		│	┌───┤ key=12, depth=3, value=12, total=49, clean=true
				//		│	│	└───┤ key=10, depth=1, value=10, total=10, clean=true
				//		└───┤ key=7, depth=4, value=7, total=166, clean=false
				//			│	┌───┤ key=6, depth=1, value=6, total=13, clean=false
				//			└───┤ key=4, depth=2, value=4, total=19, clean=false
				//				└───┤ key=2, depth=1, value=2, total=2, clean=true
				{value: value{key: 2, value: 2}, clean: true, depth: 1},
				{value: value{key: 6, value: 6}, clean: false, depth: 1},
				{value: value{key: 4, value: 4}, clean: false, depth: 2},
				{value: value{key: 10, value: 10}, clean: true, depth: 1},
				{value: value{key: 13, value: 13}, clean: true, depth: 1},
				{value: value{key: 14, value: 14}, clean: true, depth: 2},
				{value: value{key: 12, value: 12}, clean: true, depth: 3},
				{value: value{key: 7, value: 7}, clean: false, depth: 4},
				{value: value{key: 17, value: 17}, clean: true, depth: 1},
				{value: value{key: 20, value: 20}, clean: true, depth: 1},
				{value: value{key: 19, value: 19}, clean: true, depth: 2},
				{value: value{key: 18, value: 18}, clean: true, depth: 3},
				{value: value{key: 16, value: 16}, clean: false, depth: 5},
			},
		},
		{
			name: "removing root causes single right rotate",
			valuesToAdd: []value{
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
				{key: 15, value: 15},
				{key: 8, value: 8},
				{key: 18, value: 18},
				{key: 2, value: 2},
				{key: 12, value: 12},
				{key: 17, value: 17},
				{key: 19, value: 19},
				{key: 7, value: 7},
				{key: 10, value: 10},
				{key: 13, value: 13},
				{key: 20, value: 20},
				{key: 9, value: 9},
			},
			removeNodeKey: 15,
			expectedTotal: 135,
			expectedOrder: []order{
				//	 			┌───┤ key=20, depth=1, value=20, total=20, clean=true
				//			┌───┤ key=19, depth=2, value=19, total=39, clean=true
				//		┌───┤ key=18, depth=3, value=18, total=74, clean=true
				//		│	└───┤ key=17, depth=1, value=17, total=17, clean=true
				//	────┤ key=13, depth=4, value=13, total=150, clean=false
				//		│		┌───┤ key=12, depth=1, value=12, total=44, clean=false
				//		│	┌───┤ key=10, depth=2, value=10, total=19, clean=false
				//		│	│	└───┤ key=9, depth=1, value=9, total=9, clean=true
				//		└───┤ key=8, depth=3, value=8, total=61, clean=false
				//			│	┌───┤ key=7, depth=1, value=7, total=7, clean=true
				//			└───┤ key=2, depth=2, value=2, total=9, clean=true
				{value: value{key: 7, value: 7}, clean: true, depth: 1},
				{value: value{key: 2, value: 2}, clean: true, depth: 2},
				{value: value{key: 9, value: 9}, clean: true, depth: 1},
				{value: value{key: 12, value: 12}, clean: false, depth: 1},
				{value: value{key: 10, value: 10}, clean: false, depth: 2},
				{value: value{key: 8, value: 8}, clean: false, depth: 3},
				{value: value{key: 17, value: 17}, clean: true, depth: 1},
				{value: value{key: 20, value: 20}, clean: true, depth: 1},
				{value: value{key: 19, value: 19}, clean: true, depth: 2},
				{value: value{key: 18, value: 18}, clean: true, depth: 3},
				{value: value{key: 13, value: 13}, clean: false, depth: 4},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree := newIntTree()
			for _, v := range test.valuesToAdd {
				require.NoError(t, tree.Add(v.key, newIntValue(v.value)))
			}
			require.NoError(t, tree.Commit())
			require.NoError(t, tree.Delete(test.removeNodeKey))
			postOrder := postOrderNodes(tree.root)
			require.Len(t, postOrder, len(test.expectedOrder))
			for i, o := range test.expectedOrder {
				node := postOrder[i]
				require.Equal(t, o.value.key, node.key, "invalid node key: %v, required: %v", node.key, o.value.key)
				require.Equal(t, o.value.value, node.value.value, "node %d has invalid value: %v, required: %v", node.key, node.value.value, o.value.value)
				require.Equal(t, o.clean, node.clean, "node %d has invalid clean flag: %v, required: %v", node.key, node.clean, o.clean)
				require.Equal(t, o.depth, node.depth, "node %d has invalid depth: %v, required: %v", node.key, node.depth, o.depth)
			}
			require.NoError(t, tree.Commit())
			require.Equal(t, test.expectedTotal, tree.Root().value.total)
		})
	}
}

func TestDelete_RandomTree_RandomDeletes(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	for i := 0; i < 3; i++ {
		numberOfKeys := randInt(111, 333)
		t.Run(fmt.Sprintf("random deletes.tree size %d", numberOfKeys),
			func(t *testing.T) {
				RandomTree(t, numberOfKeys)
			},
		)
	}

}

func RandomTree(t *testing.T, numberOfKeys int) {
	tree := newIntTree()
	var keys []IntKey
	for i := 1; i <= numberOfKeys; i++ {
		require.NoError(t, tree.Add(IntKey(i), newIntValue(int64(i))))
		keys = append(keys, IntKey(i))
	}
	for len(keys) > 0 {
		numberOfKeys := len(keys)
		randomKeyIndex := rand.Intn(numberOfKeys)
		key := keys[randomKeyIndex]
		keys = removeKey(keys, randomKeyIndex)
		treeStr := tree.String()
		require.NoError(t, tree.Delete(key))
		require.NoError(t, tree.Commit())
		correct := isNodeDepthCorrect(tree.root)
		require.True(t, correct, "tree before: \n %v key to remove: %v \n tree after:\n %v\n", treeStr, key, tree)
	}

}

func randInt(min int, max int) int {
	rand.Seed(time.Now().UTC().UnixNano())
	return min + rand.Intn(max-min)
}

func isNodeDepthCorrect[K Key[K], V Value[V]](n *Node[K, V]) bool {
	if n == nil {
		return true
	}
	if n.left != nil {
		if !isNodeDepthCorrect(n.left) {
			return false
		}
	}
	if n.right != nil {
		if !isNodeDepthCorrect(n.right) {
			return false
		}
	}
	ld, rd := n.left.Depth(), n.right.Depth()
	if ld > rd+1 || rd > ld+1 {
		return false
	}
	return true
}

func removeKey(slice []IntKey, s int) []IntKey {
	return append(slice[:s], slice[s+1:]...)
}

func postOrderNodes[K Key[K], V Value[V]](n *Node[K, V]) []*Node[K, V] {
	var nodes []*Node[K, V]
	if n == nil {
		return nil
	}
	nodes = append(nodes, postOrderNodes(n.left)...)
	nodes = append(nodes, postOrderNodes(n.right)...)
	return append(nodes, n)
}
