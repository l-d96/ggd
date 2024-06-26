package cmdline

import (
	// "github.com/azr4e1/ggd"

	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/azr4e1/ggd"
)

const (
	ErrorFlag = 1 + iota
	ErrorEncode
	ErrorDecode
)

const (
	PrintableMinASCII = 32
	PrintableMaxASCII = unicode.MaxASCII
	DefaultColumns    = 16
	DefaultGroups     = 2
	DefaultColor      = false
	DefaultPlain      = false
	DefaultDecode     = false
	DefaultCapitalize = false
	MaxLengthOffset   = 9
)

type option func(*cmdDumper) error

type cmdDumper struct {
	Input             io.Reader
	Output            io.Writer
	Columns           int
	Groups            int
	EncodingFormatter ggd.EncodingFormatter
	DecodingFormatter ggd.DecodingFormatter
	files             []io.Reader
}

func IsPrintableAscii(b byte) bool {
	return b < PrintableMaxASCII && b >= PrintableMinASCII
}

func SpacePadding(str string, maxLength int) string {
	if len(str) >= maxLength {
		return str
	}
	padding := strings.Repeat(" ", maxLength-len(str))
	return str + padding
}

func ZeroPadding(num int, maxLength int) string {
	sNum := strconv.Itoa(num)
	if len(sNum) >= maxLength {
		return sNum
	}
	padding := strings.Repeat("0", maxLength-len(sNum))
	return padding + sNum
}

func GroupHexes(groupLength int, hexes []ggd.HexByte) []string {
	groups := []string{}

	currGroup := ""
	for i, hb := range hexes {
		if i != 0 && i%groupLength == 0 {
			groups = append(groups, currGroup)
			currGroup = ""
		}
		currGroup += hb.String()
	}
	if currGroup != "" {
		groups = append(groups, currGroup)
	}
	return groups
}

func NewEncodingFormat(groupLength, maxLengthHex, maxLengthOffset int, color bool, capitalize bool) (ggd.EncodingFormatter, error) {
	if groupLength <= 0 {
		return nil, errors.New("invalid number of groups")
	}
	if maxLengthHex <= 0 {
		return nil, errors.New("invalid max length of hex sequence")
	}
	if maxLengthOffset <= 0 {
		return nil, errors.New("invalid max length of offset")
	}

	return func(hx ggd.HexEncoding) string {
		normalizedInput := []byte{}
		for _, b := range hx.Input {
			if !IsPrintableAscii(b) {
				normalizedInput = append(normalizedInput, '.')
				continue
			}
			normalizedInput = append(normalizedInput, b)
		}

		hexCodes := SpacePadding(strings.Join(GroupHexes(groupLength, hx.HexCodes), " "), maxLengthHex)
		offset := ZeroPadding(hx.Offset, maxLengthOffset)

		normalizedInputStr := string(normalizedInput)
		if capitalize {
			hexCodes = strings.ToUpper(hexCodes)
		}
		if color {
			hexCodes = hexCodesStyle.Render(hexCodes)
			offset = offsetStyle.Render(offset)
			normalizedInputStr = inputStyle.Render(normalizedInputStr)
		}
		return fmt.Sprintf("%s    | %s |    %s", offset, hexCodes, normalizedInputStr)
	}, nil
}

func NewDecodingFormat(capitalize bool) ggd.DecodingFormatter {

	return func(s string) ([]ggd.HexByte, error) {
		splits := strings.Split(s, "|")
		if len(splits) < 3 {
			return nil, errors.New("wrong format used. Try with the '-plain' flag")
		}
		hexString := strings.Join(strings.Fields(splits[1]), "")
		if capitalize {
			hexString = strings.ToLower(hexString)
		}

		decodedHex, err := ggd.DefaultDecFormatter(hexString)
		return decodedHex, err
	}
}

func NewCmdEncoder(opts ...option) (*cmdDumper, error) {
	cmdD := &cmdDumper{
		Input:             os.Stdin,
		Output:            os.Stdout,
		Columns:           16,
		Groups:            2,
		EncodingFormatter: ggd.DefaultEncFormatter,
		DecodingFormatter: ggd.DefaultDecFormatter,
	}

	for _, o := range opts {
		err := o(cmdD)
		if err != nil {
			return &cmdDumper{}, err
		}
	}

	return cmdD, nil
}

func (cd cmdDumper) Format(hx []ggd.HexEncoding) []string {
	formatted := []string{}
	for _, h := range hx {
		formatted = append(formatted, cd.EncodingFormatter(h))
	}

	return formatted
}

func WithInput(r io.Reader) option {
	return func(cd *cmdDumper) error {
		cd.Input = r
		return nil
	}
}

func WithOutput(w io.Writer) option {
	return func(cd *cmdDumper) error {
		cd.Output = w
		return nil
	}
}

func WithColumns(c int) option {
	return func(cd *cmdDumper) error {
		if c <= 0 {
			return errors.New("invalid number of columns")
		}
		cd.Columns = c
		return nil
	}
}

func WithGroups(g int) option {
	return func(cd *cmdDumper) error {
		if g <= 0 {
			return errors.New("invalid number of groups")
		}
		cd.Groups = g
		return nil
	}
}

func WithEncFormat(f ggd.EncodingFormatter) option {
	return func(cd *cmdDumper) error {
		cd.EncodingFormatter = f
		return nil
	}
}

