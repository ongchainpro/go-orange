// Copyright 2017 The go-orange Authors
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

// Package ongconfig contains the configuration of the ONG and LES protocols.
package ongconfig

import (
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ong2020/go-orange/common"
	"github.com/ong2020/go-orange/consensus"
	"github.com/ong2020/go-orange/consensus/clique"
	"github.com/ong2020/go-orange/consensus/ongash"
	"github.com/ong2020/go-orange/core"
	"github.com/ong2020/go-orange/log"
	"github.com/ong2020/go-orange/miner"
	"github.com/ong2020/go-orange/node"
	"github.com/ong2020/go-orange/ong/downloader"
	"github.com/ong2020/go-orange/ong/gasprice"
	"github.com/ong2020/go-orange/ongdb"
	"github.com/ong2020/go-orange/params"
)

// FullNodeGPO contains default gasprice oracle settings for full node.
var FullNodeGPO = gasprice.Config{
	Blocks:     20,
	Percentile: 60,
	MaxPrice:   gasprice.DefaultMaxPrice,
}

// LightClientGPO contains default gasprice oracle settings for light client.
var LightClientGPO = gasprice.Config{
	Blocks:     2,
	Percentile: 60,
	MaxPrice:   gasprice.DefaultMaxPrice,
}

// Defaults contains default settings for use on the Orange main net.
var Defaults = Config{
	SyncMode: downloader.FastSync,
	Ongash: ongash.Config{
		CacheDir:         "ongash",
		CachesInMem:      2,
		CachesOnDisk:     3,
		CachesLockMmap:   false,
		DatasetsInMem:    1,
		DatasetsOnDisk:   2,
		DatasetsLockMmap: false,
	},
	NetworkId:               1,
	TxLookupLimit:           2350000,
	LightPeers:              100,
	UltraLightFraction:      75,
	DatabaseCache:           512,
	TrieCleanCache:          154,
	TrieCleanCacheJournal:   "triecache",
	TrieCleanCacheRejournal: 60 * time.Minute,
	TrieDirtyCache:          256,
	TrieTimeout:             60 * time.Minute,
	SnapshotCache:           102,
	Miner: miner.Config{
		GasFloor: 8000000,
		GasCeil:  8000000,
		GasPrice: big.NewInt(params.GWei),
		Recommit: 3 * time.Second,
	},
	TxPool:      core.DefaultTxPoolConfig,
	RPCGasCap:   25000000,
	GPO:         FullNodeGPO,
	RPCTxFeeCap: 1, // 1 onger
}

func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "darwin" {
		Defaults.Ongash.DatasetDir = filepath.Join(home, "Library", "Ongash")
	} else if runtime.GOOS == "windows" {
		localappdata := os.Getenv("LOCALAPPDATA")
		if localappdata != "" {
			Defaults.Ongash.DatasetDir = filepath.Join(localappdata, "Ongash")
		} else {
			Defaults.Ongash.DatasetDir = filepath.Join(home, "AppData", "Local", "Ongash")
		}
	} else {
		Defaults.Ongash.DatasetDir = filepath.Join(home, ".ongash")
	}
}

//go:generate gencodec -type Config -formats toml -out gen_config.go

