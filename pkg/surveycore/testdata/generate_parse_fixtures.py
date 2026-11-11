"""Generate offline public Parse fixtures using a surveycore-py checkout.

Install the reference checkout into an isolated environment first:
  uv pip install --python <venv-python> -e A:/surveycore-py
Then run with that environment's Python:
  python generate_parse_fixtures.py A:/surveycore-py
"""

import asyncio
from dataclasses import asdict
import json
from pathlib import Path
import subprocess
import sys
from html import escape
from urllib.parse import quote

root = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(root / "src"))

import httpx
from surveycore import SurveyClient


def envelope(data):
    return {"code": "OK", "data": data}


wjx_html = """<html><head><title>解析对照 - 问卷星</title></head><body>
<div id="divQuestion"><fieldset>
<div topic="1" id="div1" type="3" req="1"><div class="topichtml">选择</div>
<div class="ui-controlgroup"><div><span class="label">甲</span></div><div><span class="label">乙</span></div></div></div>
<div topic="2" id="div2" type="8"><div class="topichtml">滑块</div><input id="q2" type="range" min="0" max="10" step="0.5"></div>
<div topic="3" id="div3" type="1" relation="1,2"><div class="topichtml">填空</div><input id="q3" type="text"></div>
</fieldset></div></body></html>"""

qq_questions = [
    {"id": "intro", "type": "description", "title": "说明", "description": "先阅读", "page_id": "p1", "page": 10},
    {"id": "q1", "type": "radio", "title": "选择", "required": True, "page_id": "p1", "page": 10, "options": [{"id": "a", "text": "甲"}, {"id": "b", "text": "乙"}]},
    {"id": "q2", "type": "checkbox", "title": "多选", "page_id": "p2", "page": "next", "min_length": 1, "max_length": 2, "options": [{"text": "A"}, {"text": "B"}]},
    {"id": "q3", "type": "textarea", "title": "文本", "page_id": "p2", "page": "next"},
]

credamo_detail = {
    "surveyTitle": "见数对照",
    "questions": [
        {"qstNo": "Q1", "qstTitle": "选择", "questionType": 2, "selector": 1, "questionId": "q1", "required": True, "choices": [{"display": "甲"}, {"display": "乙"}]},
        {"qstNo": "Q2", "qstTitle": "矩阵", "questionType": 4, "questionId": "q2", "page": 2, "choices": [{"display": "行甲"}, {"display": "行乙"}], "answers": [{"display": "满意"}, {"display": "不满意"}]},
    ],
}

cases = [
    {"name": "wjx", "url": "  WWW.WJX.CN/vm/parity.aspx  ", "responses": {"/vm/parity.aspx": wjx_html}},
    {"name": "qq", "url": "https://wj.qq.com/s2/123/hash/", "responses": {
        "/api/v2/respondent/surveys/123/session": envelope({"answer_session_id": "fixture-session"}),
        "/api/v2/respondent/surveys/123/meta": envelope({"title": "腾讯对照 - 腾讯问卷"}),
        "/api/v2/respondent/surveys/123/questions": envelope({"questions": qq_questions}),
    }},
    {"name": "credamo", "url": "WWW.CREDAMO.COM/s/demo_?from=fixture", "responses": {
        "/v1/survey/noauth/detail/get/demoano": {"success": True, "data": credamo_detail},
    }},
    {"name": "wjx-multilevel", "url": "https://v.wjx.cn/vm/levels.aspx", "responses": {
        "/vm/levels.aspx": '''<html><head><title>层级 - 问卷星</title></head><body>
<div id="divQuestion"><fieldset>
<div topic="1" type="9" gapfill="1" relation="0" style="display:none"></div>
<div topic="2" id="div2" type="1" req="1"><div class="topichtml">请选择</div>
<input type="text" id="q2" verify="多级下拉" readonly="readonly"></div>
</fieldset></div><script>function DecodeId(input) { return input ^ 2130030173; }
var activityId = DecodeId(1797297031);</script></body></html>''',
        "/joinnew/setmenuselnew.aspx": '<input id="hfLevelData" value="总部╰1╯┌|---总部╰1╯┌\n研发部\n支持部〒公司|部门">',
    }},
]


cid_case = json.loads(json.dumps(cases[-1]))
cid_case["name"] = "wjx-multilevel-cid"
cid_case["responses"]["/vm/levels.aspx"] = cid_case["responses"]["/vm/levels.aspx"].replace(
    'verify="多级下拉"', 'verify="多级下拉" cid="cid47"'
).replace("DecodeId(1797297031)", "366296538")
cid_case["responses"]["/joinnew/setmenuselnew.aspx"] = (
    '<input id="hfLevelData" value="' + escape(quote("总部╰1╯┌|---总部╰1╯┌\n研发部\n支持部〒公司|部门"), quote=True) + '">'
)
cases.append(cid_case)


async def generate():
    for case in cases:
        def handler(request):
            body = case["responses"][request.url.path]
            return httpx.Response(200, text=body) if isinstance(body, str) else httpx.Response(200, json=body)

        async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as http_client:
            case["expected"] = asdict(await SurveyClient(http_client).parse(case["url"]))
    output = {
        "python_commit": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
        "cases": cases,
    }
    Path(__file__).with_name("python-parse.json").write_text(json.dumps(output, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


asyncio.run(generate())
