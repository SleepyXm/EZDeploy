package pretty

import (
	"fmt"
	"os"
	"strings"
)

func Print(args ...any) {
	write(fmt.Sprint(args...))
}

func Println(args ...any) {
	write(fmt.Sprintln(args...))
}

func Printf(format string, args ...any) {
	write(fmt.Sprintf(format, args...))
}

func Clear() {
	write("\033[H\033[2J\r")
	_ = os.Stdout.Sync()
}

func write(s string) {
	// Prevent double carriage returns if something already used CRLF.
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Terminal needs CRLF, otherwise cursor moves down but keeps current column.
	s = strings.ReplaceAll(s, "\n", "\r\n")

	fmt.Print(s)
}
