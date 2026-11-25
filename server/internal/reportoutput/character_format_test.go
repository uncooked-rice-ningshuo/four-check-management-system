package reportoutput

import (
	"strings"
	"testing"
)

func TestTemplateCharacterFormatting(t *testing.T) {
	raw := `{"type":"doc","schemaVersion":2,"marginTwips":1440,"content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"},"content":[{"type":"text","text":"通报","marks":[{"type":"textStyle","attrs":{"color":"#123abc","spacing":-15}}]}]}]}`
	doc, err := decodeTemplate([]byte(raw), false)
	if err != nil {
		t.Fatal(err)
	}
	xml := renderIntroduction(doc) + templateSection("landscape", doc)
	for _, want := range []string{`<w:color w:val="123abc"/>`, `<w:spacing w:val="-15"/>`, `<w:pgMar w:top="1440" w:bottom="1440" w:left="1440" w:right="1440" w:header="720" w:footer="720"/>`} {
		if !strings.Contains(xml, want) {
			t.Errorf("missing %s", want)
		}
	}
	for from, to := range map[string]string{"#123abc": "red", "-15": "-101", "1440": "2161"} {
		if _, err := decodeTemplate([]byte(strings.Replace(raw, from, to, 1)), false); err == nil {
			t.Errorf("accepted invalid %s", to)
		}
	}
}

func TestTemplateIndependentMarginsAndAdvancedFont(t *testing.T) {
	raw := `{"type":"doc","schemaVersion":2,"margins":{"top":720,"bottom":840,"left":900,"right":1230},"content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"left"},"content":[{"type":"text","text":"AV","marks":[{"type":"textStyle","attrs":{"scale":33,"position":-4,"kern":24,"snap":false}}]}]}]}`
	doc, err := decodeTemplate([]byte(raw), false)
	if err != nil {
		t.Fatal(err)
	}
	xml := renderIntroduction(doc) + templateSection("portrait", doc)
	for _, want := range []string{`<w:w w:val="33"/>`, `<w:position w:val="-4"/>`, `<w:kern w:val="24"/>`, `<w:snapToGrid w:val="false"/>`, `w:top="720" w:bottom="840" w:left="900" w:right="1230" w:header="360" w:footer="420"`} {
		if !strings.Contains(xml, want) {
			t.Errorf("missing %s", want)
		}
	}
	if doc.textWidth("landscape") != 16838-900-1230 {
		t.Fatal("incorrect landscape table width")
	}
}

func TestWordParagraphSettings(t *testing.T) {
	raw := `{"schemaVersion":2,"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"justify","leftChars":3,"rightChars":1,"hanging":2,"beforeLines":50,"afterLines":50,"lineSpacingMode":"fixed","lineSpacing":25,"rtl":true,"outline":2,"paragraphGrid":false,"gridIndent":false,"pageBefore":true,"keepNext":true,"keepLines":true,"widow":false,"tabStop":720},"content":[{"type":"text","text":"甲\t乙"}]}]}`
	doc, err := decodeTemplate([]byte(raw), false)
	if err != nil {
		t.Fatal(err)
	}
	xml := renderIntroduction(doc)
	for _, want := range []string{`w:beforeLines="50"`, `w:afterLines="50"`, `w:line="500" w:lineRule="exact"`, `w:hangingChars="200"`, `w:leftChars="300"`, `w:rightChars="100"`, `<w:jc w:val="both"/>`, `<w:outlineLvl w:val="1"/>`, `<w:bidi w:val="true"/>`, `<w:snapToGrid w:val="false"/>`, `<w:adjustRightInd w:val="false"/>`, `<w:pageBreakBefore w:val="true"/>`, `<w:keepNext w:val="true"/>`, `<w:keepLines w:val="true"/>`, `<w:widowControl w:val="false"/>`, `<w:tab w:val="left" w:pos="720"/>`, `<w:tab/>`} {
		if !strings.Contains(xml, want) {
			t.Errorf("missing %s", want)
		}
	}
	if _, err := decodeTemplate([]byte(strings.Replace(raw, `"beforeLines":50`, `"beforeLines":50,"spaceBefore":12`, 1)), false); err == nil {
		t.Fatal("conflicting spacing units accepted")
	}
}

func TestLargeWordFontSize(t *testing.T) {
	raw := `{"schemaVersion":2,"content":[{"type":"paragraph","content":[{"type":"text","text":"标题","marks":[{"type":"textStyle","attrs":{"fontSize":"108pt"}}]}]},{"type":"wordTable","attrs":{"border":"all","rows":[[{"length":0,"align":"left","fontSize":"108pt"}]]}}]}`
	doc, err := decodeTemplate([]byte(raw), true)
	if err != nil {
		t.Fatal(err)
	}
	var table strings.Builder
	cell := doc.Content[1]
	writeWordTable(&table, cell, *cell.Attrs.Rows, cell.Attrs.Border, doc.textWidth("portrait"))
	for _, xml := range []string{renderIntroduction(IntroductionDocument{Content: doc.Content[:1]}), table.String()} {
		if !strings.Contains(xml, `<w:sz w:val="216"/>`) {
			t.Fatal("missing 108-point font size")
		}
	}
}

func TestChineseSmallFontSizes(t *testing.T) {
	for size, halfPoints := range map[string]string{"5pt": "10", "5.5pt": "11", "6.5pt": "13", "7.5pt": "15"} {
		raw := strings.ReplaceAll(`{"schemaVersion":2,"content":[{"type":"paragraph","content":[{"type":"text","text":"字号","marks":[{"type":"textStyle","attrs":{"fontSize":"SIZE"}}]}]}]}`, "SIZE", size)
		doc, err := decodeTemplate([]byte(raw), true)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(renderIntroduction(doc), `<w:sz w:val="`+halfPoints+`"/>`) {
			t.Errorf("incorrect Word font size for %s", size)
		}
	}
}
