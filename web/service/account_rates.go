package service

import (
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v2/xray"
)

type AccountRate struct {
	Up   float64 `json:"up"`
	Down float64 `json:"down"`
	At   int64   `json:"at"`
}

type rateSample struct {
	at    time.Time
	rates map[string]AccountRate
}

var accountRates = struct {
	sync.Mutex
	sources map[int]rateSample
}{sources: make(map[int]rateSample)}

func recordAccountRates(source int, at time.Time, rates map[string]AccountRate) {
	accountRates.Lock()
	defer accountRates.Unlock()
	if sample, ok := accountRates.sources[source]; !ok || !at.Before(sample.at) {
		accountRates.sources[source] = rateSample{at, rates}
	}
}

func ObserveLocalAccountRates(records []*xray.ClientTraffic, at time.Time, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	rates := make(map[string]AccountRate, min(len(records), 1024))
	for _, item := range records {
		if item.Up == 0 && item.Down == 0 && !item.Online {
			continue
		}
		key := accountKey(item.Email)
		if key == "" {
			continue
		}
		rate := rates[key]
		rate.Up += float64(max(item.Up, 0)) / interval.Seconds()
		rate.Down += float64(max(item.Down, 0)) / interval.Seconds()
		rate.At = at.UnixMilli()
		rates[key] = rate
	}
	recordAccountRates(0, at, rates)
}

func CurrentAccountRates() map[string]AccountRate {
	now := time.Now()
	accountRates.Lock()
	defer accountRates.Unlock()
	size := 0
	for _, sample := range accountRates.sources {
		if now.Sub(sample.at) <= 25*time.Second {
			size += len(sample.rates)
		}
	}
	out := make(map[string]AccountRate, size)
	for source, sample := range accountRates.sources {
		if now.Sub(sample.at) > 25*time.Second {
			delete(accountRates.sources, source)
			continue
		}
		for key, rate := range sample.rates {
			prev := out[key]
			out[key] = AccountRate{Up: prev.Up + rate.Up, Down: prev.Down + rate.Down, At: max(prev.At, rate.At)}
		}
	}
	return out
}
