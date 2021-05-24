// Copyright 2015 The go-orange Authors
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

package ong

import (
	"context"
	"errors"
	"math/big"

	"github.com/ong2020/go-orange/accounts"
	"github.com/ong2020/go-orange/common"
	"github.com/ong2020/go-orange/consensus"
	"github.com/ong2020/go-orange/core"
	"github.com/ong2020/go-orange/core/bloombits"
	"github.com/ong2020/go-orange/core/rawdb"
	"github.com/ong2020/go-orange/core/state"
	"github.com/ong2020/go-orange/core/types"
	"github.com/ong2020/go-orange/core/vm"
	"github.com/ong2020/go-orange/event"
	"github.com/ong2020/go-orange/miner"
	"github.com/ong2020/go-orange/ong/downloader"
	"github.com/ong2020/go-orange/ong/gasprice"
	"github.com/ong2020/go-orange/ongdb"
	"github.com/ong2020/go-orange/params"
	"github.com/ong2020/go-orange/rpc"
)

// OngAPIBackend implements ongapi.Backend for full nodes
type OngAPIBackend struct {
	extRPCEnabled       bool
	allowUnprotectedTxs bool
	ong                 *Orange
	gpo                 *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *OngAPIBackend) ChainConfig() *params.ChainConfig {
	return b.ong.blockchain.Config()
}

func (b *OngAPIBackend) CurrentBlock() *types.Block {
	return b.ong.blockchain.CurrentBlock()
}

func (b *OngAPIBackend) SetHead(number uint64) {
	b.ong.handler.downloader.Cancel()
	b.ong.blockchain.SetHead(number)
}

func (b *OngAPIBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.ong.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.ong.blockchain.CurrentBlock().Header(), nil
	}
	return b.ong.blockchain.GetHeaderByNumber(uint64(number)), nil
}

func (b *OngAPIBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.ong.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.ong.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *OngAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.ong.blockchain.GetHeaderByHash(hash), nil
}

func (b *OngAPIBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.ong.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.ong.blockchain.CurrentBlock(), nil
	}
	return b.ong.blockchain.GetBlockByNumber(uint64(number)), nil
}

func (b *OngAPIBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.ong.blockchain.GetBlockByHash(hash), nil
}

func (b *OngAPIBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.ong.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.ong.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.ong.blockchain.GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *OngAPIBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		block, state := b.ong.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.ong.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *OngAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.ong.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.ong.BlockChain().StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *OngAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return b.ong.blockchain.GetReceiptsByHash(hash), nil
}

func (b *OngAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	receipts := b.ong.blockchain.GetReceiptsByHash(hash)
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *OngAPIBackend) GetTd(ctx context.Context, hash common.Hash) *big.Int {
	return b.ong.blockchain.GetTdByHash(hash)
}

func (b *OngAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header) (*vm.EVM, func() error, error) {
	vmError := func() error { return nil }

	txContext := core.NewEVMTxContext(msg)
	context := core.NewEVMBlockContext(header, b.ong.BlockChain(), nil)
	return vm.NewEVM(context, txContext, state, b.ong.blockchain.Config(), *b.ong.blockchain.GetVMConfig()), vmError, nil
}

func (b *OngAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.ong.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *OngAPIBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.ong.miner.SubscribePendingLogs(ch)
}

func (b *OngAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.ong.BlockChain().SubscribeChainEvent(ch)
}

func (b *OngAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.ong.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *OngAPIBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.ong.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *OngAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.ong.BlockChain().SubscribeLogsEvent(ch)
}

func (b *OngAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.ong.txPool.AddLocal(signedTx)
}

func (b *OngAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.ong.txPool.Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *OngAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.ong.txPool.Get(hash)
}

func (b *OngAPIBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.ong.ChainDb(), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *OngAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.ong.txPool.Nonce(addr), nil
}

func (b *OngAPIBackend) Stats() (pending int, queued int) {
	return b.ong.txPool.Stats()
}

func (b *OngAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.ong.TxPool().Content()
}

func (b *OngAPIBackend) TxPool() *core.TxPool {
	return b.ong.TxPool()
}

func (b *OngAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.ong.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *OngAPIBackend) Downloader() *downloader.Downloader {
	return b.ong.Downloader()
}

func (b *OngAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *OngAPIBackend) ChainDb() ongdb.Database {
	return b.ong.ChainDb()
}

func (b *OngAPIBackend) EventMux() *event.TypeMux {
	return b.ong.EventMux()
}

func (b *OngAPIBackend) AccountManager() *accounts.Manager {
	return b.ong.AccountManager()
}

func (b *OngAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *OngAPIBackend) UnprotectedAllowed() bool {
	return b.allowUnprotectedTxs
}

func (b *OngAPIBackend) RPCGasCap() uint64 {
	return b.ong.config.RPCGasCap
}

func (b *OngAPIBackend) RPCTxFeeCap() float64 {
	return b.ong.config.RPCTxFeeCap
}

func (b *OngAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.ong.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *OngAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.ong.bloomRequests)
	}
}

func (b *OngAPIBackend) Engine() consensus.Engine {
	return b.ong.engine
}

func (b *OngAPIBackend) CurrentHeader() *types.Header {
	return b.ong.blockchain.CurrentHeader()
}

func (b *OngAPIBackend) Miner() *miner.Miner {
	return b.ong.Miner()
}

func (b *OngAPIBackend) StartMining(threads int) error {
	return b.ong.StartMining(threads)
}

func (b *OngAPIBackend) StateAtBlock(ctx context.Context, block *types.Block, reexec uint64) (*state.StateDB, func(), error) {
	return b.ong.stateAtBlock(block, reexec)
}

func (b *OngAPIBackend) StatesInRange(ctx context.Context, fromBlock *types.Block, toBlock *types.Block, reexec uint64) ([]*state.StateDB, func(), error) {
	return b.ong.statesInRange(fromBlock, toBlock, reexec)
}

func (b *OngAPIBackend) StateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, *state.StateDB, func(), error) {
	return b.ong.stateAtTransaction(block, txIndex, reexec)
}
