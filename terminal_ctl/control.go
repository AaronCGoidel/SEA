package terminal_ctl

import (
	"io"
	"log"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh/terminal"
)

var STDOUT_FD uintptr = os.Stdout.Fd()

type Window struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// Sets the terminal to its original state
func Disable_Raw(default_state *terminal.State) {
	terminal.Restore(0, default_state)
}

// Sets the terminal to raw mode
// Stores the original state of the terminal for later reversion
func Enable_Raw() *terminal.State {
	default_state, err := terminal.MakeRaw(0)

	if err != nil {
		panic(err)
	}

	return default_state
}

// Makes syscall to get dimensions of the terminal
func get_window_size() (uint, uint) {
	var win Window
	winptr := uintptr(unsafe.Pointer(&win))
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, STDOUT_FD, syscall.TIOCGWINSZ, winptr)

	if err != 0 {
		io.WriteString(os.Stdout, "\x1b[2J")
		io.WriteString(os.Stdout, "\x1b[H")
		log.Fatalf("Couldn't get window size: %s\n", err)
	}
	y := uint(win.Row)
	x := uint(win.Col)
	return x, y
}

func Size() (uint, uint) {
	return get_window_size()
}