func WithDecFormat(f ggd.DecodingFormatter) option {
	return func(cd *cmdDumper) error {
		cd.DecodingFormatter = f
		return nil
	}
}

func WithInputFromArgs(args []string) option {
	return func(cd *cmdDumper) error {
		if len(args) < 1 {
			return nil
		}
		cd.files = make([]io.Reader, len(args))
		for i, path := range args {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			cd.files[i] = f
		}
		cd.Input = io.MultiReader(cd.files...)
		return nil
	}
}

func (cd *cmdDumper) Encode() error {
	for _, f := range cd.files {
		defer f.(io.Closer).Close()
	}

	dumper, err := ggd.NewEncoder(
		ggd.EncoderChunkSize(cd.Columns),
		ggd.EncoderInput(cd.Input),
		ggd.EncoderOutput(cd.Output),
		ggd.EncoderFormatter(cd.EncodingFormatter))
	if err != nil {
		return err
	}
	err = dumper.Encode()
	if err != nil {
		return err
	}

	return nil
}

func (cd *cmdDumper) Decode() error {
	for _, f := range cd.files {
		defer f.(io.Closer).Close()
	}

	dumper, err := ggd.NewDecoder(
		ggd.DecoderInput(cd.Input),
		ggd.DecoderOutput(cd.Output),
		ggd.DecoderFormatter(cd.DecodingFormatter),
	)
	if err != nil {
		return err
	}

	err = dumper.Decode()
	if err != nil {
		return err
	}

	return nil
}

func Main() int {
	flag.Usage = func() {
		fmt.Printf("Usage: %s [options] [files...]\n\n", os.Args[0])
		fmt.Print("Turn input data from stdin or files into hexadecimal representation.\n\n")
		fmt.Println("Options:")
		fmt.Println("  -d --decode\n\tdecode hex dump")
		fmt.Println("  -g --groups int\n\tnumber of hex codes in a single group")
		fmt.Println("  -c --columns int\n\tnumber of hex codes in a single line")
		fmt.Println("  -r --no-color\n\tcolored output")
		fmt.Println("  -p --plain\n\tplain output")
		fmt.Println("  -C --capitalize\n\tcapitalized hex codes")
		fmt.Println("  -o --output string\n\toutput file")
		fmt.Println("  -h --help\n\tshow this help and exit")
	}

	var decode, noColor, capitalize, plain bool
	var groups, columns int
	var outputName string

	flag.BoolVar(&decode, "decode", DefaultDecode, "decode hex dump")
	flag.BoolVar(&decode, "d", DefaultDecode, "decode hex dump")
	flag.IntVar(&groups, "groups", DefaultGroups, "number of hex codes in a single group")
	flag.IntVar(&groups, "g", DefaultGroups, "number of hex codes in a single group")
	flag.IntVar(&columns, "columns", DefaultColumns, "number of hex codes in a single line")
	flag.IntVar(&columns, "c", DefaultColumns, "number of hex codes in a single line")
	flag.BoolVar(&noColor, "no-color", DefaultColor, "colored output")
	flag.BoolVar(&noColor, "r", DefaultColor, "colored output")
	flag.BoolVar(&plain, "plain", DefaultPlain, "plain output")
	flag.BoolVar(&plain, "p", DefaultPlain, "plain output")
	flag.BoolVar(&capitalize, "capitalize", DefaultCapitalize, "capitalized hex codes")
	flag.BoolVar(&capitalize, "C", DefaultCapitalize, "capitalized hex codes")
	flag.StringVar(&outputName, "output", "", "output file")
	flag.StringVar(&outputName, "o", "", "output file")
	flag.Parse()

	if groups <= 0 {
		fmt.Fprintln(os.Stderr, "invalid number of groups")
		return ErrorFlag
	}
	maxLength := columns*2 + columns/groups - 1
	if columns%groups != 0 {
		maxLength++
	}

	var output io.Writer = os.Stdout
	if outputName != "" {
		outputFile, err := os.Create(outputName)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ErrorFlag
		}
		defer outputFile.Close()

		output = outputFile
		noColor = true
	}

	encFormatter := func(capitalize bool) ggd.EncodingFormatter {
		return func(hx ggd.HexEncoding) string {
			str := ggd.DefaultEncFormatter(hx)
			if capitalize {
				str = strings.ToUpper(str)
			}
			return str
		}
	}(capitalize)

	decFormatter := func(capitalize bool) ggd.DecodingFormatter {
		return func(s string) ([]ggd.HexByte, error) {
			if capitalize {
				s = strings.ToLower(s)
			}
			return ggd.DefaultDecFormatter(s)
		}
	}(capitalize)

	if !plain {
		var err error
		encFormatter, err = NewEncodingFormat(groups, maxLength, MaxLengthOffset, !noColor, capitalize)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ErrorFlag
		}

		decFormatter = NewDecodingFormat(capitalize)
	}

	dumper, err := NewCmdEncoder(
		WithColumns(columns),
		WithGroups(groups),
		WithInputFromArgs(flag.Args()),
		WithOutput(output),
		WithEncFormat(encFormatter),
		WithDecFormat(decFormatter),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ErrorFlag
	}

	if !decode {
		err := dumper.Encode()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ErrorEncode
		}
	} else {
		err := dumper.Decode()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ErrorDecode
		}
	}

	return 0
}
