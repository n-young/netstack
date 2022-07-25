package util

import (
	"fmt"
	"os"
	"os/exec"
)

func InitREPL(prompt string, readyChan chan bool) chan string {
	strChan := make(chan string)

	// disable input buffering
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run()
	// do not display entered characters on the screen
	exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
	// Print prompt
	fmt.Printf("\033[2K\r%v", prompt)

	go func() {
		history := make([]string, 0)
		histcopy := make([]string, 0)
		histcopy = append(histcopy, "")
		b := make([]byte, 1)
		var backidx int = 0
		var idx int = 0
		for {
			curstr := histcopy[idx]
			os.Stdin.Read(b)
			if string(b[0]) == "\x1b" { // ESC character
				os.Stdin.Read(b)
				if string(b[0]) == "[" {
					os.Stdin.Read(b)
					switch string(b[0]) {
					case "C": // ^[[C i.e. right arrow
						if backidx > 0 {
							backidx--
						}
					case "B": // ^[[B i.e. down arrow
						if idx < len(histcopy)-1 {
							idx++
							backidx = 0
							curstr = histcopy[idx]
						}
					case "D": // ^[[D i.e. left arrow
						if backidx < len(curstr) {
							backidx++
						}
					case "A": // ^[[A i.e. up arrow
						if idx > 0 {
							idx--
							backidx = 0
							curstr = histcopy[idx]
						}
					case "3":
						os.Stdin.Read(b)
						if string(b[0]) == "~" { // Delete key
							if backidx > 0 {
								endstr := curstr[len(curstr)-backidx+1:]
								curstr = curstr[0:len(curstr)-backidx] + endstr
								backidx--
							}
						}
					}
				}
			} else if isPrintableChar(b[0]) {
				endstr := curstr[len(curstr)-backidx:]
				startstr := curstr[0 : len(curstr)-backidx]
				curstr = startstr + string(b[0]) + endstr
			} else {
				// Check for special characters
				switch string(b[0]) {
				case "\x7f": // DEL character
					if backidx < len(curstr) {
						endstr := curstr[len(curstr)-backidx:]
						curstr = curstr[0:len(curstr)-backidx-1] + endstr
					}
				case "\x04": // Ctrl-d
					if backidx > 0 {
						endstr := curstr[len(curstr)-backidx+1:]
						curstr = curstr[0:len(curstr)-backidx] + endstr
						backidx--
					}
				case "\n", "\r": // Newline
					fmt.Printf("\n")
					// Reset the REPL
					if len(curstr) > 0 {
						// Send off the line to the channel
						strChan <- curstr
						<-readyChan
						history = append(history, curstr)
						histcopy[len(histcopy)-1] = curstr
						backidx = 0
						curstr = ""
						histcopy[idx] = history[idx]
						histcopy = append(histcopy, "")
					}
					idx = len(histcopy) - 1
				}
			}
			fmt.Printf("\033[2K\r%v%v", prompt, curstr)
			for i := 0; i < backidx; i++ {
				fmt.Printf("\b")
			}
			histcopy[idx] = curstr
		}
	}()

	return strChan
}

func CloseRepl() {
	// reenable displaying characters on the screen
	exec.Command("stty", "-F", "/dev/tty", "echo").Run()
}

// isPrintableChar returns if b represents a printable ascii char based on
// https://www.asciitable.com/
func isPrintableChar(b byte) bool {
	val := int(b)
	return int(' ') <= val && val <= int('~')
}
