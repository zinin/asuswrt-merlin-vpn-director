//go:build linux

package bot

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func bindToDeviceControl(iface string) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var sockErr error
		if err := c.Control(func(fd uintptr) {
			sockErr = unix.BindToDevice(int(fd), iface)
		}); err != nil {
			return err
		}
		return sockErr
	}
}
