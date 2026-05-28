package wjx

import "testing"

func TestParseHTMLExtractsWjxQuestionTypesWithoutFieldsetDuplicate(t *testing.T) {
	html := `
<html><body>
  <div id="divQuestion">
    <fieldset>
      <div topic="1" id="div1" type="3">
        <div class="topichtml">1. 请选择</div>
        <div class="ui-controlgroup">
          <div><span class="label">A</span></div>
          <div><span class="label">B</span></div>
        </div>
      </div>
      <div topic="2" id="div2" type="8">
        <div class="topichtml">2. 请拖动滑块</div>
        <input id="q2" type="range" min="1" max="5" step="0.5" />
      </div>
    </fieldset>
  </div>
</body></html>`

	questions, _, err := ParseHTML(html)
	if err != nil {
		t.Fatalf("ParseHTML returned error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("questions length = %d, want 2: %#v", len(questions), questions)
	}
	if questions[0].TypeCode != "3" || questions[0].Options != 2 {
		t.Fatalf("first question = %#v, want WJX single type 3 with 2 options", questions[0])
	}
	if questions[0].DisplayNum == nil || *questions[0].DisplayNum != 1 || questions[0].Title != "请选择" {
		t.Fatalf("first title/display = %q/%v, want cleaned title and display num 1", questions[0].Title, questions[0].DisplayNum)
	}
	if questions[1].TypeCode != "8" || questions[1].SliderMin == nil || *questions[1].SliderMin != 1 {
		t.Fatalf("second question = %#v, want WJX slider type 8", questions[1])
	}
}

func TestParseHTMLExtractsForcedChoiceFromTitle(t *testing.T) {
	html := `
<html><body>
  <div topic="1" id="div1" type="3">
    <div class="topichtml">1. 本题检测，请选择 非常满意</div>
    <div class="ui-controlgroup">
      <div><span class="label">一般</span></div>
      <div><span class="label">非常满意</span></div>
    </div>
  </div>
  <div topic="2" id="div2" type="3">
    <div class="topichtml">2. 请务必选B项</div>
    <div class="ui-controlgroup">
      <div><span class="label">(A) 苹果</span></div>
      <div><span class="label">(B) 香蕉</span></div>
    </div>
  </div>
</body></html>`

	questions, _, err := ParseHTML(html)
	if err != nil {
		t.Fatalf("ParseHTML returned error: %v", err)
	}
	if questions[0].ForcedOptionIndex == nil || *questions[0].ForcedOptionIndex != 1 {
		t.Fatalf("first forced index = %v, want 1", questions[0].ForcedOptionIndex)
	}
	if questions[0].ForcedOptionText != "非常满意" {
		t.Fatalf("first forced text = %q, want 非常满意", questions[0].ForcedOptionText)
	}
	if questions[1].ForcedOptionIndex == nil || *questions[1].ForcedOptionIndex != 1 {
		t.Fatalf("second forced index = %v, want 1", questions[1].ForcedOptionIndex)
	}
}

func TestParseHTMLExtractsMatrixHeaderAndRows(t *testing.T) {
	html := `
<html><body>
  <div topic="3" id="div3" type="6">
    <div class="topichtml">3. 请评价以下项目</div>
    <table id="divRefTab3">
      <tr id="drv3_1"><td></td><td>差</td><td>好</td></tr>
      <tr rowindex="1"><td>外观</td><td><input name="q3_1_1" type="radio" /></td><td><input name="q3_1_2" type="radio" /></td></tr>
      <tr rowindex="2"><td data-title="功能"></td><td><input name="q3_2_1" type="radio" /></td><td><input name="q3_2_2" type="radio" /></td></tr>
    </table>
  </div>
</body></html>`

	questions, _, err := ParseHTML(html)
	if err != nil {
		t.Fatalf("ParseHTML returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("questions length = %d, want 1", len(questions))
	}
	got := questions[0]
	if got.Rows != 2 || got.Options != 2 {
		t.Fatalf("matrix rows/options = %d/%d, want 2/2: %#v", got.Rows, got.Options, got)
	}
	if got.RowTexts[0] != "外观" || got.RowTexts[1] != "功能" {
		t.Fatalf("row texts = %#v, want 外观/功能", got.RowTexts)
	}
	if got.OptionTexts[0] != "差" || got.OptionTexts[1] != "好" {
		t.Fatalf("option texts = %#v, want 差/好", got.OptionTexts)
	}
}
