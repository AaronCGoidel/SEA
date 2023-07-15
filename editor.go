package main

import (
	"bufio"
	"bytes"
	"editor/syntax"
	"editor/terminal_ctl"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/ssh/terminal"
)

const NAME = "SEA"
const VERSION = "0.2.2"
const AUTHOR = "Aaron Goidel"
const EMAIL = "acgoidel@gmail.com"

// values for special keys
const (
	KEY_QUIT      = 0x11
	KEY_SAVE      = 0x13
	KEY_BACKSPACE = 0x7F
	KEY_NEW_LINE  = 0x0D
	KEY_LEFT      = 1000 + iota
	KEY_RIGHT     = 1000 + iota
	KEY_UP        = 1000 + iota
	KEY_DOWN      = 1000 + iota
	KEY_PG_UP     = 1000 + iota
	KEY_PG_DOWN   = 1000 + iota
	KEY_DEL       = 1000 + iota
)

const (
	T_BOLD   byte = 1
	T_RED    byte = 31
	T_GREEN  byte = 32
	T_YELLOW byte = 33
	T_BLUE   byte = 34
	T_PURPLE byte = 35
	T_CYAN   byte = 36
	T_OFF    byte = 0
	T_INVERT byte = 7
)

// highlighting values
const (
	H_NONE    byte = T_OFF
	H_NUM     byte = T_BLUE
	H_MATCH   byte = T_INVERT
	H_STR     byte = T_GREEN
	H_COMMENT      = T_CYAN
	H_KEY          = T_PURPLE
	H_KEY_ALT      = T_YELLOW
)

type line_t struct {
	text      []byte
	highlight []byte
	len       uint
}

type vector struct {
	x uint
	y uint
}

type editor_state struct {
	default_term_state *terminal.State

	file_name string
	new_file  bool

	msg         string
	msg_time    time.Time
	msg_timeout time.Duration

	dim    vector
	offset vector
	cursor vector

	used_rows uint
	lines     []line_t

	clean          bool
	quit_attempted bool

	language syntax.Syntax
}

var editor editor_state

type buf struct {
	buffer []byte
	len    uint
}

// Have the editor exit more gracefully when an error occurs
func kill(msg string, err error) {
	terminal_ctl.Disable_Raw(editor.default_term_state)
	// clear terminal before printing error message
	io.WriteString(os.Stdout, "\x1b[2J")
	io.WriteString(os.Stdout, "\x1b[H")

	log.Fatalf("%s: %s\n", msg, err)
	os.Exit(1)
}

// Adds a string to a buffer
func add_to_buffer(b *buf, str string) {
	len := uint(len(str))

	b.buffer = append(b.buffer, str...)
	b.len += len
}

func del_from_buffer(b *buf, loc int) {
	front := b.buffer[:loc]
	back := b.buffer[loc+1:]
	b.buffer = append(front, back...)
	b.len--
}

// Logic for controlling the cursor
func move_cursor(key uint) {
	var l *line_t
	switch key {
	case KEY_UP:
		// only move up if cursor is not on first line
		if editor.cursor.y > 0 {
			editor.cursor.y--
		}
	case KEY_DOWN:
		// only move down if we are above the first unused line
		if editor.cursor.y+1 < editor.used_rows {
			editor.cursor.y++
		}
	case KEY_LEFT:
		// move left if this is not the beginning of the line
		// or, if there is a line above, move to the end of it
		if editor.cursor.x > 0 {
			editor.cursor.x--
		} else if editor.cursor.y > 0 {
			editor.cursor.y--
			editor.cursor.x = editor.lines[editor.cursor.y].len
		}
	case KEY_RIGHT:
		// only move right if this is not the end of a line
		// or if there is a line below to move to
		if editor.cursor.y < editor.used_rows {
			l = &editor.lines[editor.cursor.y]
		} else {
			l = nil
		}
		if l != nil {
			if editor.cursor.x < l.len {
				editor.cursor.x++
			} else if editor.cursor.x == l.len {
				if editor.cursor.y+1 < editor.used_rows {
					editor.cursor.x = 0
					editor.cursor.y++
				}
			}
		}
	}
	var length uint = 0
	if editor.cursor.y < editor.used_rows {
		length = editor.lines[editor.cursor.y].len
	}

	if editor.cursor.x > length {
		editor.cursor.x = length
	}
}

