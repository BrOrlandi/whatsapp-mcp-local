package mediaread

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

const docxBody = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">
<w:body>
<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="720"/></w:tabs></w:pPr><w:r><w:t>Olá,</w:t></w:r><w:r><w:t xml:space="preserve"> mundo</w:t></w:r></w:p>
<w:p><w:r><w:t>Nome</w:t><w:tab/><w:t>Bruno</w:t><w:br/><w:t>linha 2</w:t></w:r></w:p>
<w:p/>
<w:p/>
<w:tbl><w:tblPr><w:tblW w:w="0"/></w:tblPr>
<w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p><w:p><w:r><w:t>mais</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>A2</w:t></w:r></w:p></w:tc><w:tc><w:p/></w:tc></w:tr>
</w:tbl>
<w:p><w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:t>caixa</w:t></mc:Choice><mc:Fallback><w:t>caixa</w:t></mc:Fallback></mc:AlternateContent></w:r></w:p>
<w:sectPr/>
</w:body>
</w:document>`

func docx(t *testing.T, body string) []byte {
	return zipParts(t,
		[2]string{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`},
		[2]string{"word/document.xml", body})
}

func TestReadTextDOCX(t *testing.T) {
	// No extension and no MIME: the zip's parts tell.
	got, err := ReadText(writeTemp(t, "document", docx(t, docxBody)), "", TextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "Olá, mundo\nNome\tBruno\nlinha 2\n\nA1 | B1 mais\nA2 |\ncaixa"
	if got.Format != "docx" || got.Text != want || got.Truncated {
		t.Fatalf("got %+v\nwant text %q", got, want)
	}
}

func TestReadTextDOCXTruncates(t *testing.T) {
	got, err := ReadText(writeTemp(t, "a.docx", docx(t, docxBody)), mimeDOCX, TextOptions{MaxChars: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Olá" || !got.Truncated {
		t.Fatalf("got %q truncated %v", got.Text, got.Truncated)
	}
}

func TestReadTextZipGuard(t *testing.T) {
	old := maxZipBytes
	maxZipBytes = 1024
	defer func() { maxZipBytes = old }()
	body := `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>` + strings.Repeat("a", 4096) + `</w:t></w:r></w:p></w:body></w:document>`
	_, err := ReadText(writeTemp(t, "bomb.docx", docx(t, body)), "", TextOptions{})
	if err == nil || !strings.Contains(err.Error(), "expands to more than") {
		t.Fatalf("err = %v", err)
	}
}

func xlsx(t *testing.T) []byte {
	return zipParts(t,
		[2]string{"xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Vendas" sheetId="1" r:id="rId2"/><sheet name="Notas" sheetId="2" r:id="rId1"/></sheets></workbook>`},
		[2]string{"xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet1.xml"/>
</Relationships>`},
		[2]string{"xl/sharedStrings.xml", `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<si><t>Produto</t></si><si><r><t>Pre</t></r><r><t>ço</t></r></si><si><t>Café</t><rPh><t>kafe</t></rPh></si></sst>`},
		[2]string{"xl/styles.xml", `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="1"><numFmt numFmtId="164" formatCode="dd/mm/yyyy"/></numFmts>
<cellXfs count="3"><xf numFmtId="0"/><xf numFmtId="164"/><xf numFmtId="4"/></cellXfs></styleSheet>`},
		[2]string{"xl/worksheets/sheet1.xml", `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>Data</t></is></c></row>
<row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2" s="2"><v>12.5</v></c><c r="C2" s="1"><v>45658</v></c><c r="E2" t="b"><v>1</v></c></row>
<row r="3"/>
<row r="4"><c r="A4" t="str"><f>A1</f><v>Produto</v></c></row>
</sheetData></worksheet>`},
		[2]string{"xl/worksheets/sheet2.xml", `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
<row r="1"><c r="B1" t="inlineStr"><is><t>só B</t></is></c></row>
</sheetData></worksheet>`})
}

func TestReadTextXLSX(t *testing.T) {
	got, err := ReadText(writeTemp(t, "planilha.xlsx", xlsx(t)), mimeXLSX, TextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "## Vendas\nProduto\tPreço\tData\nCafé\t12.5\t2025-01-01\t\tTRUE\nProduto\n\n## Notas\n\tsó B"
	if got.Format != "xlsx" || got.Text != want || got.Pages != 2 || got.Read != 2 || got.Truncated {
		t.Fatalf("got %+v\nwant text %q", got, want)
	}
}

func TestReadTextXLSXMaxRows(t *testing.T) {
	got, err := ReadText(writeTemp(t, "planilha.xlsx", xlsx(t)), "", TextOptions{MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || !strings.HasPrefix(got.Text, "## Vendas\nProduto\tPreço\tData\n[more rows not read") || strings.Contains(got.Text, "Café") {
		t.Fatalf("got %+v", got)
	}
}

func TestReadTextPlain(t *testing.T) {
	for _, c := range []struct {
		name, mime string
		data       []byte
		max        int
		want       string
		truncated  bool
	}{
		{"utf8.txt", "text/plain", []byte("olá mundo"), 0, "olá mundo", false},
		{"cut.txt", "", []byte("olá mundo"), 3, "olá", true},
		{"cut-emoji.txt", "", []byte("a😀b"), 2, "a😀", true},
		{"latin1.csv", "text/csv; charset=windows-1252", []byte("a\xe7\xe3o;pre\xe7o\n"), 0, "ação;preço\n", false},
		{"utf16.txt", "", []byte("\xff\xfeo\x00l\x00\xe1\x00"), 0, "olá", false},
		{"bom.json", "application/json", []byte("\xef\xbb\xbf{\"a\":1}"), 0, `{"a":1}`, false},
		{"mixed.txt", "", []byte("ção \xff fim"), 0, "ção � fim", false},
	} {
		got, err := ReadText(writeTemp(t, c.name, c.data), c.mime, TextOptions{MaxChars: c.max})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Format != "text" || got.Text != c.want || got.Truncated != c.truncated || !utf8.ValidString(got.Text) {
			t.Fatalf("%s: got %q truncated %v, want %q truncated %v", c.name, got.Text, got.Truncated, c.want, c.truncated)
		}
	}
}

func TestReadTextUnsupported(t *testing.T) {
	for name, data := range map[string][]byte{
		"blob.bin":   {0x00, 0x01, 0x02, 0xff, 0x00},
		"slides.zip": zipParts(t, [2]string{"ppt/presentation.xml", "<p/>"}),
	} {
		if _, err := ReadText(writeTemp(t, name, data), "", TextOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
}

func TestClipNeverSplitsARune(t *testing.T) {
	s := "ação😀"
	for n := 0; n <= 6; n++ {
		got, cut := clip(s, n)
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) != min(n, 5) || cut != (n < 5) {
			t.Fatalf("clip(%q, %d) = %q, %v", s, n, got, cut)
		}
	}
}
