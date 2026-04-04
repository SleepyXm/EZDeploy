from __future__ import annotations
 
import json
import os
import re
import textwrap
from typing import Any
from dotenv import load_dotenv

from openai import OpenAI
 
from .schema import (
    FunctionTestBank,
    ProjectTestBank,
    TestCase,
    MockSpec,
    bank_from_dict,
)

load_dotenv()
 
# ---------------------------------------------------------------------------
# Prompt templates
# ---------------------------------------------------------------------------
 
SYSTEM_PROMPT = textwrap.dedent("""\
    You are a test case architect for Python projects.
 
    You receive a JSON description of one or more Python functions/endpoints
    (names, parameters, type annotations, decorators, docstrings).
 
    Your ONLY job is to output a JSON test bank — a structured list of test cases
    that thoroughly exercise each function. You do NOT write pytest code.
    You do NOT generate imports. You reason about inputs and expected outputs.
 
    Output format (strict JSON, no markdown, no explanation):
 
    {
      "functions": [
        {
          "function": "<name>",
          "module": "<dotted.module>",
          "class_name": null,
          "route_path": null,
          "http_methods": [],
          "cases": [
            {
              "id": "<snake_case_id>",
              "type": "unit" | "integration" | "edge" | "regression",
              "description": "<one sentence>",
              "args": { "<param_name>": <value>, ... },
              "expect_return": <value_or_null>,
              "expect_exception": "<ExceptionClassName_or_null>",
              "expect_side_effects": [],
              "mocks": [
                { "target": "<dotted.path.to.patch>", "return_value": <value> }
              ],
              "tags": []
            }
          ]
        }
      ]
    }
 
    Rules:
    - Generate at minimum: 1 happy path, 2 edge cases, 1 regression case per function.
    - For endpoints also include: wrong HTTP method, missing required fields, auth failure (if auth decorator present).
    - Use null for expect_return when the return value is not meaningful (e.g. void functions, or when an exception is expected).
    - For mocks, use the full dotted path that would be passed to unittest.mock.patch.
    - Case ids must be unique within a function, snake_case, concise.
    - Output ONLY the JSON object. No markdown fences. No preamble. No trailing text.
""")
 
 
def _build_user_message(payload: list[dict[str, Any]]) -> str:
    return (
        "Generate a test bank for the following functions:\n\n"
        + json.dumps(payload, indent=2)
    )
 
 
# ---------------------------------------------------------------------------
# Response parsing
# ---------------------------------------------------------------------------
 
def _extract_json(text: str) -> str:
    """Strip accidental markdown fences and find the outermost JSON object."""
    text = text.strip()
    # Remove ```json ... ``` or ``` ... ```
    text = re.sub(r"^```(?:json)?\s*", "", text)
    text = re.sub(r"\s*```$", "", text)
    text = text.strip()
    # Find the first { ... } at the top level
    start = text.find("{")
    if start == -1:
        raise ValueError("No JSON object found in LLM response")
    # Walk to find the matching closing brace
    depth = 0
    for i, ch in enumerate(text[start:], start):
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return text[start : i + 1]
    raise ValueError("Unmatched braces in LLM response")
 
 
def _parse_response(raw: str) -> ProjectTestBank:
    json_str = _extract_json(raw)
    try:
        d = json.loads(json_str)
    except json.JSONDecodeError as exc:
        raise ValueError(f"LLM returned invalid JSON: {exc}\n\nRaw:\n{json_str[:500]}") from exc
    # Wrap in the top-level schema if missing
    if "schema_version" not in d:
        d["schema_version"] = "1.0"
        d["generator"] = "testgen"
    return bank_from_dict(d)
 
 
# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

class TestBankGenerator:
    """
    Wraps the OpenAI-compatible DeepSeek API call.  Feed it a list of function metadata dicts
    (from ``scanner.functions_to_payload``), get back a ``ProjectTestBank``.
    """
 
    def __init__(
        self,
        model: str = "deepseek-chat",
        api_key: str | None = None,
        batch_size: int = 20,
    ) -> None:
        key = api_key or os.environ.get("DEEPSEEK_API_KEY")
        if not key:
            raise EnvironmentError(
                "[testgen] DEEPSEEK_API_KEY environment variable is not set."
            )

        # Include /v1 suffix for proper OpenAI-compatible routing
        self.client = OpenAI(
            api_key=key,
            base_url="https://api.deepseek.com/v1"
        )

        self.model = model
        self.batch_size = batch_size
 
    def generate(
        self,
        payload: list[dict],
        verbose: bool = False,
    ) -> ProjectTestBank:
        """
        Call the LLM with *payload* (list of function metadata dicts).
        Returns a merged ``ProjectTestBank``.
 
        Large payloads are automatically split into batches of
        ``self.batch_size`` functions each.
        """
        if not payload:
            from .schema import ProjectTestBank
            return ProjectTestBank()
 
        batches = [
            payload[i : i + self.batch_size]
            for i in range(0, len(payload), self.batch_size)
        ]
 
        all_functions: list = []
 
        for idx, batch in enumerate(batches):
            if verbose:
                names = [f["name"] for f in batch]
                print(
                    f"[testgen] batch {idx+1}/{len(batches)}: "
                    f"{len(batch)} function(s) → {names}"
                )
 
            bank = self._call_llm(batch)
            all_functions.extend(bank.functions)
 
        from .schema import ProjectTestBank
        return ProjectTestBank(functions=all_functions)
 
    def _call_llm(self, batch: list[dict]) -> ProjectTestBank:
        try:
            response = self.client.chat.completions.create(
                model=self.model,
                messages=[
                    {
                        "role": "system",
                        "content": SYSTEM_PROMPT
                    },
                    {
                        "role": "user",
                        "content": _build_user_message(batch)
                    }
                ],
                max_tokens=4096,
            )
        except Exception as exc:
            raise RuntimeError(f"[testgen] API error: {exc}") from exc

        message = response.choices[0].message
        raw = message.content if hasattr(message, "content") else message["content"]
        return _parse_response(raw)