// Update which row and column to start displaying at
// so that the editor scrolls
func scroll() {
	if editor.cursor.x < editor.offset.x {
		editor.offset.x = editor.cursor.x
	}
	if editor.cursor.x >= editor.offset.x+editor.dim.x {
		editor.offset.x = editor.cursor.x - editor.dim.x + 1
	}
	if editor.cursor.y < editor.offset.y {
		editor.offset.y = editor.cursor.y
	}
	if editor.cursor.y >= editor.dim.y+editor.offset.y {
		editor.offset.y = editor.cursor.y - editor.dim.y + 1
	}
}

var delimiters []byte = []byte(",.()+-/*=~%<>[]; \t\n\r")

func is_delimiter(char byte) bool {
	if bytes.IndexByte(delimiters, char) < 0 {
		return false
	}
	return true
}

func highlight_line(line *line_t) {
	line.highlight = make([]byte, line.len)

	new_word := true
	skip := 0
	var string_char byte = 0
	for i, char := range line.text {
		if skip > 0 {
			skip--
			continue
		}

		if string_char == 0 {
			in_line_comment := editor.language.In_line_comment
			if bytes.HasPrefix(line.text[i:], in_line_comment) {
				for j := 0; j < len(line.text[i:]); j++ {
					line.highlight[j+i] = H_COMMENT
				}
				break
			}
		}

		if string_char != 0 {
			line.highlight[i] = H_STR
			if char == '\\' && i+1 < int(line.len) {
				line.highlight[i+1] = H_STR
				skip++
				continue
			}
			if char == string_char {
				string_char = 0
			}
			continue
		} else {
			if char == '\'' || char == '"' {
				string_char = char
				line.highlight[i] = H_STR
				continue
			}
		}

		prev := H_NONE
		if i > 0 {
			prev = line.highlight[i-1]
		}
		if bytes.HasPrefix(line.text[i:], []byte("0x")) && new_word {
			is_hex := true
			var j int
			for j = i + 2; j < int(line.len); j++ {
				if is_delimiter(line.text[j]) {
					break
				} else if !strings.Contains("abcdef1234567890", string(line.text[j])) {
					is_hex = false
					break
				}
			}
			if is_hex {
				for j > i {
					line.highlight[j-1] = H_NUM
					j--
				}
			}
		}
		if (unicode.IsDigit(rune(char)) && (new_word || prev == H_NUM)) || (char == '.' && prev == H_NUM) {
			line.highlight[i] = H_NUM
			new_word = false
		}

		if new_word {
			for _, keyword := range editor.language.Keywords {
				kw_type := H_KEY
				kw_len := len(keyword)
				if keyword[kw_len-1:] == "|" {
					keyword = keyword[:kw_len-1]
					kw_type = H_KEY_ALT
					kw_len--
				}
				if len(line.text[i:]) <= kw_len-1 {
					continue
				}
				if string(line.text[i:i+kw_len]) == keyword {
					if len(line.text[i:]) == kw_len || is_delimiter(line.text[i+kw_len]) {
						for j := 0; j < kw_len; j++ {
							line.highlight[i+j] = kw_type
						}
						i += kw_len
						break
					}
				}
				new_word = false
			}
		}
		new_word = is_delimiter(char)
	}
}

// Set the status message and the time that it was set
func set_message(args ...interface{}) {
	editor.msg = fmt.Sprintf(args[0].(string), args[1:]...)
	editor.msg_time = time.Now()
}

// Mark file as modified and display status for how to save
func modified() {
	editor.clean = false
	editor.quit_attempted = false
	set_message("CTRL-S to save")
}

// Add a new line to the editor with the given content at the given location
func add_line(loc uint, line []byte) {
	if loc > editor.used_rows {
		return
	}

	var row line_t
	row.text = line
	row.len = uint(len(line))

	if loc == editor.used_rows {
		editor.lines = append(editor.lines, row)
	} else {
		temp := make([]line_t, 1)
		temp[0] = row
		if loc == 0 {
			editor.lines = append(temp, editor.lines...)
		} else {
			back := append(temp, editor.lines[loc:]...)
			editor.lines = append(editor.lines[:loc], back...)
		}
	}
	editor.used_rows++
	highlight_line(&editor.lines[loc])
	modified()
}

