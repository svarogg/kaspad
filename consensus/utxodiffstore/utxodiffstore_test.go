package utxodiffstore

// TODO: restore these tests
//func TestUTXODiffStore(t *testing.T) {
//	// Create a new database and DAG instance to run tests against.
//	dag, teardownFunc, err := blockdag.DAGSetup("TestUTXODiffStore", true, blockdag.Config{
//		DAGParams: &dagconfig.SimnetParams,
//	})
//	if err != nil {
//		t.Fatalf("TestUTXODiffStore: Failed to setup DAG instance: %v", err)
//	}
//	defer teardownFunc()
//
//	nodeCounter := byte(0)
//	createNode := func() *blocknode.BlockNode {
//		nodeCounter++
//		node := blocknode.NewBlockNodeForTest(&daghash.Hash{nodeCounter})
//		dag.blockNodeStore.AddNode(node)
//		return node
//	}
//
//	// Check that an error is returned when asking for non existing node
//	nonExistingNode := createNode()
//	_, err = dag.UTXODiffStore.DiffByNode(nonExistingNode)
//	if !dbaccess.IsNotFoundError(err) {
//		if err != nil {
//			t.Errorf("DiffByNode: %s", err)
//		} else {
//			t.Errorf("DiffByNode: unexpectedly found diff data")
//		}
//	}
//
//	// Add node's diff data to the UTXODiffStore and check if it's checked correctly.
//	node := createNode()
//	diff := NewUTXODiff()
//
//	entryToAdd := NewUTXOEntry(&wire.TxOut{Value: 1, ScriptPubKey: []byte{0x01}}, false, 0)
//	err = diff.AddEntry(wire.Outpoint{TxID: daghash.TxID{0x01}, Index: 0}, entryToAdd)
//	if err != nil {
//		t.Fatalf("AddEntry: unexpected error: %s", err)
//	}
//
//	entryToRemove := NewUTXOEntry(&wire.TxOut{Value: 2, ScriptPubKey: []byte{0x02}}, false, 0)
//	err = diff.RemoveEntry(wire.Outpoint{TxID: daghash.TxID{0x02}, Index: 0}, entryToRemove)
//	if err != nil {
//		t.Fatalf("RemoveEntry: unexpected error: %s", err)
//	}
//
//	if err := dag.UTXODiffStore.SetBlockDiff(node, diff); err != nil {
//		t.Fatalf("SetBlockDiff: unexpected error: %s", err)
//	}
//	diffChild := createNode()
//	if err := dag.UTXODiffStore.SetBlockDiffChild(node, diffChild); err != nil {
//		t.Fatalf("SetBlockDiffChild: unexpected error: %s", err)
//	}
//
//	if storeDiff, err := dag.UTXODiffStore.DiffByNode(node); err != nil {
//		t.Fatalf("DiffByNode: unexpected error: %s", err)
//	} else if !reflect.DeepEqual(storeDiff, diff) {
//		t.Errorf("Expected diff and storeDiff to be equal")
//	}
//
//	if storeDiffChild, err := dag.UTXODiffStore.DiffChildByNode(node); err != nil {
//		t.Fatalf("DiffByNode: unexpected error: %s", err)
//	} else if !reflect.DeepEqual(storeDiffChild, diffChild) {
//		t.Errorf("Expected diff and storeDiff to be equal")
//	}
//
//	// Flush changes to db, delete them from the dag.UTXODiffStore.loaded
//	// map, and check if the diff data is re-fetched from the database.
//	dbTx, err := dag.databaseContext.NewTx()
//	if err != nil {
//		t.Fatalf("Failed to open database transaction: %s", err)
//	}
//	defer dbTx.RollbackUnlessClosed()
//	err = dag.UTXODiffStore.FlushToDB(dbTx)
//	if err != nil {
//		t.Fatalf("Error flushing UTXODiffStore data to DB: %s", err)
//	}
//	err = dbTx.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit database transaction: %s", err)
//	}
//	delete(dag.UTXODiffStore.loaded, node)
//
//	if storeDiff, err := dag.UTXODiffStore.DiffByNode(node); err != nil {
//		t.Fatalf("DiffByNode: unexpected error: %s", err)
//	} else if !reflect.DeepEqual(storeDiff, diff) {
//		t.Errorf("Expected diff and storeDiff to be equal")
//	}
//
//	// Check if getBlockDiff caches the result in dag.UTXODiffStore.loaded
//	if loadedDiffData, ok := dag.UTXODiffStore.loaded[node]; !ok {
//		t.Errorf("the diff data wasn't added to loaded map after requesting it")
//	} else if !reflect.DeepEqual(loadedDiffData.diff, diff) {
//		t.Errorf("Expected diff and loadedDiff to be equal")
//	}
//}
//
//func TestClearOldEntries(t *testing.T) {
//	// Create a new database and DAG instance to run tests against.
//	dag, teardownFunc, err := blockdag.DAGSetup("TestClearOldEntries", true, blockdag.Config{
//		DAGParams: &dagconfig.SimnetParams,
//	})
//	if err != nil {
//		t.Fatalf("TestClearOldEntries: Failed to setup DAG instance: %v", err)
//	}
//	defer teardownFunc()
//
//	// Set maxBlueScoreDifferenceToKeepLoaded to 10 to make this test fast to run
//	currentDifference := maxBlueScoreDifferenceToKeepLoaded
//	maxBlueScoreDifferenceToKeepLoaded = 10
//	defer func() { maxBlueScoreDifferenceToKeepLoaded = currentDifference }()
//
//	// Add 10 blocks
//	blockNodes := make([]*blocknode.BlockNode, 10)
//	for i := 0; i < 10; i++ {
//		processedBlock := blockdag.PrepareAndProcessBlockForTest(t, dag, dag.TipHashes(), nil)
//
//		node, ok := dag.blockNodeStore.LookupNode(processedBlock.BlockHash())
//		if !ok {
//			t.Fatalf("TestClearOldEntries: missing BlockNode for hash %s", processedBlock.BlockHash())
//		}
//		blockNodes[i] = node
//	}
//
//	// Make sure that all of them exist in the loaded set
//	for _, node := range blockNodes {
//		_, ok := dag.UTXODiffStore.loaded[node]
//		if !ok {
//			t.Fatalf("TestClearOldEntries: diffData for node %s is not in the loaded set", node.Hash())
//		}
//	}
//
//	// Add 10 more blocks on top of the others
//	for i := 0; i < 10; i++ {
//		blockdag.PrepareAndProcessBlockForTest(t, dag, dag.TipHashes(), nil)
//	}
//
//	// Make sure that all the old nodes no longer exist in the loaded set
//	for _, node := range blockNodes {
//		_, ok := dag.UTXODiffStore.loaded[node]
//		if ok {
//			t.Fatalf("TestClearOldEntries: diffData for node %s is in the loaded set", node.Hash())
//		}
//	}
//
//	// Add a block on top of the genesis to force the retrieval of all diffData
//	processedBlock := blockdag.PrepareAndProcessBlockForTest(t, dag, []*daghash.Hash{dag.genesis.Hash()}, nil)
//	node, ok := dag.blockNodeStore.LookupNode(processedBlock.BlockHash())
//	if !ok {
//		t.Fatalf("TestClearOldEntries: missing BlockNode for hash %s", processedBlock.BlockHash())
//	}
//
//	// Make sure that the child-of-genesis node is in the loaded set, since it
//	// is a tip.
//	_, ok = dag.UTXODiffStore.loaded[node]
//	if !ok {
//		t.Fatalf("TestClearOldEntries: diffData for node %s is not in the loaded set", node.Hash())
//	}
//
//	// Make sure that all the old nodes still do not exist in the loaded set
//	for _, node := range blockNodes {
//		_, ok := dag.UTXODiffStore.loaded[node]
//		if ok {
//			t.Fatalf("TestClearOldEntries: diffData for node %s is in the loaded set", node.Hash())
//		}
//	}
//}
