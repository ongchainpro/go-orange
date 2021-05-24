// Copyright 2016 The go-orange Authors
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

// Package les implements the Light Orange Subprotocol.
package les

import (
	"fmt"
	"time"

	"github.com/ong2020/go-orange/accounts"
	"github.com/ong2020/go-orange/common"
	"github.com/ong2020/go-orange/common/hexutil"
	"github.com/ong2020/go-orange/common/mclock"
	"github.com/ong2020/go-orange/consensus"
	"github.com/ong2020/go-orange/core"
	"github.com/ong2020/go-orange/core/bloombits"
	"github.com/ong2020/go-orange/core/rawdb"
	"github.com/ong2020/go-orange/core/types"
	"github.com/ong2020/go-orange/event"
	"github.com/ong2020/go-orange/internal/ongapi"
	"github.com/ong2020/go-orange/les/vflux"
	vfc "github.com/ong2020/go-orange/les/vflux/client"
	"github.com/ong2020/go-orange/light"
	"github.com/ong2020/go-orange/log"
	"github.com/ong2020/go-orange/node"
	"github.com/ong2020/go-orange/ong/downloader"
	"github.com/ong2020/go-orange/ong/filters"
	"github.com/ong2020/go-orange/ong/gasprice"
	"github.com/ong2020/go-orange/ong/ongconfig"
	"github.com/ong2020/go-orange/p2p"
	"github.com/ong2020/go-orange/p2p/enode"
	"github.com/ong2020/go-orange/p2p/enr"
	"github.com/ong2020/go-orange/params"
	"github.com/ong2020/go-orange/rlp"
	"github.com/ong2020/go-orange/rpc"
)

type LightOrange struct {
	lesCommons

	peers              *serverPeerSet
	reqDist            *requestDistributor
	retriever          *retrieveManager
	odr                *LesOdr
	relay              *lesTxRelay
	handler            *clientHandler
	txPool             *light.TxPool
	blockchain         *light.LightChain
	serverPool         *vfc.ServerPool
	serverPoolIterator enode.Iterator
	pruner             *pruner

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	ApiBackend     *LesApiBackend
	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager
	netRPCService  *ongapi.PublicNetAPI

	p2pServer *p2p.Server
	p2pConfig *p2p.Config
}

