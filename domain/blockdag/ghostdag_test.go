package blockdag

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/db/dbaccess"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
)

type testBlockData struct {
	parents                []string
	id                     string // id is a virtual entity that is used only for tests so we can define relations between blocks without knowing their hash
	expectedScore          uint64
	expectedSelectedParent string
	expectedBlues          []string
	hash                   *uint64
}

func newUint64(n uint64) *uint64 {
	return &n
}

// CalculateGhostDag prints the ghostdag result.
func TestPrintGhostDag(t *testing.T) {
	dagParams := dagconfig.SimnetParams

	tests := []struct {
		k            dagconfig.KType
		expectedReds []string
		genesisID    string
		dagData      []*testBlockData
	}{
		{
			k:         4,
			genesisID: "0",
			dagData: []*testBlockData{
				{
					id:      "1",
					hash:    newUint64(950),
					parents: []string{"0"},
				},
				{
					id:      "2",
					hash:    newUint64(400),
					parents: []string{"0"},
				},
				{
					id:      "3",
					hash:    newUint64(889),
					parents: []string{"0"},
				},
				{
					id:      "4",
					hash:    newUint64(875),
					parents: []string{"1"},
				},
				{
					id:      "5",
					hash:    newUint64(57),
					parents: []string{"2", "3"},
				},
				{
					id:      "6",
					hash:    newUint64(301),
					parents: []string{"3"},
				},
				{
					id:      "7",
					hash:    newUint64(33),
					parents: []string{"6"},
				},
				{
					id:      "8",
					hash:    newUint64(223),
					parents: []string{"1", "2"},
				},
				{
					id:      "9",
					hash:    newUint64(379),
					parents: []string{"5", "6"},
				},
				{
					id:      "10",
					hash:    newUint64(119),
					parents: []string{"8", "4"},
				},
				{
					id:      "11",
					hash:    newUint64(746),
					parents: []string{"7", "9"},
				},
				{
					id:      "12",
					hash:    newUint64(771),
					parents: []string{"10", "9"},
				},
				{
					id:      "13",
					hash:    newUint64(83),
					parents: []string{"5", "8"},
				},
				{
					id:      "14",
					hash:    newUint64(258),
					parents: []string{"13", "10"},
				},
				{
					id:      "15",
					hash:    newUint64(979),
					parents: []string{"11", "13"},
				},
				{
					id:      "16",
					hash:    newUint64(405),
					parents: []string{"11"},
				},
				{
					id:      "17",
					hash:    newUint64(508),
					parents: []string{"14"},
				},
				{
					id:      "18",
					hash:    newUint64(728),
					parents: []string{"13"},
				},
				{
					id:      "19",
					hash:    newUint64(30),
					parents: []string{"18", "15"},
				},
				{
					id:      "20",
					hash:    newUint64(705),
					parents: []string{"16", "17"},
				},
				{
					id:      "21",
					hash:    newUint64(32),
					parents: []string{"18", "20"},
				},
				{
					id:      "22",
					hash:    newUint64(242),
					parents: []string{"19", "21"},
				},
				{
					id:      "23",
					hash:    newUint64(207),
					parents: []string{"12", "17"},
				},
				{
					id:      "24",
					hash:    newUint64(268),
					parents: []string{"20", "23"},
				},
				{
					id:      "25",
					hash:    newUint64(589),
					parents: []string{"21"},
				},
				{
					id:      "26",
					hash:    newUint64(664),
					parents: []string{"22", "24", "25"},
				},
				{
					id:      "27",
					hash:    newUint64(280),
					parents: []string{"16"},
				},
				{
					id:      "28",
					hash:    newUint64(485),
					parents: []string{"23", "25"},
				},
				{
					id:      "29",
					hash:    newUint64(37),
					parents: []string{"26", "28"},
				},
				{
					id:      "30",
					hash:    newUint64(245),
					parents: []string{"27"},
				},
			},
		},
	}

	for i, test := range tests {
		func() {
			resetExtraNonceForTest()
			dagParams.K = test.k
			dag, teardownFunc, err := DAGSetup(fmt.Sprintf("TestGHOSTDAG%d", i), true, Config{
				DAGParams: &dagParams,
			})
			if err != nil {
				t.Fatalf("Failed to setup dag instance: %v", err)
			}
			defer teardownFunc()

			genesisNode := dag.genesis
			blockByIDMap := make(map[string]*blockNode)
			idByBlockMap := make(map[*blockNode]string)
			blockByIDMap[test.genesisID] = genesisNode
			idByBlockMap[genesisNode] = test.genesisID

			for _, blockData := range test.dagData {
				parents := blockSet{}
				for _, parentID := range blockData.parents {
					parent := blockByIDMap[parentID]
					parents.add(parent)
				}

				block, err := PrepareBlockForTest(dag, parents.hashes(), nil)
				if err != nil {
					t.Fatalf("TestPrintGhostDag: block %v got unexpected error from PrepareBlockForTest: %v", blockData.id, err)
				}

				utilBlock := util.NewBlock(block)
				if blockData.hash != nil {
					hash := utilBlock.Hash()
					bytes := [daghash.HashSize]byte{}
					binary.LittleEndian.PutUint64(bytes[:], *blockData.hash)
					hash.SetBytes(bytes[:])
				}
				isOrphan, isDelayed, err := dag.ProcessBlock(utilBlock, BFNoPoWCheck)
				if err != nil {
					t.Fatalf("TestPrintGhostDag: dag.ProcessBlock got unexpected error for block %v: %v", blockData.id, err)
				}
				if isDelayed {
					t.Fatalf("TestGHOSTDAG: block %s "+
						"is too far in the future", blockData.id)
				}
				if isOrphan {
					t.Fatalf("TestPrintGhostDag: block %v was unexpectedly orphan", blockData.id)
				}

				node, ok := dag.index.LookupNode(utilBlock.Hash())
				if !ok {
					t.Fatalf("block %s does not exist in the DAG", utilBlock.Hash())
				}

				blockByIDMap[blockData.id] = node
				idByBlockMap[node] = blockData.id

				bluesIDs := make([]string, 0, len(node.blues))
				for _, blue := range node.blues {
					bluesIDs = append(bluesIDs, idByBlockMap[blue])
				}
				selectedParentID := idByBlockMap[node.selectedParent]

				t.Errorf(" Test %d: Block %v. blues: %v. selectedParent: %v, blueScore: %v", i, blockData.id, bluesIDs,
					selectedParentID, node.blueScore)
			}

			reds := make(map[string]bool)

			for id := range blockByIDMap {
				reds[id] = true
			}

			for tip := &dag.virtual.blockNode; tip.selectedParent != nil; tip = tip.selectedParent {
				tipID := idByBlockMap[tip]
				delete(reds, tipID)
				for _, blue := range tip.blues {
					blueID := idByBlockMap[blue]
					delete(reds, blueID)
				}
			}
			redsIDs := make([]string, 0, len(reds))
			for id := range reds {
				redsIDs = append(redsIDs, id)
			}
			sort.Strings(redsIDs)
			t.Errorf("Test %d: Reds: %v", i, redsIDs)
		}()
	}
}

