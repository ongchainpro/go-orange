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

// Contains the metrics collected by the downloader.

package downloader

import (
	"github.com/ong2020/go-orange/metrics"
)

var (
	headerInMeter      = metrics.NewRegisteredMeter("ong/downloader/headers/in", nil)
	headerReqTimer     = metrics.NewRegisteredTimer("ong/downloader/headers/req", nil)
	headerDropMeter    = metrics.NewRegisteredMeter("ong/downloader/headers/drop", nil)
	headerTimeoutMeter = metrics.NewRegisteredMeter("ong/downloader/headers/timeout", nil)

	bodyInMeter      = metrics.NewRegisteredMeter("ong/downloader/bodies/in", nil)
	bodyReqTimer     = metrics.NewRegisteredTimer("ong/downloader/bodies/req", nil)
	bodyDropMeter    = metrics.NewRegisteredMeter("ong/downloader/bodies/drop", nil)
	bodyTimeoutMeter = metrics.NewRegisteredMeter("ong/downloader/bodies/timeout", nil)

	receiptInMeter      = metrics.NewRegisteredMeter("ong/downloader/receipts/in", nil)
	receiptReqTimer     = metrics.NewRegisteredTimer("ong/downloader/receipts/req", nil)
	receiptDropMeter    = metrics.NewRegisteredMeter("ong/downloader/receipts/drop", nil)
	receiptTimeoutMeter = metrics.NewRegisteredMeter("ong/downloader/receipts/timeout", nil)

	stateInMeter   = metrics.NewRegisteredMeter("ong/downloader/states/in", nil)
	stateDropMeter = metrics.NewRegisteredMeter("ong/downloader/states/drop", nil)

	throttleCounter = metrics.NewRegisteredCounter("ong/downloader/throttle", nil)
)
