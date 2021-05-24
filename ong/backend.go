// Copyright 2014 The go-orange Authors
// This file is part of the go-orange library.
//
// The go-orange library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-orange library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-orange library. If not, see <http://www.gnu.org/licenses/>.

// Package ong implements the Orange protocol.
package ong

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ong2020/go-orange/accounts"
	"github.com/ong2020/go-orange/common"
	"github.com/ong2020/go-orange/common/hexutil"
	"github.com/ong2020/go-orange/consensus"
	"github.com/ong2020/go-orange/consensus/clique"
	"github.com/ong2020/go-orange/core"
	"github.com/ong2020/go-orange/core/bloombits"
	"github.com/ong2020/go-orange/core/rawdb"
	"github.com/ong2020/go-orange/core/state/pruner"
	"github.com/ong2020/go-orange/core/types"
	"github.com/ong2020/go-orange/core/vm"
	"github.com/ong2020/go-orange/event"
	"github.com/ong2020/go-orange/internal/ongapi"
	"github.com/ong2020/go-orange/log"
	"github.com/ong2020/go-orange/miner"
	"github.com/ong2020/go-orange/node"
	"github.com/ong2020/go-orange/ong/downloader"
	"github.com/ong2020/go-orange/ong/filters"
	"github.com/ong2020/go-orange/ong/gasprice"
	"github.com/ong2020/go-orange/ong/ongconfig"
	"github.com/ong2020/go-orange/ong/protocols/ong"
	"github.com/ong2020/go-orange/ong/protocols/snap"
	"github.com/ong2020/go-orange/ongdb"
	"github.com/ong2020/go-orange/p2p"
	"github.com/ong2020/go-orange/p2p/enode"
	"github.com/ong2020/go-orange/params"
	"github.com/ong2020/go-orange/rlp"
	"github.com/ong2020/go-orange/rpc"
)

// Config contains the configuration options of the ONG protocol.
// Deprecated: use ongconfig.Config instead.
type Config = ongconfig.Config

// Orange implements the Orange full node service.
type Orange struct {
	config *ongconfig.Config

	// Handlers
	txPool             *core.TxPool
	blockchain         *core.BlockChain
	handler            *handler
	ongDialCandidates  enode.Iterator
	snapDialCandidates enode.Iterator

	// DB interfaces
	chainDb ongdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests     chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer      *core.ChainIndexer             // Bloom indexer operating during block imports
	closeBloomHandler chan struct{}

	APIBackend *OngAPIBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	ongerbase common.Address

	networkID     uint64
	netRPCService *ongapi.PublicNetAPI

	p2pServer *p2p.Server

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and ongerbase)
}

