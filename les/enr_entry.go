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

package les

import (
	"github.com/ong2020/go-orange/core/forkid"
	"github.com/ong2020/go-orange/p2p"
	"github.com/ong2020/go-orange/p2p/dnsdisc"
	"github.com/ong2020/go-orange/p2p/enode"
	"github.com/ong2020/go-orange/rlp"
)

// lesEntry is the "les" ENR entry. This is set for LES servers only.
type lesEntry struct {
	// Ignore additional fields (for forward compatibility).
	VfxVersion uint
	Rest       []rlp.RawValue `rlp:"tail"`
}

func (lesEntry) ENRKey() string { return "les" }

// ongEntry is the "ong" ENR entry. This is redeclared here to avoid depending on package ong.
type ongEntry struct {
	ForkID forkid.ID
	_      []rlp.RawValue `rlp:"tail"`
}

func (ongEntry) ENRKey() string { return "ong" }

// setupDiscovery creates the node discovery source for the ong protocol.
func (ong *LightOrange) setupDiscovery(cfg *p2p.Config) (enode.Iterator, error) {
	it := enode.NewFairMix(0)

	// Enable DNS discovery.
	if len(ong.config.OngDiscoveryURLs) != 0 {
		client := dnsdisc.NewClient(dnsdisc.Config{})
		dns, err := client.NewIterator(ong.config.OngDiscoveryURLs...)
		if err != nil {
			return nil, err
		}
		it.AddSource(dns)
	}

	// Enable DHT.
	if cfg.DiscoveryV5 && ong.p2pServer.DiscV5 != nil {
		it.AddSource(ong.p2pServer.DiscV5.RandomNodes())
	}

	forkFilter := forkid.NewFilter(ong.blockchain)
	iterator := enode.Filter(it, func(n *enode.Node) bool { return nodeIsServer(forkFilter, n) })
	return iterator, nil
}

// nodeIsServer checks whonger n is an LES server node.
func nodeIsServer(forkFilter forkid.Filter, n *enode.Node) bool {
	var les lesEntry
	var ong ongEntry
	return n.Load(&les) == nil && n.Load(&ong) == nil && forkFilter(ong.ForkID) == nil
}
