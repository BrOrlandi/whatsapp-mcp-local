package mediaread

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

// maxZipBytes caps what is inflated from one DOCX or XLSX, against zip bombs.
var maxZipBytes int64 = 256 << 20

// maxZipOpens caps the parts opened from one document.
const maxZipOpens = 1000

const maxCellChars = 500

// officeZip is a DOCX or XLSX package, read within a budget of inflated bytes.
type officeZip struct {
	r     *zip.ReadCloser
	files map[string]*zip.File
	left  int64
	opens int
}

func openZip(name string) (*officeZip, error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, fmt.Errorf("opening the document: %w", err)
	}
	z := &officeZip{r: r, files: make(map[string]*zip.File, len(r.File)), left: maxZipBytes}
	for _, f := range r.File {
		z.files[strings.TrimPrefix(f.Name, "/")] = f
	}
	return z, nil
}

func (z *officeZip) Close() error { return z.r.Close() }

func (z *officeZip) errTooLarge() error {
	return fmt.Errorf("the document expands to more than %d MiB; it is too large or malformed", maxZipBytes>>20)
}

func (z *officeZip) open(name string) (io.ReadCloser, error) {
	f := z.files[name]
	if f == nil {
		return nil, fmt.Errorf("the document has no %s: %w", name, fs.ErrNotExist)
	}
	if z.opens++; z.opens > maxZipOpens {
		return nil, errors.New("the document has too many parts to read")
	}
	if f.UncompressedSize64 > uint64(z.left) {
		return nil, z.errTooLarge()
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s in the document: %w", name, err)
	}
	return &budgetReader{rc: rc, z: z}, nil
}

// budgetReader fails once the archive's budget is spent, whatever sizes the
// archive declares.
type budgetReader struct {
	rc io.ReadCloser
	z  *officeZip
}

func (b *budgetReader) Read(p []byte) (int, error) {
	if int64(len(p)) > b.z.left+1 {
		p = p[:b.z.left+1]
	}
	n, err := b.rc.Read(p)
	b.z.left -= int64(n)
	if b.z.left < 0 {
		return 0, b.z.errTooLarge()
	}
	return n, err
}

func (b *budgetReader) Close() error { return b.rc.Close() }

func (z *officeZip) decode(name string, v any) error {
	r, err := z.open(name)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := xml.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("reading %s in the document: %w", name, err)
	}
	return nil
}

// zipFormat tells a DOCX from an XLSX by the parts inside.
func zipFormat(name string) (string, error) {
	z, err := openZip(name)
	if err != nil {
		return "", err
	}
	defer z.Close()
	switch {
	case z.files["word/document.xml"] != nil:
		return "docx", nil
	case z.files["xl/workbook.xml"] != nil:
		return "xlsx", nil
	}
	return "", errNotDocument
}

// ---- DOCX ----

func docxText(name string, opts TextOptions) (Text, error) {
	z, err := openZip(name)
	if err != nil {
		return Text{}, err
	}
	defer z.Close()
	r, err := z.open("word/document.xml")
	if err != nil {
		return Text{}, err
	}
	defer r.Close()
	out := newTextOut(opts.MaxChars)
	if err := walkDocx(xml.NewDecoder(r), out); err != nil {
		return Text{}, err
	}
	text := out.String()
	if strings.TrimSpace(text) == "" {
		return Text{Format: "docx"}, ErrNoText
	}
	return Text{Format: "docx", Text: text, Truncated: out.truncated}, nil
}

// walkDocx writes word/document.xml as text: a paragraph per line, a table
// row per line with its cells separated by " | ".
func walkDocx(d *xml.Decoder, out *textOut) error {
	type table struct {
		row  []string
		cell strings.Builder
	}
	var (
		para   strings.Builder
		inText bool
		tables []*table
	)
	// emit sends a finished line to the enclosing cell, or to out.
	emit := func(s string) bool {
		if len(tables) == 0 {
			return out.line(s)
		}
		t := tables[len(tables)-1]
		s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
		if s != "" {
			if t.cell.Len() > 0 {
				t.cell.WriteByte(' ')
			}
			t.cell.WriteString(s)
		}
		return true
	}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading the document: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				para.WriteByte('\t')
			case "br", "cr":
				para.WriteByte('\n')
			case "noBreakHyphen":
				para.WriteByte('-')
			case "tbl":
				tables = append(tables, &table{})
			case "pPr", "rPr", "tblPr", "trPr", "tcPr", "sectPr", "Fallback":
				// Properties hold tab stops, not tabs; Fallback repeats
				// what its Choice already said.
				if err := d.Skip(); err != nil {
					return fmt.Errorf("reading the document: %w", err)
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				s := para.String()
				para.Reset()
				if !emit(s) {
					return nil
				}
			case "tc":
				if n := len(tables); n > 0 {
					tb := tables[n-1]
					tb.row = append(tb.row, tb.cell.String())
					tb.cell.Reset()
				}
			case "tr":
				if n := len(tables); n > 0 {
					tb := tables[n-1]
					line := strings.Join(tb.row, " | ")
					tb.row = nil
					tables = tables[:n-1] // the row belongs to the enclosing cell, if any
					ok := emit(line)
					tables = append(tables, tb)
					if !ok {
						return nil
					}
				}
			case "tbl":
				if n := len(tables); n > 0 {
					tables = tables[:n-1]
				}
			}
		case xml.CharData:
			if inText {
				para.Write(t)
			}
		}
	}
}