// TestGHOSTDAG iterates over several dag simulations, and checks
// that the blue score, blue set and selected parent of each
// block are calculated as expected.
func TestGHOSTDAG(t *testing.T) {
	dagParams := dagconfig.SimnetParams

	tests := []struct {
		k            dagconfig.KType
		expectedReds []string
		dagData      []*testBlockData
	}{
		{
			k:            3,
			expectedReds: []string{"F", "G", "H", "I", "O", "P"},
			dagData: []*testBlockData{
				{
					parents:                []string{"A"},
					id:                     "B",
					expectedScore:          1,
					expectedSelectedParent: "A",
					expectedBlues:          []string{"A"},
				},
				{
					parents:                []string{"B"},
					id:                     "C",
					expectedScore:          2,
					expectedSelectedParent: "B",
					expectedBlues:          []string{"B"},
				},
				{
					parents:                []string{"A"},
					id:                     "D",
					expectedScore:          1,
					expectedSelectedParent: "A",
					expectedBlues:          []string{"A"},
				},
				{
					parents:                []string{"C", "D"},
					id:                     "E",
					expectedScore:          4,
					expectedSelectedParent: "C",
					expectedBlues:          []string{"C", "D"},
				},
				{
					parents:                []string{"A"},
					id:                     "F",
					expectedScore:          1,
					expectedSelectedParent: "A",
					expectedBlues:          []string{"A"},
				},
				{
					parents:                []string{"F"},
					id:                     "G",
					expectedScore:          2,
					expectedSelectedParent: "F",
					expectedBlues:          []string{"F"},
				},
				{
					parents:                []string{"A"},
					id:                     "H",
					expectedScore:          1,
					expectedSelectedParent: "A",
					expectedBlues:          []string{"A"},
				},
				{
					parents:                []string{"A"},
					id:                     "I",
					expectedScore:          1,
					expectedSelectedParent: "A",
					expectedBlues:          []string{"A"},
				},
				{
					parents:                []string{"E", "G"},
					id:                     "J",
					expectedScore:          5,
					expectedSelectedParent: "E",
					expectedBlues:          []string{"E"},
				},
				{
					parents:                []string{"J"},
					id:                     "K",
					expectedScore:          6,
					expectedSelectedParent: "J",
					expectedBlues:          []string{"J"},
				},
				{
					parents:                []string{"I", "K"},
					id:                     "L",
					expectedScore:          7,
					expectedSelectedParent: "K",
					expectedBlues:          []string{"K"},
				},
				{
					parents:                []string{"L"},
					id:                     "M",
					expectedScore:          8,
					expectedSelectedParent: "L",
					expectedBlues:          []string{"L"},
				},
				{
					parents:                []string{"M"},
					id:                     "N",
					expectedScore:          9,
					expectedSelectedParent: "M",
					expectedBlues:          []string{"M"},
				},
				{
					parents:                []string{"M"},
					id:                     "O",
					expectedScore:          9,
					expectedSelectedParent: "M",
					expectedBlues:          []string{"M"},
				},
				{
					parents:                []string{"M"},
					id:                     "P",
					expectedScore:          9,
					expectedSelectedParent: "M",
					expectedBlues:          []string{"M"},
				},
				{
					parents:                []string{"M"},
					id:                     "Q",
					expectedScore:          9,
					expectedSelectedParent: "M",
					expectedBlues:          []string{"M"},
				},
				{
					parents:                []string{"M"},
					id:                     "R",
					expectedScore:          9,
					expectedSelectedParent: "M",
					expectedBlues:          []string{"M"},
				},
				{
					parents:                []string{"R"},
					id:                     "S",
					expectedScore:          10,
					expectedSelectedParent: "R",
					expectedBlues:          []string{"R"},
				},
				{
					parents:                []string{"N", "O", "P", "Q", "S"},
					id:                     "T",
					expectedScore:          13,
					expectedSelectedParent: "S",
					expectedBlues:          []string{"S", "Q", "N"},
				},
			},
		},
	}

	for i, test := range tests {
		func() {
			resetExtraNonceForTest()
			dagParams.K = test.k
			dag, teardownFunc, err := DAGSetup(fmt.Sprintf("TestGHOSTDAG%d", i), true, Config{
				DAGParams: &dagParams,
			})
			if err != nil {
				t.Fatalf("Failed to setup dag instance: %v", err)
			}
			defer teardownFunc()

			genesisNode := dag.genesis
			blockByIDMap := make(map[string]*blockNode)
			idByBlockMap := make(map[*blockNode]string)
			blockByIDMap["A"] = genesisNode
			idByBlockMap[genesisNode] = "A"

			for _, blockData := range test.dagData {
				parents := blockSet{}
				for _, parentID := range blockData.parents {
					parent := blockByIDMap[parentID]
					parents.add(parent)
				}

				block, err := PrepareBlockForTest(dag, parents.hashes(), nil)
				if err != nil {
					t.Fatalf("TestGHOSTDAG: block %v got unexpected error from PrepareBlockForTest: %v", blockData.id, err)
				}

				utilBlock := util.NewBlock(block)
				isOrphan, isDelayed, err := dag.ProcessBlock(utilBlock, BFNoPoWCheck)
				if err != nil {
					t.Fatalf("TestGHOSTDAG: dag.ProcessBlock got unexpected error for block %v: %v", blockData.id, err)
				}
				if isDelayed {
					t.Fatalf("TestGHOSTDAG: block %s "+
						"is too far in the future", blockData.id)
				}
				if isOrphan {
					t.Fatalf("TestGHOSTDAG: block %v was unexpectedly orphan", blockData.id)
				}

				node, ok := dag.index.LookupNode(utilBlock.Hash())
				if !ok {
					t.Fatalf("block %s does not exist in the DAG", utilBlock.Hash())
				}

				blockByIDMap[blockData.id] = node
				idByBlockMap[node] = blockData.id

				bluesIDs := make([]string, 0, len(node.blues))
				for _, blue := range node.blues {
					bluesIDs = append(bluesIDs, idByBlockMap[blue])
				}
				selectedParentID := idByBlockMap[node.selectedParent]
				fullDataStr := fmt.Sprintf("blues: %v, selectedParent: %v, score: %v",
					bluesIDs, selectedParentID, node.blueScore)
				if blockData.expectedScore != node.blueScore {
					t.Errorf("Test %d: Block %v expected to have score %v but got %v (fulldata: %v)",
						i, blockData.id, blockData.expectedScore, node.blueScore, fullDataStr)
				}
				if blockData.expectedSelectedParent != selectedParentID {
					t.Errorf("Test %d: Block %v expected to have selected parent %v but got %v (fulldata: %v)",
						i, blockData.id, blockData.expectedSelectedParent, selectedParentID, fullDataStr)
				}
				if !reflect.DeepEqual(blockData.expectedBlues, bluesIDs) {
					t.Errorf("Test %d: Block %v expected to have blues %v but got %v (fulldata: %v)",
						i, blockData.id, blockData.expectedBlues, bluesIDs, fullDataStr)
				}
			}

			reds := make(map[string]bool)

			for id := range blockByIDMap {
				reds[id] = true
			}

			for tip := &dag.virtual.blockNode; tip.selectedParent != nil; tip = tip.selectedParent {
				tipID := idByBlockMap[tip]
				delete(reds, tipID)
				for _, blue := range tip.blues {
					blueID := idByBlockMap[blue]
					delete(reds, blueID)
				}
			}
			if !checkReds(test.expectedReds, reds) {
				redsIDs := make([]string, 0, len(reds))
				for id := range reds {
					redsIDs = append(redsIDs, id)
				}
				sort.Strings(redsIDs)
				sort.Strings(test.expectedReds)
				t.Errorf("Test %d: Expected reds %v but got %v", i, test.expectedReds, redsIDs)
			}
		}()
	}
}

