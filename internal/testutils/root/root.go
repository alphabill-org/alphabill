package root

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/unicitytree"
)

type TestRootChain struct {
	ut            unicitytree.UnicityTree
	genesisBlocks map[string]*partition.UnicityCertificateRecord
}