// ---- XLSX ----

type xlsxSheet struct{ name, part string }

func xlsxText(name string, opts TextOptions) (Text, error) {
	z, err := openZip(name)
	if err != nil {
		return Text{}, err
	}
	defer z.Close()
	sheets, date1904, err := workbookSheets(z)
	if err != nil {
		return Text{}, err
	}
	shared, err := sharedStrings(z)
	if err != nil {
		return Text{}, err
	}
	dates, err := dateStyles(z)
	if err != nil {
		return Text{}, err
	}
	res := Text{Format: "xlsx", Pages: len(sheets)}
	out := newTextOut(opts.MaxChars)
	s := &sheetReader{shared: shared, dates: dates, date1904: date1904, maxRows: opts.MaxRows, out: out}
	for _, sh := range sheets {
		if out.truncated {
			break
		}
		res.Read++
		out.line("## " + sh.name)
		if err := s.read(z, sh.part); err != nil {
			return Text{}, fmt.Errorf("reading sheet %q: %w", sh.name, err)
		}
		out.line("")
	}
	if !s.any {
		return Text{Format: "xlsx", Pages: res.Pages, Read: res.Read}, ErrNoText
	}
	res.Text = out.String()
	res.Truncated = out.truncated || s.cut || res.Read < res.Pages
	return res, nil
}

// workbookSheets lists the sheets in order, with the part each one is in.
func workbookSheets(z *officeZip) ([]xlsxSheet, bool, error) {
	var wb struct {
		Pr struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"` // r:id
		} `xml:"sheets>sheet"`
	}
	if err := z.decode("xl/workbook.xml", &wb); err != nil {
		return nil, false, err
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	targets := map[string]string{}
	if err := z.decode("xl/_rels/workbook.xml.rels", &rels); err == nil {
		for _, r := range rels.Rels {
			t := r.Target
			if strings.HasPrefix(t, "/") {
				t = path.Clean(t[1:])
			} else {
				t = path.Join("xl", t)
			}
			targets[r.ID] = t
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	sheets := make([]xlsxSheet, 0, len(wb.Sheets))
	for i, s := range wb.Sheets {
		part, ok := targets[s.ID]
		if !ok {
			part = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		sheets = append(sheets, xlsxSheet{name: s.Name, part: part})
	}
	return sheets, wb.Pr.Date1904 == "1" || wb.Pr.Date1904 == "true", nil
}

// sharedStrings reads the table most cell text lives in, each entry already
// cut to what a cell shows.
func sharedStrings(z *officeZip) ([]string, error) {
	r, err := z.open("xl/sharedStrings.xml")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer r.Close()
	d := xml.NewDecoder(r)
	var (
		list   []string
		cur    strings.Builder
		inText bool
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return list, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading the spreadsheet's text: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				cur.Reset()
			case "t":
				inText = true
			case "rPh": // phonetic guide, not the text
				if err := d.Skip(); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "si":
				list = append(list, cellText(cur.String()))
			}
		case xml.CharData:
			if inText && cur.Len() <= maxCellChars*4 {
				cur.Write(t)
			}
		}
	}
}

// dateStyles says, by cell style index, which styles show a number as a date.
func dateStyles(z *officeZip) ([]bool, error) {
	var st struct {
		NumFmts []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Xfs []struct {
			NumFmtID int `xml:"numFmtId,attr"`
		} `xml:"cellXfs>xf"`
	}
	if err := z.decode("xl/styles.xml", &st); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	custom := map[int]bool{}
	for _, f := range st.NumFmts {
		custom[f.ID] = isDateFormat(f.Code)
	}
	dates := make([]bool, len(st.Xfs))
	for i, xf := range st.Xfs {
		id := xf.NumFmtID
		if d, ok := custom[id]; ok {
			dates[i] = d
		} else {
			dates[i] = id >= 14 && id <= 22 || id >= 27 && id <= 36 || id >= 45 && id <= 47 || id >= 50 && id <= 58
		}
	}
	return dates, nil
}

// isDateFormat looks for date or time parts in a number format code, outside
// quoted text, escapes and [brackets].
func isDateFormat(code string) bool {
	var quoted, bracket, escaped bool
	for _, c := range strings.ToLower(code) {
		switch {
		case escaped:
			escaped = false
		case quoted:
			quoted = c != '"'
		case bracket:
			bracket = c != ']'
		case c == '\\':
			escaped = true
		case c == '"':
			quoted = true
		case c == '[':
			bracket = true
		case strings.ContainsRune("dmyhs", c):
			return true
		}
	}
	return false
}

type sheetReader struct {
	shared   []string
	dates    []bool
	date1904 bool
	maxRows  int
	out      *textOut
	any      bool // some cell had text
	cut      bool // a sheet had more rows than maxRows
}

// read writes one sheet's rows, cells separated by tabs, empty rows skipped.
func (s *sheetReader) read(z *officeZip, part string) error {
	r, err := z.open(part)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer r.Close()
	d := xml.NewDecoder(r)
	var (
		row          []string
		rows         int
		col          int
		typ, style   string
		val          strings.Builder
		inVal, inStr bool
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = row[:0]
			case "c":
				typ, style, col = "", "", len(row)
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "r":
						if c := columnIndex(a.Value); c >= 0 {
							col = c
						}
					case "t":
						typ = a.Value
					case "s":
						style = a.Value
					}
				}
				val.Reset()
			case "v":
				inVal = true
			case "t":
				inStr = true
			case "f", "rPh": // formula source, phonetic guide
				if err := d.Skip(); err != nil {
					return err
				}
			}
		case xml.CharData:
			if (inVal || inStr) && val.Len() <= maxCellChars*4 {
				val.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inVal = false
			case "t":
				inStr = false
			case "c":
				v := s.value(typ, style, val.String())
				for len(row) < col {
					row = append(row, "")
				}
				if col < len(row) {
					row[col] = v
				} else {
					row = append(row, v)
				}
			case "row":
				line := strings.TrimRight(strings.Join(row, "\t"), "\t")
				if strings.TrimSpace(line) == "" {
					continue
				}
				if rows == s.maxRows {
					s.cut = true
					s.out.line(fmt.Sprintf("[more rows not read: the limit is %d per sheet]", s.maxRows))
					return nil
				}
				rows++
				s.any = true
				if !s.out.line(line) {
					return nil
				}
			case "sheetData":
				return nil
			}
		}
	}
}