// Config contains configuration options for of the ONG and LES protocols.
type Config struct {
	// The genesis block, which is inserted if the database is empty.
	// If nil, the Orange main net block is used.
	Genesis *core.Genesis `toml:",omitempty"`

	// Protocol options
	NetworkId uint64 // Network ID to use for selecting peers to connect to
	SyncMode  downloader.SyncMode

	// This can be set to list of enrtree:// URLs which will be queried for
	// for nodes to connect to.
	OngDiscoveryURLs  []string
	SnapDiscoveryURLs []string

	NoPruning  bool // Whonger to disable pruning and flush everything to disk
	NoPrefetch bool // Whonger to disable prefetching and only load state on demand

	TxLookupLimit uint64 `toml:",omitempty"` // The maximum number of blocks from head whose tx indices are reserved.

	// Whitelist of required block number -> hash values to accept
	Whitelist map[uint64]common.Hash `toml:"-"`

	// Light client options
	LightServ          int  `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightIngress       int  `toml:",omitempty"` // Incoming bandwidth limit for light servers
	LightEgress        int  `toml:",omitempty"` // Outgoing bandwidth limit for light servers
	LightPeers         int  `toml:",omitempty"` // Maximum number of LES client peers
	LightNoPrune       bool `toml:",omitempty"` // Whonger to disable light chain pruning
	LightNoSyncServe   bool `toml:",omitempty"` // Whonger to serve light clients before syncing
	SyncFromCheckpoint bool `toml:",omitempty"` // Whonger to sync the header chain from the configured checkpoint

	// Ultra Light client options
	UltraLightServers      []string `toml:",omitempty"` // List of trusted ultra light servers
	UltraLightFraction     int      `toml:",omitempty"` // Percentage of trusted servers to accept an announcement
	UltraLightOnlyAnnounce bool     `toml:",omitempty"` // Whonger to only announce headers, or also serve them

	// Database options
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int
	DatabaseFreezer    string

	TrieCleanCache          int
	TrieCleanCacheJournal   string        `toml:",omitempty"` // Disk journal directory for trie cache to survive node restarts
	TrieCleanCacheRejournal time.Duration `toml:",omitempty"` // Time interval to regenerate the journal for clean cache
	TrieDirtyCache          int
	TrieTimeout             time.Duration
	SnapshotCache           int
	Preimages               bool

	// Mining options
	Miner miner.Config

	// Ongash options
	Ongash ongash.Config

	// Transaction pool options
	TxPool core.TxPoolConfig

	// Gas Price Oracle options
	GPO gasprice.Config

	// Enables tracking of SHA3 preimages in the VM
	EnablePreimageRecording bool

	// Miscellaneous options
	DocRoot string `toml:"-"`

	// Type of the EWASM interpreter ("" for default)
	EWASMInterpreter string

	// Type of the EVM interpreter ("" for default)
	EVMInterpreter string

	// RPCGasCap is the global gas cap for ong-call variants.
	RPCGasCap uint64 `toml:",omitempty"`

	// RPCTxFeeCap is the global transaction fee(price * gaslimit) cap for
	// send-transction variants. The unit is onger.
	RPCTxFeeCap float64 `toml:",omitempty"`

	// Checkpoint is a hardcoded checkpoint which can be nil.
	Checkpoint *params.TrustedCheckpoint `toml:",omitempty"`

	// CheckpointOracle is the configuration for checkpoint oracle.
	CheckpointOracle *params.CheckpointOracleConfig `toml:",omitempty"`

	// Berlin block override (TODO: remove after the fork)
	OverrideBerlin *big.Int `toml:",omitempty"`
}

// CreateConsensusEngine creates a consensus engine for the given chain configuration.
func CreateConsensusEngine(stack *node.Node, chainConfig *params.ChainConfig, config *ongash.Config, notify []string, noverify bool, db ongdb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	if chainConfig.Clique != nil {
		return clique.New(chainConfig.Clique, db)
	}
	// Otherwise assume proof-of-work
	switch config.PowMode {
	case ongash.ModeFake:
		log.Warn("Ongash used in fake mode")
		return ongash.NewFaker()
	case ongash.ModeTest:
		log.Warn("Ongash used in test mode")
		return ongash.NewTester(nil, noverify)
	case ongash.ModeShared:
		log.Warn("Ongash used in shared mode")
		return ongash.NewShared()
	default:
		engine := ongash.New(ongash.Config{
			CacheDir:         stack.ResolvePath(config.CacheDir),
			CachesInMem:      config.CachesInMem,
			CachesOnDisk:     config.CachesOnDisk,
			CachesLockMmap:   config.CachesLockMmap,
			DatasetDir:       config.DatasetDir,
			DatasetsInMem:    config.DatasetsInMem,
			DatasetsOnDisk:   config.DatasetsOnDisk,
			DatasetsLockMmap: config.DatasetsLockMmap,
		}, notify, noverify)
		engine.SetThreads(-1) // Disable CPU mining
		return engine
	}
}
