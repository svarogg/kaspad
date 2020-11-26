package difficultymanager_test

import (
	"github.com/kaspanet/kaspad/domain/consensus"
	consensusdatabase "github.com/kaspanet/kaspad/domain/consensus/database"
	"github.com/kaspanet/kaspad/domain/consensus/datastructures/blockheaderstore"
	"github.com/kaspanet/kaspad/domain/consensus/datastructures/blockrelationstore"
	"github.com/kaspanet/kaspad/domain/consensus/datastructures/ghostdagdatastore"
	"github.com/kaspanet/kaspad/domain/consensus/datastructures/reachabilitydatastore"
	"github.com/kaspanet/kaspad/domain/consensus/model"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/processes/dagtopologymanager"
	"github.com/kaspanet/kaspad/domain/consensus/processes/dagtraversalmanager"
	"github.com/kaspanet/kaspad/domain/consensus/processes/difficultymanager"
	"github.com/kaspanet/kaspad/domain/consensus/processes/ghostdagmanager"
	"github.com/kaspanet/kaspad/domain/consensus/processes/reachabilitymanager"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	infrastructuredatabase "github.com/kaspanet/kaspad/infrastructure/db/database"
	"github.com/kaspanet/kaspad/infrastructure/db/database/ldb"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var TestZeroedBlockHash = &externalapi.DomainHash{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

func setupDBForTest(dbName string) (infrastructuredatabase.Database, func(), error) {
	var err error
	tmpDir, err := ioutil.TempDir("", "setupDBManager")
	if err != nil {
		return nil, nil, errors.Errorf("error creating temp dir: %s", err)
	}
	dbPath := filepath.Join(tmpDir, dbName)
	_ = os.RemoveAll(dbPath)
	db, err := ldb.NewLevelDB(dbPath)
	if err != nil {
		return nil, nil, err
	}

	originalLDBOptions := ldb.Options
	ldb.Options = func() *opt.Options {
		return nil
	}
	teardown := func() {
		_ = db.Close()
		ldb.Options = originalLDBOptions
		_ = os.RemoveAll(dbPath)
	}

	return db, teardown, err
}

func InitTestContext(dbManager model.DBManager, dagParams *dagconfig.Params) model.DifficultyManager {
	reachabilityDataStore := reachabilitydatastore.New()
	blockRelationStore := blockrelationstore.New()
	blockHeaderStore, _ := blockheaderstore.New(dbManager)
	ghostdagDataStore := ghostdagdatastore.New()
	reachabilityManager := reachabilitymanager.New(
		dbManager,
		ghostdagDataStore,
		reachabilityDataStore)
	dagTopologyManager := dagtopologymanager.New(
		dbManager,
		reachabilityManager,
		blockRelationStore)
	ghostdagManager := ghostdagmanager.New(
		dbManager,
		dagTopologyManager,
		ghostdagDataStore,
		model.KType(dagParams.K))
	dagTraversalManager := dagtraversalmanager.New(
		dbManager,
		dagTopologyManager,
		ghostdagDataStore,
		ghostdagManager)
	difficultyManager := difficultymanager.New(
		dbManager,
		ghostdagManager,
		ghostdagDataStore,
		blockHeaderStore,
		dagTopologyManager,
		dagTraversalManager,
		dagParams.PowMax,
		dagParams.DifficultyAdjustmentWindowSize,
		dagParams.TargetTimePerBlock)

	return difficultyManager
}

func TestRequiredDifficulty(t *testing.T) {
	dagParams := &dagconfig.SimnetParams
	consensusFactory := consensus.NewFactory()
	db, teardownFunc, err := setupDBForTest(t.Name())
	if err != nil {
		t.Fatalf("Failed to setup db: %v", err)
	}
	defer teardownFunc()

	dbManager := consensusdatabase.New(db)

	t.Run("Try with some constant hashes", func(t *testing.T) {
		_, err := consensusFactory.NewConsensus(dagParams, db)
		if err != nil {
			t.Fatalf("NewConsensus: %v", err)
		}

		difficultyManager := InitTestContext(dbManager, dagParams)
		_, err = difficultyManager.RequiredDifficulty(model.VirtualBlockHash)
		if err != nil {
			t.Fatalf("RequiredDifficulty: failed getting required difficulty:error-%v", err)
		}

		_, err = difficultyManager.RequiredDifficulty(TestZeroedBlockHash)
		if err == nil {
			t.Fatalf("RequiredDifficulty: failed. Got required difficulty from zeroed hash:error-%v", err)
		}

	})

}