// Convert the current contents of the editor to a single buffer for saving
func stringify(b *buf) {
	var len uint
	var text []byte

	for _, line := range editor.lines {
		len += line.len + 1
		text = append(text, line.text...)
		text = append(text, "\n"...)
	}
	b.buffer = text
	b.len = len
}

func prompt(text string, callback func(*buf, uint)) string {
	var in_buf buf = buf{}

	for {
		set_message(text, in_buf.buffer)
		refresh_terminal()

		char := read_input()

		if char == KEY_DEL || char == KEY_BACKSPACE || char == 0x08 {
			if in_buf.len > 0 {
				del_from_buffer(&in_buf, len(in_buf.buffer)-1)
			}
		} else if char == '\r' {
			if in_buf.len != 0 {
				set_message("")
				if callback != nil {
					callback(&in_buf, char)
				}
				return string(in_buf.buffer)
			}
		} else if char == '\x1b' {
			set_message("")
			if callback != nil {
				callback(&in_buf, char)
			}
			return ""
		} else {
			if unicode.IsPrint(rune(char)) {
				add_to_buffer(&in_buf, string(char))
			}
		}
		if callback != nil {
			callback(&in_buf, char)
		}
	}
}

// Handles file saving logic
// Checks if there is a filename, opens file, calls strigify and writes to file
func save() {
	// is there a current filename
	if editor.file_name == "" {
		editor.file_name = prompt("Save as: %q", nil)
		if editor.file_name == "" {
			set_message("Did not save")
			return
		}
		editor.language = syntax.Setup_syntax(editor.file_name)
		for i := 0; i < len(editor.lines); i++ {
			highlight_line(&editor.lines[i])
		}
	}
	b := buf{}
	stringify(&b) // get a buffer of the entire editor state

	f, err := os.OpenFile(editor.file_name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, err = f.Write(b.buffer)
	if err != nil {
		set_message("Unable to save file. Error: %s", err)
	} else {
		set_message("File saved. %d bytes written", b.len)
		editor.clean = true
	}
}

// Logic for handling the insertion of a character into the editor
func insert(c byte) {
	if editor.cursor.y == editor.used_rows {
		var new_row []byte
		add_line(editor.used_rows, new_row)
	}

	line := &editor.lines[editor.cursor.y]
	loc := editor.cursor.x

	if loc > line.len {
		line.text = append(line.text, c)
	} else if loc == 0 {
		temp := make([]byte, line.len+1)
		temp[0] = c
		copy(temp[1:], line.text)
		line.text = temp
	} else {
		back := append([]byte{c}, line.text[loc:]...)
		line.text = append(line.text[:loc], back...)
	}
	line.len++
	editor.cursor.x++
	highlight_line(line)
	modified()
}

func new_line() {
	if editor.cursor.x == 0 {
		empty_line := make([]byte, 0)
		add_line(editor.cursor.y, empty_line)
	} else {
		cur_line := editor.lines[editor.cursor.y]
		back := cur_line.text[editor.cursor.x:]
		add_line(editor.cursor.y+1, back)

		front := cur_line.text[:editor.cursor.x]
		editor.lines[editor.cursor.y].text = front
		editor.lines[editor.cursor.y].len = uint(len(front))
	}
	editor.cursor.y++
	editor.cursor.x = 0
}

// Logic for deleting a character out of the editor
func del() {
	if editor.cursor.y == editor.used_rows {
		return
	}
	if editor.cursor.x == 0 && editor.cursor.y == 0 {
		return
	}

	line := &editor.lines[editor.cursor.y]
	if editor.cursor.x > 0 {
		loc := editor.cursor.x - 1
		if loc >= line.len {
			return
		}
		line.text = append(line.text[:loc], line.text[loc+1:]...)
		line.len--
		editor.cursor.x--

		highlight_line(line)

	} else {
		editor.cursor.x = editor.lines[editor.cursor.y-1].len

		above := &editor.lines[editor.cursor.y-1]
		above.text = append(above.text, line.text...)
		above.len = uint(len(above.text))

		loc := editor.cursor.y
		if loc > editor.used_rows {
			return
		}
		editor.lines = append(editor.lines[:loc], editor.lines[loc+1:]...)
		editor.used_rows--
		editor.cursor.y--

		highlight_line(above)
	}
	modified()
}

func open_file(file_name string) {
	editor.file_name = file_name
	fd, err := os.Open(file_name)

	editor.language = syntax.Setup_syntax(file_name)

	if err != nil {
		if os.IsNotExist(err) {
			editor.new_file = true
			return
		} else {
			kill("Couldn't open file: ", err)
		}
	}
	defer fd.Close()

	f := bufio.NewReader(fd)
	for line, err := f.ReadBytes('\n'); err == nil; line, err = f.ReadBytes('\n') {
		for char := line[len(line)-1]; len(line) > 0 && (char == '\n' || char == '\r'); {
			line = line[:len(line)-1]
			if len(line) > 0 {
				char = line[len(line)-1]
			}
		}
		add_line(editor.used_rows, line)
	}
	if err != nil && err != io.EOF {
		kill("Error while reading file: ", err)
	}

	editor.clean = true
}

func print_status(b *buf) {
	add_to_buffer(b, "\x1b[7m")

	file_name := editor.file_name
	if file_name != "" {
		file_name += " "
	}

	mod := ""
	if editor.new_file {
		mod = "[New File]"
	} else if !editor.clean {
		mod = "(modified)"
	}

	msg := fmt.Sprintf("%.20s%s", file_name, mod)
	msg_len := uint(len(msg))
	if msg_len > editor.dim.x {
		msg_len = editor.dim.x
	}
	add_to_buffer(b, msg[:msg_len])

	y := editor.cursor.y + 1
	if editor.used_rows == 0 {
		y = 0
	}

	x := editor.cursor.x + 1

	loc_msg := fmt.Sprintf("row: %d, col: %d", y, x)
	loc_msg_len := uint(len(loc_msg))

	for msg_len < editor.dim.x {
		if editor.dim.x-msg_len == loc_msg_len {
			add_to_buffer(b, loc_msg)
			break
		} else {
			add_to_buffer(b, " ")
			msg_len++
		}
	}

	add_to_buffer(b, "\x1b[m")
	add_to_buffer(b, "\r\n")
}

func print_message(b *buf) {
	add_to_buffer(b, "\x1b[K")

	len := uint(len(editor.msg))
	if len > editor.dim.x {
		len = editor.dim.x
	}
	elapsed := (time.Now().Sub(editor.msg_time))
	if len > 0 && elapsed < editor.msg_timeout {
		add_to_buffer(b, editor.msg)
	}

}

func center_msg(b *buf, msg string, print_len uint) {
	msg_len := uint(len(msg))
	if msg_len > editor.dim.x {
		msg_len = editor.dim.x
	}

	padding := (editor.dim.x - print_len) / 2
	if padding > 0 {
		add_to_buffer(b, "~")
		padding--
	}
	for padding > 0 {
		add_to_buffer(b, " ")
		padding--
	}
	add_to_buffer(b, msg[:msg_len])
}

func print_welcome(b *buf) {
	var msg_len uint

	name := fmt.Sprintf("\x1b[36m"+"%s"+"\x1b[m", NAME)
	version := fmt.Sprintf("\x1b[32m"+"v%s"+"\x1b[m", VERSION)
	title := fmt.Sprintf("%s --- %s", name, version)
	msg_len = uint(len(NAME+VERSION) + 5)
	center_msg(b, title, msg_len)

	add_to_buffer(b, "\x1b[K")
	add_to_buffer(b, "\r\n~\r\n")

	author := fmt.Sprintf("\x1b[36m"+"%s"+"\x1b[m", AUTHOR)
	email := fmt.Sprintf("\x1b[32m"+"<%s>"+"\x1b[m", EMAIL)
	byline := fmt.Sprintf("Created by: %s %s", author, email)
	msg_len = uint(len(AUTHOR+EMAIL) + 13)
	center_msg(b, byline, msg_len)
}

func draw_rows(b *buf) {
	var start_row uint
	for start_row = 0; start_row < editor.dim.y; start_row++ {
		row := start_row + editor.offset.y
		if row >= editor.used_rows {
			if editor.used_rows == 0 && start_row == editor.dim.y/4 {
				print_welcome(b)
				start_row += 2
			} else {
				add_to_buffer(b, "~")
			}
		} else {
			var len uint
			if editor.offset.x >= editor.lines[row].len {
				len = 0
			} else {
				len = editor.lines[row].len - editor.offset.x
				if len > editor.dim.x {
					len = editor.dim.x
				}
				cutoff := len + editor.offset.x
				line_highlights := editor.lines[row].highlight[editor.offset.x:cutoff]
				current_highlight := H_NONE

				for i, char := range editor.lines[row].text[editor.offset.x:cutoff] {
					if editor.language.Is_highlighted {
						color := line_highlights[i]

						if current_highlight != color {
							current_highlight = color
							esc := fmt.Sprintf("\x1b[%dm", color)
							add_to_buffer(b, esc)
						}
					}
					add_to_buffer(b, string(char))
				}
				add_to_buffer(b, "\x1b[m")
			}
		}
		add_to_buffer(b, "\x1b[K")
		add_to_buffer(b, "\r\n")
	}
}

func refresh_terminal() {
	scroll()

	var b buf = buf{}

	add_to_buffer(&b, "\x1b[?25l")
	add_to_buffer(&b, "\x1b[H")

	draw_rows(&b)
	print_status(&b)
	print_message(&b)

	cursor := fmt.Sprintf("\x1b[%d;%dH",
		editor.cursor.y-editor.offset.y+1,
		editor.cursor.x-editor.offset.x+1)
	add_to_buffer(&b, cursor)
	add_to_buffer(&b, "\x1b[?25h")

	_, err := os.Stdout.Write(b.buffer)

	if err != nil {
		kill("Couldn't refresh screen", err)
	}
}

func read_input() uint {
	var c [1]byte
	var in int

	var err error

	for in, err = os.Stdin.Read(c[:]); in != 1; in, err = os.Stdin.Read(c[:]) {
	}
	if err != nil {
		kill("Couldn't read from terminal", err)
	}
	if c[0] == '\x1b' {
		var sequence [2]byte

		if in, err = os.Stdin.Read(sequence[:]); in != 2 {
			return '\x1b'
		}

		if sequence[0] == 0x5B {
			if sequence[1] >= 0x30 && sequence[1] < 0x39 {
				if in, err = os.Stdin.Read(c[:]); in != 1 {
					return '\x1b'
				}
				if c[0] == 0x7E {
					switch sequence[1] {
					case 0x33:
						return KEY_DEL
					case 0x35:
						return KEY_PG_UP
					case 0x36:
						return KEY_PG_DOWN
					}
				}
			}

			switch sequence[1] {
			case 0x41:
				return KEY_UP
			case 0x42:
				return KEY_DOWN
			case 0x43:
				return KEY_RIGHT
			case 0x44:
				return KEY_LEFT
			}
		}
		return '\x1b'
	}
	return uint(c[0])
}

func handle_key_event() {
	c := read_input()

	switch c {
	case KEY_QUIT:
		if editor.clean || editor.quit_attempted {
			io.WriteString(os.Stdout, "\x1b[2J")
			io.WriteString(os.Stdout, "\x1b[H")
			terminal_ctl.Disable_Raw(editor.default_term_state)
			os.Exit(0)
		} else if !editor.clean {
			set_message("There are unsaved changes, press CTRL-Q again to force quit.")
			editor.quit_attempted = true
		}
	case KEY_SAVE:
		save()
	case KEY_NEW_LINE:
		new_line()
	case KEY_DEL:
		move_cursor(KEY_RIGHT)
		del()
	case KEY_BACKSPACE, 0x08: // ctrl-h
		del()

	case KEY_UP, KEY_DOWN, KEY_LEFT, KEY_RIGHT:
		move_cursor(c)

	case KEY_PG_UP, KEY_PG_DOWN:
		{
			var dir uint
			if c == KEY_PG_UP {
				dir = KEY_UP
			} else {
				dir = KEY_DOWN
			}
			for i := editor.dim.y; i > 0; i-- {
				move_cursor(dir)
			}
		}

	case '\x1b':
		break

	default:
		insert(byte(c))
	}
}

func setup() {
	editor.clean = true

	editor.dim.x, editor.dim.y = terminal_ctl.Size()
	editor.dim.y -= 2

	editor.msg_timeout = time.Second * 5
}
func main() {
	editor.default_term_state = terminal_ctl.Enable_Raw()
	defer terminal_ctl.Disable_Raw(editor.default_term_state)
	setup()

	if len(os.Args) > 1 {
		open_file(os.Args[1])
	} else {
		editor.new_file = true
	}

	set_message("CTRL-Q to quit")

	for {
		refresh_terminal()
		handle_key_event()
	}
}
