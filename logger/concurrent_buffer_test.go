package logger

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentLogBufferRetainsIndependentEvents(t *testing.T) {
	logBufferMu.Lock()
	logBuffer = nil
	logBufferMu.Unlock()
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			for i := 0; i < 100; i++ {
				addToBuffer("INFO", fmt.Sprintf("buffer-event-%d-%d", id, i))
				_ = GetLogs(30, "debug")
			}
		}(worker)
	}
	workers.Wait()
	logs := GetLogs(maxLogBufferSize, "debug")
	if len(logs) != 800 {
		t.Fatalf("concurrent log events lost: %d", len(logs))
	}
	seen := map[string]bool{}
	for _, entry := range logs {
		marker := entry[strings.Index(entry, "buffer-event-"):]
		if seen[marker] {
			t.Fatal("concurrent log entry duplicated")
		}
		seen[marker] = true
	}
}