// New creates an instance of the light client.
func New(stack *node.Node, config *ongconfig.Config) (*LightOrange, error) {
	chainDb, err := stack.OpenDatabase("lightchaindata", config.DatabaseCache, config.DatabaseHandles, "ong/db/chaindata/")
	if err != nil {
		return nil, err
	}
	lesDb, err := stack.OpenDatabase("les.client", 0, 0, "ong/db/lesclient/")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlockWithOverride(chainDb, config.Genesis, config.OverrideBerlin)
	if _, isCompat := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !isCompat {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	peers := newServerPeerSet()
	long := &LightOrange{
		lesCommons: lesCommons{
			genesis:     genesisHash,
			config:      config,
			chainConfig: chainConfig,
			iConfig:     light.DefaultClientIndexerConfig,
			chainDb:     chainDb,
			lesDb:       lesDb,
			closeCh:     make(chan struct{}),
		},
		peers:          peers,
		eventMux:       stack.EventMux(),
		reqDist:        newRequestDistributor(peers, &mclock.System{}),
		accountManager: stack.AccountManager(),
		engine:         ongconfig.CreateConsensusEngine(stack, chainConfig, &config.Ongash, nil, false, chainDb),
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   core.NewBloomIndexer(chainDb, params.BloomBitsBlocksClient, params.HelperTrieConfirmations),
		p2pServer:      stack.Server(),
		p2pConfig:      &stack.Config().P2P,
	}

	var prenegQuery vfc.QueryFunc
	if long.p2pServer.DiscV5 != nil {
		prenegQuery = long.prenegQuery
	}
	long.serverPool, long.serverPoolIterator = vfc.NewServerPool(lesDb, []byte("serverpool:"), time.Second, prenegQuery, &mclock.System{}, config.UltraLightServers, requestList)
	long.serverPool.AddMetrics(suggestedTimeoutGauge, totalValueGauge, serverSelectableGauge, serverConnectedGauge, sessionValueMeter, serverDialedMeter)

	long.retriever = newRetrieveManager(peers, long.reqDist, long.serverPool.GetTimeout)
	long.relay = newLesTxRelay(peers, long.retriever)

	long.odr = NewLesOdr(chainDb, light.DefaultClientIndexerConfig, long.peers, long.retriever)
	long.chtIndexer = light.NewChtIndexer(chainDb, long.odr, params.CHTFrequency, params.HelperTrieConfirmations, config.LightNoPrune)
	long.bloomTrieIndexer = light.NewBloomTrieIndexer(chainDb, long.odr, params.BloomBitsBlocksClient, params.BloomTrieFrequency, config.LightNoPrune)
	long.odr.SetIndexers(long.chtIndexer, long.bloomTrieIndexer, long.bloomIndexer)

	checkpoint := config.Checkpoint
	if checkpoint == nil {
		checkpoint = params.TrustedCheckpoints[genesisHash]
	}
	// Note: NewLightChain adds the trusted checkpoint so it needs an ODR with
	// indexers already set but not started yet
	if long.blockchain, err = light.NewLightChain(long.odr, long.chainConfig, long.engine, checkpoint); err != nil {
		return nil, err
	}
	long.chainReader = long.blockchain
	long.txPool = light.NewTxPool(long.chainConfig, long.blockchain, long.relay)

	// Set up checkpoint oracle.
	long.oracle = long.setupOracle(stack, genesisHash, config)

	// Note: AddChildIndexer starts the update process for the child
	long.bloomIndexer.AddChildIndexer(long.bloomTrieIndexer)
	long.chtIndexer.Start(long.blockchain)
	long.bloomIndexer.Start(long.blockchain)

	// Start a light chain pruner to delete useless historical data.
	long.pruner = newPruner(chainDb, long.chtIndexer, long.bloomTrieIndexer)

	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		long.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	long.ApiBackend = &LesApiBackend{stack.Config().ExtRPCEnabled(), stack.Config().AllowUnprotectedTxs, long, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.Miner.GasPrice
	}
	long.ApiBackend.gpo = gasprice.NewOracle(long.ApiBackend, gpoParams)

	long.handler = newClientHandler(config.UltraLightServers, config.UltraLightFraction, checkpoint, long)
	if long.handler.ulc != nil {
		log.Warn("Ultra light client is enabled", "trustedNodes", len(long.handler.ulc.keys), "minTrustedFraction", long.handler.ulc.fraction)
		long.blockchain.DisableCheckFreq()
	}

	long.netRPCService = ongapi.NewPublicNetAPI(long.p2pServer, long.config.NetworkId)

	// Register the backend on the node
	stack.RegisterAPIs(long.APIs())
	stack.RegisterProtocols(long.Protocols())
	stack.RegisterLifecycle(long)

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
	return long, nil
}

// VfluxRequest sends a batch of requests to the given node through discv5 UDP TalkRequest and returns the responses
func (s *LightOrange) VfluxRequest(n *enode.Node, reqs vflux.Requests) vflux.Replies {
	if s.p2pServer.DiscV5 == nil {
		return nil
	}
	reqsEnc, _ := rlp.EncodeToBytes(&reqs)
	repliesEnc, _ := s.p2pServer.DiscV5.TalkRequest(s.serverPool.DialNode(n), "vfx", reqsEnc)
	var replies vflux.Replies
	if len(repliesEnc) == 0 || rlp.DecodeBytes(repliesEnc, &replies) != nil {
		return nil
	}
	return replies
}

// vfxVersion returns the version number of the "les" service subdomain of the vflux UDP
// service, as advertised in the ENR record
func (s *LightOrange) vfxVersion(n *enode.Node) uint {
	if n.Seq() == 0 {
		var err error
		if s.p2pServer.DiscV5 == nil {
			return 0
		}
		if n, err = s.p2pServer.DiscV5.RequestENR(n); n != nil && err == nil && n.Seq() != 0 {
			s.serverPool.Persist(n)
		} else {
			return 0
		}
	}

	var les []rlp.RawValue
	if err := n.Load(enr.WithEntry("les", &les)); err != nil || len(les) < 1 {
		return 0
	}
	var version uint
	rlp.DecodeBytes(les[0], &version) // Ignore additional fields (for forward compatibility).
	return version
}

// prenegQuery sends a capacity query to the given server node to determine whonger
// a connection slot is immediately available
func (s *LightOrange) prenegQuery(n *enode.Node) int {
	if s.vfxVersion(n) < 1 {
		// UDP query not supported, always try TCP connection
		return 1
	}

	var requests vflux.Requests
	requests.Add("les", vflux.CapacityQueryName, vflux.CapacityQueryReq{
		Bias:      180,
		AddTokens: []vflux.IntOrInf{{}},
	})
	replies := s.VfluxRequest(n, requests)
	var cqr vflux.CapacityQueryReply
	if replies.Get(0, &cqr) != nil || len(cqr) != 1 { // Note: Get returns an error if replies is nil
		return -1
	}
	if cqr[0] > 0 {
		return 1
	}
	return 0
}

type LightDummyAPI struct{}

// Orangerbase is the address that mining rewards will be send to
func (s *LightDummyAPI) Orangerbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("mining is not supported in light mode")
}

