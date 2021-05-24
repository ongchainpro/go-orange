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

package runtime

import (
	"github.com/ong2020/go-orange/core/vm/runtime"
)

// Fuzz is the basic entry point for the go-fuzz tool
//
// This returns 1 for valid parsable/runable code, 0
// for invalid opcode.
func Fuzz(input []byte) int {
	_, _, err := runtime.Execute(input, input, &runtime.Config{
		GasLimit: 12000000,
	})
	// invalid opcode
	if err != nil && len(err.Error()) > 6 && err.Error()[:7] == "invalid" {
		return 0
	}
	return 1
}
