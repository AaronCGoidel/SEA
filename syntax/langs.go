package syntax

import "path/filepath"

type Syntax struct {
	Is_highlighted  bool
	In_line_comment []byte
	// start_block_comment []byte
	// end_block_comment   []byte
	Keywords []string
}

var syntax Syntax

const (
	py     = ".py"
	golang = ".go"
	c      = ".c"
	txt    = ".txt"
)

func Setup_syntax(file_name string) Syntax {
	ext := filepath.Ext(file_name)

	syntax.Is_highlighted = true

	switch ext {
	case py:
		syntax.In_line_comment = []byte("#")
		syntax.Keywords = []string{"False|", "None|", "True|", "and|", "as", "assert", "break", "class|",
			"continue", "def|", "del", "elif", "else", "except", "finally", "for", "from", "global|",
			"if", "import", "in|", "is|", "lambda|", "nonlocal|", "not|", "or|", "pass", "raise", "return",
			"try", "while", "with", "yield"}
	case c:
		syntax.In_line_comment = []byte("//")
		syntax.Keywords = []string{"switch", "if", "while", "for", "break", "continue", "return", "else",
			"struct", "union", "typedef", "static", "enum", "class", "case",
			"int|", "long|", "double|", "float|", "char|", "unsigned|", "signed|",
			"void|"}
	case golang:
		syntax.In_line_comment = []byte("//")
		syntax.Keywords = []string{"break", "case", "chan", "const",
			"continue", "default", "defer", "else", "fallthrough", "for", "func",
			"go", "goto", "if", "import", "interface", "map", "package", "range",
			"return", "select", "struct", "switch", "type", "var",
			"bool|", "string|", "int|", "int8|", "int16|", "int32|", "int64|",
			"uint|", "uint8|", "uint16|", "uint32|", "uint64|", "byte|", "rune|",
			"float32|", "float64|", "complex64|", "complex128|", "uintptr|"}
	default:
		syntax.Is_highlighted = false
	}

	return syntax
}