// Coinbase is the address that mining rewards will be send to (alias for Orangerbase)
func (s *LightDummyAPI) Coinbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("mining is not supported in light mode")
}

// Hashrate returns the POW hashrate
func (s *LightDummyAPI) Hashrate() hexutil.Uint {
	return 0
}

// Mining returns an indication if this node is currently mining.
func (s *LightDummyAPI) Mining() bool {
	return false
}

// APIs returns the collection of RPC services the orange package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *LightOrange) APIs() []rpc.API {
	apis := ongapi.GetAPIs(s.ApiBackend)
	apis = append(apis, s.engine.APIs(s.BlockChain().HeaderChain())...)
	return append(apis, []rpc.API{
		{
			Namespace: "ong",
			Version:   "1.0",
			Service:   &LightDummyAPI{},
			Public:    true,
		}, {
			Namespace: "ong",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.handler.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "ong",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, true, 5*time.Minute),
			Public:    true,
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		}, {
			Namespace: "les",
			Version:   "1.0",
			Service:   NewPrivateLightAPI(&s.lesCommons),
			Public:    false,
		}, {
			Namespace: "vflux",
			Version:   "1.0",
			Service:   s.serverPool.API(),
			Public:    false,
		},
	}...)
}

func (s *LightOrange) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *LightOrange) BlockChain() *light.LightChain      { return s.blockchain }
func (s *LightOrange) TxPool() *light.TxPool              { return s.txPool }
func (s *LightOrange) Engine() consensus.Engine           { return s.engine }
func (s *LightOrange) LesVersion() int                    { return int(ClientProtocolVersions[0]) }
func (s *LightOrange) Downloader() *downloader.Downloader { return s.handler.downloader }
func (s *LightOrange) EventMux() *event.TypeMux           { return s.eventMux }

// Protocols returns all the currently configured network protocols to start.
func (s *LightOrange) Protocols() []p2p.Protocol {
	return s.makeProtocols(ClientProtocolVersions, s.handler.runPeer, func(id enode.ID) interface{} {
		if p := s.peers.peer(id.String()); p != nil {
			return p.Info()
		}
		return nil
	}, s.serverPoolIterator)
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// light orange protocol implementation.
func (s *LightOrange) Start() error {
	log.Warn("Light client mode is an experimental feature")

	discovery, err := s.setupDiscovery(s.p2pConfig)
	if err != nil {
		return err
	}
	s.serverPool.AddSource(discovery)
	s.serverPool.Start()
	// Start bloom request workers.
	s.wg.Add(bloomServiceThreads)
	s.startBloomHandlers(params.BloomBitsBlocksClient)
	s.handler.start()

	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Orange protocol.
func (s *LightOrange) Stop() error {
	close(s.closeCh)
	s.serverPool.Stop()
	s.peers.close()
	s.reqDist.close()
	s.odr.Stop()
	s.relay.Stop()
	s.bloomIndexer.Close()
	s.chtIndexer.Close()
	s.blockchain.Stop()
	s.handler.stop()
	s.txPool.Stop()
	s.engine.Close()
	s.pruner.close()
	s.eventMux.Stop()
	rawdb.PopUncleanShutdownMarker(s.chainDb)
	s.chainDb.Close()
	s.lesDb.Close()
	s.wg.Wait()
	log.Info("Light orange stopped")
	return nil
}
