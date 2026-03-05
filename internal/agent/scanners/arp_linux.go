//go:build linux

package scanners

import "os"

func openProcNetARP() (*os.File, error) {
	return os.Open("/proc/net/arp")
}
