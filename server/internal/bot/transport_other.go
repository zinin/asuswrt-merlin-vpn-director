//go:build !linux

package bot

import (
	"fmt"
	"syscall"
)

func bindToDeviceControl(iface string) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return fmt.Errorf("SO_BINDTODEVICE is not supported on this platform (iface %q)", iface)
	}
}
