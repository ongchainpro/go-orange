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

// Package prometheus exposes go-metrics into a prometheus format.
package prometheus

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/ong2020/go-orange/log"
	"github.com/ong2020/go-orange/metrics"
)

// Handler returns an HTTP handler which dump metrics in prometheus format.
func Handler(reg metrics.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gather and pre-sort the metrics to avoid random listings
		var names []string
		reg.Each(func(name string, i interface{}) {
			names = append(names, name)
		})
		sort.Strings(names)

		// Aggregate all the metris into a prometheus collector
		c := newCollector()

		for _, name := range names {
			i := reg.Get(name)

			switch m := i.(type) {
			case metrics.Counter:
				c.addCounter(name, m.Snapshot())
			case metrics.Gauge:
				c.addGauge(name, m.Snapshot())
			case metrics.GaugeFloat64:
				c.addGaugeFloat64(name, m.Snapshot())
			case metrics.Histogram:
				c.addHistogram(name, m.Snapshot())
			case metrics.Meter:
				c.addMeter(name, m.Snapshot())
			case metrics.Timer:
				c.addTimer(name, m.Snapshot())
			case metrics.ResettingTimer:
				c.addResettingTimer(name, m.Snapshot())
			default:
				log.Warn("Unknown prometheus metric type", "type", fmt.Sprintf("%T", i))
			}
		}
		w.Header().Add("Content-Type", "text/plain")
		w.Header().Add("Content-Length", fmt.Sprint(c.buff.Len()))
		w.Write(c.buff.Bytes())
	})
}
