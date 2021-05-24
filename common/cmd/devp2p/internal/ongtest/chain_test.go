// Copyright 2020 The go-orange Authors
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

package ongtest

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ong2020/go-orange/ong/protocols/ong"
	"github.com/ong2020/go-orange/p2p"
	"github.com/stretchr/testify/assert"
)

// TestOngProtocolNegotiation tests whonger the test suite
// can negotiate the highest ong protocol in a status message exchange
func TestOngProtocolNegotiation(t *testing.T) {
	var tests = []struct {
		conn     *Conn
		caps     []p2p.Cap
		expected uint32
	}{
		{
			conn: &Conn{},
			caps: []p2p.Cap{
				{Name: "ong", Version: 63},
				{Name: "ong", Version: 64},
				{Name: "ong", Version: 65},
			},
			expected: uint32(65),
		},
		{
			conn: &Conn{},
			caps: []p2p.Cap{
				{Name: "ong", Version: 0},
				{Name: "ong", Version: 89},
				{Name: "ong", Version: 65},
			},
			expected: uint32(65),
		},
		{
			conn: &Conn{},
			caps: []p2p.Cap{
				{Name: "ong", Version: 63},
				{Name: "ong", Version: 64},
				{Name: "wrongProto", Version: 65},
			},
			expected: uint32(64),
		},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tt.conn.negotiateOngProtocol(tt.caps)
			assert.Equal(t, tt.expected, uint32(tt.conn.ongProtocolVersion))
		})
	}
}

// TestChain_GetHeaders tests whonger the test suite can correctly
// respond to a GetBlockHeaders request from a node.
func TestChain_GetHeaders(t *testing.T) {
	chainFile, err := filepath.Abs("./testdata/chain.rlp")
	if err != nil {
		t.Fatal(err)
	}
	genesisFile, err := filepath.Abs("./testdata/genesis.json")
	if err != nil {
		t.Fatal(err)
	}

	chain, err := loadChain(chainFile, genesisFile)
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		req      GetBlockHeaders
		expected BlockHeaders
	}{
		{
			req: GetBlockHeaders{
				Origin: ong.HashOrNumber{
					Number: uint64(2),
				},
				Amount:  uint64(5),
				Skip:    1,
				Reverse: false,
			},
			expected: BlockHeaders{
				chain.blocks[2].Header(),
				chain.blocks[4].Header(),
				chain.blocks[6].Header(),
				chain.blocks[8].Header(),
				chain.blocks[10].Header(),
			},
		},
		{
			req: GetBlockHeaders{
				Origin: ong.HashOrNumber{
					Number: uint64(chain.Len() - 1),
				},
				Amount:  uint64(3),
				Skip:    0,
				Reverse: true,
			},
			expected: BlockHeaders{
				chain.blocks[chain.Len()-1].Header(),
				chain.blocks[chain.Len()-2].Header(),
				chain.blocks[chain.Len()-3].Header(),
			},
		},
		{
			req: GetBlockHeaders{
				Origin: ong.HashOrNumber{
					Hash: chain.Head().Hash(),
				},
				Amount:  uint64(1),
				Skip:    0,
				Reverse: false,
			},
			expected: BlockHeaders{
				chain.Head().Header(),
			},
		},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			headers, err := chain.GetHeaders(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, headers, tt.expected)
		})
	}
}