func (s *sheetReader) value(typ, style, raw string) string {
	switch typ {
	case "s":
		i, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || i < 0 || i >= len(s.shared) {
			return ""
		}
		return s.shared[i]
	case "b":
		if strings.TrimSpace(raw) == "1" {
			return "TRUE"
		}
		return "FALSE"
	case "", "n":
		if i, err := strconv.Atoi(style); err == nil && i >= 0 && i < len(s.dates) && s.dates[i] {
			if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
				if d, ok := excelDate(f, s.date1904); ok {
					return d
				}
			}
		}
	}
	return cellText(raw) // inlineStr, str, e, d, plain numbers
}

// excelDate formats a date serial: days since 1900 (or 1904), the fraction
// being the time of day.
func excelDate(v float64, date1904 bool) (string, bool) {
	if v < 0 || v > 2958465 { // after 9999-12-31
		return "", false
	}
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	switch {
	case date1904:
		epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	case v < 60: // Excel counts a 29 February 1900 that never was
		epoch = epoch.AddDate(0, 0, 1)
	}
	days := math.Floor(v)
	secs := math.Round((v - days) * 86400)
	t := epoch.AddDate(0, 0, int(days)).Add(time.Duration(secs) * time.Second)
	switch {
	case days == 0 && secs > 0:
		return t.Format("15:04:05"), true
	case secs == 0:
		return t.Format("2006-01-02"), true
	}
	return t.Format("2006-01-02 15:04:05"), true
}

// columnIndex turns the letters of a cell reference ("AB12") into a 0-based
// column, or -1.
func columnIndex(ref string) int {
	c := 0
	for i := 0; i < len(ref); i++ {
		ch := ref[i]
		if ch < 'A' || ch > 'Z' {
			if i == 0 {
				return -1
			}
			break
		}
		c = c*26 + int(ch-'A'+1)
		if c > 16384 { // past column XFD
			return -1
		}
	}
	return c - 1
}

// cellText keeps a cell on one line and within maxCellChars.
func cellText(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s))
	if cut, ok := clip(s, maxCellChars); ok {
		return cut + "…"
	}
	return s
}
