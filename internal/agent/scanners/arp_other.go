//go:build !linux

package scanners

import (
	"fmt"
	"os"
)

func openProcNetARP() (*os.File, error) {
	return nil, fmt.Errorf("/proc/net/arp not available on this platform")
}
