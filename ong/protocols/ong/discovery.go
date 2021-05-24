// Copyright 2019 The go-orange Authors
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
	"github.com/ong2020/go-orange/core"
	"github.com/ong2020/go-orange/core/forkid"
	"github.com/ong2020/go-orange/p2p/enode"
	"github.com/ong2020/go-orange/rlp"
)

// enrEntry is the ENR entry which advertises `ong` protocol on the discovery.
type enrEntry struct {
	ForkID forkid.ID // Fork identifier per EIP-2124

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

// ENRKey implements enr.Entry.
func (e enrEntry) ENRKey() string {
	return "ong"
}

// StartENRUpdater starts the `ong` ENR updater loop, which listens for chain
// head events and updates the requested node record whenever a fork is passed.
func StartENRUpdater(chain *core.BlockChain, ln *enode.LocalNode) {
	var newHead = make(chan core.ChainHeadEvent, 10)
	sub := chain.SubscribeChainHeadEvent(newHead)

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-newHead:
				ln.Set(currentENREntry(chain))
			case <-sub.Err():
				// Would be nice to sync with Stop, but there is no
				// good way to do that.
				return
			}
		}
	}()
}

// currentENREntry constructs an `ong` ENR entry based on the current state of the chain.
func currentENREntry(chain *core.BlockChain) *enrEntry {
	return &enrEntry{
		ForkID: forkid.NewID(chain.Config(), chain.Genesis().Hash(), chain.CurrentHeader().Number.Uint64()),
	}
}