func checkReds(expectedReds []string, reds map[string]bool) bool {
	if len(expectedReds) != len(reds) {
		return false
	}
	for _, redID := range expectedReds {
		if !reds[redID] {
			return false
		}
	}
	return true
}

func TestBlueAnticoneSizeErrors(t *testing.T) {
	// Create a new database and DAG instance to run tests against.
	dag, teardownFunc, err := DAGSetup("TestBlueAnticoneSizeErrors", true, Config{
		DAGParams: &dagconfig.SimnetParams,
	})
	if err != nil {
		t.Fatalf("TestBlueAnticoneSizeErrors: Failed to setup DAG instance: %s", err)
	}
	defer teardownFunc()

	// Prepare a block chain with size K beginning with the genesis block
	currentBlockA := dag.Params.GenesisBlock
	for i := dagconfig.KType(0); i < dag.Params.K; i++ {
		newBlock := prepareAndProcessBlockByParentMsgBlocks(t, dag, currentBlockA)
		currentBlockA = newBlock
	}

	// Prepare another block chain with size K beginning with the genesis block
	currentBlockB := dag.Params.GenesisBlock
	for i := dagconfig.KType(0); i < dag.Params.K; i++ {
		newBlock := prepareAndProcessBlockByParentMsgBlocks(t, dag, currentBlockB)
		currentBlockB = newBlock
	}

	// Get references to the tips of the two chains
	blockNodeA, ok := dag.index.LookupNode(currentBlockA.BlockHash())
	if !ok {
		t.Fatalf("block %s does not exist in the DAG", currentBlockA.BlockHash())
	}

	blockNodeB, ok := dag.index.LookupNode(currentBlockB.BlockHash())
	if !ok {
		t.Fatalf("block %s does not exist in the DAG", currentBlockB.BlockHash())
	}

	// Try getting the blueAnticoneSize between them. Since the two
	// blocks are not in the anticones of eachother, this should fail.
	_, err = dag.blueAnticoneSize(blockNodeA, blockNodeB)
	if err == nil {
		t.Fatalf("TestBlueAnticoneSizeErrors: blueAnticoneSize unexpectedly succeeded")
	}
	expectedErrSubstring := "is not in blue set of"
	if !strings.Contains(err.Error(), expectedErrSubstring) {
		t.Fatalf("TestBlueAnticoneSizeErrors: blueAnticoneSize returned wrong error. "+
			"Want: %s, got: %s", expectedErrSubstring, err)
	}
}

