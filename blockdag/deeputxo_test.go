package blockdag

import (
	"fmt"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"path"
	"testing"

	"github.com/kaspanet/kaspad/logs"

	"github.com/kaspanet/kaspad/database"

	"github.com/kaspanet/kaspad/dagconfig"
)

func loadDAG() (*BlockDAG, error) {
	dagParams := &dagconfig.DevnetParams

	kaspadPath := "/home/mike/dev/tmp/kaspad_data/.kaspad"
	dbPath := path.Join(kaspadPath, "data", "devnet", "blocks_ffldb")
	db, err := database.Open("ffldb", dbPath, dagParams.Net)
	if err != nil {
		return nil, fmt.Errorf("Error opening database: %+s", err)
	}

	return New(&Config{
		DB:         db,
		DAGParams:  dagParams,
		TimeSource: NewMedianTime(),
	})
}

type nodeSelector func(dag *BlockDAG) *blockNode

func generateBlocks(dag *BlockDAG) {
	for i := 0; i < 10e3; i++ {
		collection := dag.UTXOSet().collection()
		tx := wire.NewNativeMsgTx(wire.TxVersion, nil, nil)
	}
}

func benchmarkRestoreUTXO(b *testing.B, selector nodeSelector) {
	log.SetLevel(logs.LevelOff)

	params := &dagconfig.SimnetParams
	params.BlockCoinbaseMaturity = 0

	// Create a new database and dag instance to run tests against.
	dag, teardownFunc, err := DAGSetup("benchmarkRestoreUTXO", Config{
		DAGParams: params,
	})
	if err != nil {
		b.Fatalf("Failed to setup dag instance: %v", err)
	}
	defer teardownFunc()

	block, err := PrepareBlockForTest(dag, []*daghash.Hash{dag.dagParams.GenesisHash}, nil)
	if err != nil {
		b.Fatalf("PrepareBlockForTest: %s", err)
	}
	isOrphan, isDelayed, err := dag.ProcessBlock(util.NewBlock(block), BFNoPoWCheck)
	if err != nil {
		b.Fatalf("ProcessBlock: %v", err)
	}
	if isDelayed {
		b.Fatalf("ProcessBlock: block1 " +
			"is too far in the future")
	}
	if isOrphan {
		b.Fatalf("ProcessBlock: block1 got unexpectedly orphaned")
	}

	dag, err := loadDAG()
	if err != nil {
		b.Fatalf("Error loading dag: %+s", err)
	}
	defer dag.db.Close()

	node := selector(dag)
	//	profileFile, err := os.Create("/tmp/profile")
	//	pprof.StartCPUProfile(profileFile)
	//	defer pprof.StopCPUProfile()
	//  if err != nil {
	//  	b.Fatalf("Error creating profile file: %s", err)
	//  }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := dag.restoreUTXO(node)
		if err != nil {
			b.Fatalf("Error restoringUTXO: %s", err)
		}
	}
}

func benchmarkNRestoreUTXO(b *testing.B, n int) {
	selector := func(dag *BlockDAG) *blockNode {
		current := dag.selectedTip()
		for i := 0; i < n; i++ {
			current = current.selectedParent
		}
		return current
	}
	benchmarkRestoreUTXO(b, selector)
}

func BenchmarkDeepRestoreUTXO(b *testing.B) {
	benchmarkRestoreUTXO(b, func(dag *BlockDAG) *blockNode { return dag.genesis.children.bluest() })
}

func BenchmarkRestoreUTXO(b *testing.B) {
	ns := []int{
		0,
		1,
		2,
		3,
		4,
		5,
		10,
		20,
		50,
		100,
		150,
		200,
		300,
		400,
		500,
		600,
		700,
		800,
		900,
		1000,
	}
	for _, n := range ns {
		b.Run(fmt.Sprintf("Benchmark%dRestoreUtxo", n),
			func(b *testing.B) { benchmarkNRestoreUTXO(b, n) })
	}
}