// New creates a new Orange object (including the
// initialisation of the common Orange object)
func New(stack *node.Node, config *ongconfig.Config) (*Orange, error) {
	// Ensure configuration values are compatible and sane
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run ong.Orange in light sync mode, use les.LightOrange")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if config.Miner.GasPrice == nil || config.Miner.GasPrice.Cmp(common.Big0) <= 0 {
		log.Warn("Sanitizing invalid miner gas price", "provided", config.Miner.GasPrice, "updated", ongconfig.Defaults.Miner.GasPrice)
		config.Miner.GasPrice = new(big.Int).Set(ongconfig.Defaults.Miner.GasPrice)
	}
	if config.NoPruning && config.TrieDirtyCache > 0 {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	// Assemble the Orange object
	chainDb, err := stack.OpenDatabaseWithFreezer("chaindata", config.DatabaseCache, config.DatabaseHandles, config.DatabaseFreezer, "ong/db/chaindata/")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlockWithOverride(chainDb, config.Genesis, config.OverrideBerlin)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb, stack.ResolvePath(config.TrieCleanCacheJournal)); err != nil {
		log.Error("Failed to recover state", "error", err)
	}
	ong := &Orange{
		config:            config,
		chainDb:           chainDb,
		eventMux:          stack.EventMux(),
		accountManager:    stack.AccountManager(),
		engine:            ongconfig.CreateConsensusEngine(stack, chainConfig, &config.Ongash, config.Miner.Notify, config.Miner.Noverify, chainDb),
		closeBloomHandler: make(chan struct{}),
		networkID:         config.NetworkId,
		gasPrice:          config.Miner.GasPrice,
		ongerbase:         config.Miner.Orangerbase,
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      core.NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms),
		p2pServer:         stack.Server(),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Info("Initialising Orange protocol", "network", config.NetworkId, "dbversion", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Gong %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	var (
		vmConfig = vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
			EWASMInterpreter:        config.EWASMInterpreter,
			EVMInterpreter:          config.EVMInterpreter,
		}
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanJournal:    stack.ResolvePath(config.TrieCleanCacheJournal),
			TrieCleanRejournal:  config.TrieCleanCacheRejournal,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
			SnapshotLimit:       config.SnapshotCache,
			Preimages:           config.Preimages,
		}
	)
	ong.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, chainConfig, ong.engine, vmConfig, ong.shouldPreserve, &config.TxLookupLimit)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		ong.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	ong.bloomIndexer.Start(ong.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
	}
	ong.txPool = core.NewTxPool(config.TxPool, chainConfig, ong.blockchain)

	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit + cacheConfig.SnapshotLimit
	checkpoint := config.Checkpoint
	if checkpoint == nil {
		checkpoint = params.TrustedCheckpoints[genesisHash]
	}
	if ong.handler, err = newHandler(&handlerConfig{
		Database:   chainDb,
		Chain:      ong.blockchain,
		TxPool:     ong.txPool,
		Network:    config.NetworkId,
		Sync:       config.SyncMode,
		BloomCache: uint64(cacheLimit),
		EventMux:   ong.eventMux,
		Checkpoint: checkpoint,
		Whitelist:  config.Whitelist,
	}); err != nil {
		return nil, err
	}
	ong.miner = miner.New(ong, &config.Miner, chainConfig, ong.EventMux(), ong.engine, ong.isLocalBlock)
	ong.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	ong.APIBackend = &OngAPIBackend{stack.Config().ExtRPCEnabled(), stack.Config().AllowUnprotectedTxs, ong, nil}
	if ong.APIBackend.allowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.Miner.GasPrice
	}
	ong.APIBackend.gpo = gasprice.NewOracle(ong.APIBackend, gpoParams)

	ong.ongDialCandidates, err = setupDiscovery(ong.config.OngDiscoveryURLs)
	if err != nil {
		return nil, err
	}
	ong.snapDialCandidates, err = setupDiscovery(ong.config.SnapDiscoveryURLs)
	if err != nil {
		return nil, err
	}
	// Start the RPC service
	ong.netRPCService = ongapi.NewPublicNetAPI(ong.p2pServer, config.NetworkId)

	// Register the backend on the node
	stack.RegisterAPIs(ong.APIs())
	stack.RegisterProtocols(ong.Protocols())
	stack.RegisterLifecycle(ong)
	// Check for unclean shutdown
	if uncleanShutdowns, discards, err := rawdb.PushUncleanShutdownMarker(chainDb); err != nil {
		log.Error("Could not update unclean-shutdown-marker list", "error", err)
	} else {
		if discards > 0 {
			log.Warn("Old unclean shutdowns found", "count", discards)
		}
		for _, tstamp := range uncleanShutdowns {
			t := time.Unix(int64(tstamp), 0)
			log.Warn("Unclean shutdown detected", "booted", t,
				"age", common.PrettyAge(t))
		}
	}
	return ong, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"gong",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// APIs return the collection of RPC services the orange package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Orange) APIs() []rpc.API {
	apis := ongapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "ong",
			Version:   "1.0",
			Service:   NewPublicOrangeAPI(s),
			Public:    true,
		}, {
			Namespace: "ong",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "ong",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.handler.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "ong",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.APIBackend, false, 5*time.Minute),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Orange) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Orange) Orangerbase() (eb common.Address, err error) {
	s.lock.RLock()
	ongerbase := s.ongerbase
	s.lock.RUnlock()

	if ongerbase != (common.Address{}) {
		return ongerbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			ongerbase := accounts[0].Address

			s.lock.Lock()
			s.ongerbase = ongerbase
			s.lock.Unlock()

			log.Info("Orangerbase automatically configured", "address", ongerbase)
			return ongerbase, nil
		}
	}
	return common.Address{}, fmt.Errorf("ongerbase must be explicitly specified")
}

// isLocalBlock checks whonger the specified block is mined
// by local miner accounts.
//
// We regard two types of accounts as local miner account: ongerbase
// and accounts specified via `txpool.locals` flag.
func (s *Orange) isLocalBlock(block *types.Block) bool {
	author, err := s.engine.Author(block.Header())
	if err != nil {
		log.Warn("Failed to retrieve block author", "number", block.NumberU64(), "hash", block.Hash(), "err", err)
		return false
	}
	// Check whonger the given address is ongerbase.
	s.lock.RLock()
	ongerbase := s.ongerbase
	s.lock.RUnlock()
	if author == ongerbase {
		return true
	}
	// Check whonger the given address is specified by `txpool.local`
	// CLI flag.
	for _, account := range s.config.TxPool.Locals {
		if account == author {
			return true
		}
	}
	return false
}

