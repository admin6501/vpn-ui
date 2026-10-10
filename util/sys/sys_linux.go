//go:build linux
// +build linux

package sys

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
)

var SIGUSR1 = syscall.SIGUSR1

// TCP counts established sockets only. Listening, closing and TIME_WAIT
// sockets are not live connections. UDP counts sockets, not client sessions.
func GetTCPCount() (int, error) { return getSocketCount("tcp", true) }
func GetUDPCount() (int, error) { return getSocketCount("udp", false) }

func getSocketCount(protocol string, establishedOnly bool) (int, error) {
	total := 0
	for _, suffix := range []string{"", "6"} {
		file, err := os.Open(fmt.Sprintf("%s/net/%s%s", HostProc(), protocol, suffix))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		count, err := countSocketRows(file, establishedOnly)
		file.Close()
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func countSocketRows(reader io.Reader, establishedOnly bool) (int, error) {
	scanner := bufio.NewScanner(reader)
	total := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[0] == "sl" {
			continue
		}
		if !strings.HasSuffix(fields[0], ":") {
			continue
		}
		if _, err := strconv.ParseUint(strings.TrimSuffix(fields[0], ":"), 10, 64); err != nil {
			continue
		}
		if _, err := strconv.ParseUint(fields[3], 16, 8); err != nil {
			continue
		}
		if establishedOnly && fields[3] != "01" {
			continue
		}
		total++
	}
	return total, scanner.Err()
}

// --- CPU Utilization (Linux native) ---

// CPUTimesRaw returns the cumulative-since-boot idle and total CPU jiffies from
// /proc/stat. Utilization is the ratio of their DELTAS between two reads, and the
// deltas are deliberately left to the caller: these counters used to be turned into
// a percentage here against a package-level baseline, which made the reading depend
// on who called last. The dashboard polls every 2s and the Telegram bot's usage
// report calls the same path on its own schedule, so each was silently consuming the
// other's interval and reporting a percentage measured over a window it did not own.
// Per-caller state cannot have that problem.
func CPUTimesRaw() (idleAll, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	line, err := rd.ReadString('\n')
	if err != nil && err != io.EOF {
		return 0, 0, err
	}
	// Expect line like: cpu  user nice system idle iowait irq softirq steal guest guest_nice
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("unexpected /proc/stat format")
	}

	var nums []uint64
	for i := 1; i < len(fields); i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			break
		}
		nums = append(nums, v)
	}
	if len(nums) < 4 { // need at least user,nice,system,idle
		return 0, 0, fmt.Errorf("insufficient cpu fields")
	}

	// Conform with standard Linux CPU accounting
	var user, nice, system, idle, iowait, irq, softirq, steal uint64
	user = nums[0]
	if len(nums) > 1 {
		nice = nums[1]
	}
	if len(nums) > 2 {
		system = nums[2]
	}
	if len(nums) > 3 {
		idle = nums[3]
	}
	if len(nums) > 4 {
		iowait = nums[4]
	}
	if len(nums) > 5 {
		irq = nums[5]
	}
	if len(nums) > 6 {
		softirq = nums[6]
	}
	if len(nums) > 7 {
		steal = nums[7]
	}

	idleAll = idle + iowait
	nonIdle := user + nice + system + irq + softirq + steal
	return idleAll, idleAll + nonIdle, nil
}