func TestGHOSTDAGErrors(t *testing.T) {
	// Create a new database and DAG instance to run tests against.
	dag, teardownFunc, err := DAGSetup("TestGHOSTDAGErrors", true, Config{
		DAGParams: &dagconfig.SimnetParams,
	})
	if err != nil {
		t.Fatalf("TestGHOSTDAGErrors: Failed to setup DAG instance: %s", err)
	}
	defer teardownFunc()

	// Add two child blocks to the genesis
	block1 := prepareAndProcessBlockByParentMsgBlocks(t, dag, dag.Params.GenesisBlock)
	block2 := prepareAndProcessBlockByParentMsgBlocks(t, dag, dag.Params.GenesisBlock)

	// Add a child block to the previous two blocks
	block3 := prepareAndProcessBlockByParentMsgBlocks(t, dag, block1, block2)

	// Clear the reachability store
	dag.reachabilityTree.store.loaded = map[daghash.Hash]*reachabilityData{}

	dbTx, err := dag.databaseContext.NewTx()
	if err != nil {
		t.Fatalf("NewTx: %s", err)
	}
	defer dbTx.RollbackUnlessClosed()

	err = dbaccess.ClearReachabilityData(dbTx)
	if err != nil {
		t.Fatalf("ClearReachabilityData: %s", err)
	}

	err = dbTx.Commit()
	if err != nil {
		t.Fatalf("Commit: %s", err)
	}

	// Try to rerun GHOSTDAG on the last block. GHOSTDAG uses
	// reachability data, so we expect it to fail.
	blockNode3, ok := dag.index.LookupNode(block3.BlockHash())
	if !ok {
		t.Fatalf("block %s does not exist in the DAG", block3.BlockHash())
	}
	_, err = dag.ghostdag(blockNode3)
	if err == nil {
		t.Fatalf("TestGHOSTDAGErrors: ghostdag unexpectedly succeeded")
	}
	expectedErrSubstring := "couldn't find reachability data"
	if !strings.Contains(err.Error(), expectedErrSubstring) {
		t.Fatalf("TestGHOSTDAGErrors: ghostdag returned wrong error. "+
			"Want: %s, got: %s", expectedErrSubstring, err)
	}
}