// shouldPreserve checks whonger we should preserve the given block
// during the chain reorg depending on whonger the author of block
// is a local account.
func (s *Orange) shouldPreserve(block *types.Block) bool {
	// The reason we need to disable the self-reorg preserving for clique
	// is it can be probable to introduce a deadlock.
	//
	// e.g. If there are 7 available signers
	//
	// r1   A
	// r2     B
	// r3       C
	// r4         D
	// r5   A      [X] F G
	// r6    [X]
	//
	// In the round5, the inturn signer E is offline, so the worst case
	// is A, F and G sign the block of round5 and reject the block of opponents
	// and in the round6, the last available signer B is offline, the whole
	// network is stuck.
	if _, ok := s.engine.(*clique.Clique); ok {
		return false
	}
	return s.isLocalBlock(block)
}

// SetOrangerbase sets the mining reward address.
func (s *Orange) SetOrangerbase(ongerbase common.Address) {
	s.lock.Lock()
	s.ongerbase = ongerbase
	s.lock.Unlock()

	s.miner.SetOrangerbase(ongerbase)
}

// StartMining starts the miner with the given number of CPU threads. If mining
// is already running, this Method adjust the number of threads allowed to use
// and updates the minimum price required by the transaction pool.
func (s *Orange) StartMining(threads int) error {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := s.engine.(threaded); ok {
		log.Info("Updated mining threads", "threads", threads)
		if threads == 0 {
			threads = -1 // Disable the miner from within
		}
		th.SetThreads(threads)
	}
	// If the miner was not running, initialize it
	if !s.IsMining() {
		// Propagate the initial price point to the transaction pool
		s.lock.RLock()
		price := s.gasPrice
		s.lock.RUnlock()
		s.txPool.SetGasPrice(price)

		// Configure the local mining address
		eb, err := s.Orangerbase()
		if err != nil {
			log.Error("Cannot start mining without ongerbase", "err", err)
			return fmt.Errorf("ongerbase missing: %v", err)
		}
		if clique, ok := s.engine.(*clique.Clique); ok {
			wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
			if wallet == nil || err != nil {
				log.Error("Orangerbase account unavailable locally", "err", err)
				return fmt.Errorf("signer missing: %v", err)
			}
			clique.Authorize(eb, wallet.SignData)
		}
		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.handler.acceptTxs, 1)

		go s.miner.Start(eb)
	}
	return nil
}

// StopMining terminates the miner, both at the consensus engine level as well as
// at the block creation level.
func (s *Orange) StopMining() {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := s.engine.(threaded); ok {
		th.SetThreads(-1)
	}
	// Stop the block creating itself
	s.miner.Stop()
}

func (s *Orange) IsMining() bool      { return s.miner.Mining() }
func (s *Orange) Miner() *miner.Miner { return s.miner }

func (s *Orange) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *Orange) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *Orange) TxPool() *core.TxPool               { return s.txPool }
func (s *Orange) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Orange) Engine() consensus.Engine           { return s.engine }
func (s *Orange) ChainDb() ongdb.Database            { return s.chainDb }
func (s *Orange) IsListening() bool                  { return true } // Always listening
func (s *Orange) Downloader() *downloader.Downloader { return s.handler.downloader }
func (s *Orange) Synced() bool                       { return atomic.LoadUint32(&s.handler.acceptTxs) == 1 }
func (s *Orange) ArchiveMode() bool                  { return s.config.NoPruning }
func (s *Orange) BloomIndexer() *core.ChainIndexer   { return s.bloomIndexer }

// Protocols returns all the currently configured
// network protocols to start.
func (s *Orange) Protocols() []p2p.Protocol {
	protos := ong.MakeProtocols((*ongHandler)(s.handler), s.networkID, s.ongDialCandidates)
	if s.config.SnapshotCache > 0 {
		protos = append(protos, snap.MakeProtocols((*snapHandler)(s.handler), s.snapDialCandidates)...)
	}
	return protos
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Orange protocol implementation.
func (s *Orange) Start() error {
	ong.StartENRUpdater(s.blockchain, s.p2pServer.LocalNode())

	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(params.BloomBitsBlocks)

	// Figure out a max peers count based on the server limits
	maxPeers := s.p2pServer.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= s.p2pServer.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, s.p2pServer.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.handler.Start(maxPeers)
	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Orange protocol.
func (s *Orange) Stop() error {
	// Stop all the peer-related stuff first.
	s.handler.Stop()

	// Then stop everything else.
	s.bloomIndexer.Close()
	close(s.closeBloomHandler)
	s.txPool.Stop()
	s.miner.Stop()
	s.blockchain.Stop()
	s.engine.Close()
	rawdb.PopUncleanShutdownMarker(s.chainDb)
	s.chainDb.Close()
	s.eventMux.Stop()

	return nil
}
