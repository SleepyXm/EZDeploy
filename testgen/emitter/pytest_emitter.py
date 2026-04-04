from __future__ import annotations
 
import json
import textwrap
from pathlib import Path
from typing import Any
 
from generator.schema import (
    FunctionTestBank,
    MockSpec,
    ProjectTestBank,
    TestCase,
)
 
# ---------------------------------------------------------------------------
# Value rendering
# ---------------------------------------------------------------------------
 
def _render_value(v: Any, indent: int = 0) -> str:
    """Render a Python literal from a JSON value."""
    if v is None:
        return "None"
    if isinstance(v, bool):
        return "True" if v else "False"
    if isinstance(v, str):
        # Prefer single quotes; escape if needed
        escaped = v.replace("\\", "\\\\").replace("'", "\\'")
        return f"'{escaped}'"
    if isinstance(v, (int, float)):
        return repr(v)
    if isinstance(v, list):
        items = ", ".join(_render_value(i) for i in v)
        return f"[{items}]"
    if isinstance(v, dict):
        pairs = ", ".join(f"{_render_value(k)}: {_render_value(val)}" for k, val in v.items())
        return "{" + pairs + "}"
    return repr(v)
 
 
# ---------------------------------------------------------------------------
# Single test function
# ---------------------------------------------------------------------------
 
def _render_test_case(fn: FunctionTestBank, case: TestCase) -> str:
    lines: list[str] = []
 
    # Decorator
    if case.skip_reason:
        lines.append(f'    @pytest.mark.skip(reason={_render_value(case.skip_reason)})')
    lines.append(f'    @pytest.mark.{case.type}')
 
    # Signature
    lines.append(f'    def test_{case.id}(self):')
 
    # Docstring
    lines.append(f'        """{case.description}"""')
 
    # Mocks
    mock_vars: list[str] = []
    mock_ctx: list[str] = []
    for i, mock in enumerate(case.mocks):
        var = f"mock_{i}"
        mock_vars.append(var)
        rv = _render_value(mock.return_value) if mock.return_value is not None else "None"
        if mock.side_effect:
            # side_effect is an exception class name
            mock_ctx.append(
                f'        with patch({_render_value(mock.target)}, '
                f'side_effect={mock.side_effect}()) as {var}:'
            )
        else:
            mock_ctx.append(
                f'        with patch({_render_value(mock.target)}, '
                f'return_value={rv}) as {var}:'
            )
 
    body_indent = "        "  # base indent inside test method
 
    if mock_ctx:
        # Nest each with-block inside the previous one
        for depth, ctx_line in enumerate(mock_ctx):
            lines.append("    " * depth + ctx_line.lstrip())
        body_indent += "    " * len(mock_ctx)
 
    # Build call expression
    call_args = ", ".join(
        f"{k}={_render_value(v)}" for k, v in (case.args or {}).items()
    )
 
    qualified = (
        f"{fn.class_name}().{fn.function}"
        if fn.class_name
        else fn.function
    )
 
    if case.expect_exception:
        lines.append(f"{body_indent}with pytest.raises({case.expect_exception}):")
        lines.append(f"{body_indent}    {qualified}({call_args})")
    else:
        lines.append(f"{body_indent}result = {qualified}({call_args})")
        if case.expect_return is not None:
            lines.append(
                f"{body_indent}assert result == {_render_value(case.expect_return)}"
            )
 
    # Side-effect assertions (freeform comments since they vary)
    for effect in case.expect_side_effects:
        lines.append(f"{body_indent}# assert: {effect}")
 
    # Close mock context managers (just pass — the with block handles it)
    lines.append("")
    return "\n".join(lines)
 
 
# ---------------------------------------------------------------------------
# Single test class (one per function/endpoint)
# ---------------------------------------------------------------------------
 
def _render_test_class(fn: FunctionTestBank) -> str:
    safe_name = fn.function.replace(".", "_")
    class_name_part = f"{fn.class_name}_" if fn.class_name else ""
    klass = f"Test_{class_name_part}{safe_name}"
 
    lines: list[str] = [f"class {klass}:"]
 
    if fn.route_path:
        lines.append(
            f'    """Tests for {fn.http_methods} {fn.route_path} '
            f'({fn.module}.{fn.function})"""'
        )
    else:
        lines.append(f'    """Tests for {fn.module}.{fn.function}"""')
 
    lines.append("")
 
    for case in fn.cases:
        lines.append(_render_test_case(fn, case))
 
    return "\n".join(lines)
 
 
# ---------------------------------------------------------------------------
# Full file
# ---------------------------------------------------------------------------
 
def _render_file(functions: list[FunctionTestBank]) -> str:
    # Gather unique modules to import from
    modules: dict[str, set[str]] = {}  # module → {function names}
    for fn in functions:
        modules.setdefault(fn.module, set()).add(fn.function)
 
    header = textwrap.dedent("""\
        # Auto-generated by testgen — do not edit manually.
        # Re-generate with:  testgen emit tests.json
        import pytest
        from unittest.mock import patch, MagicMock
    """)
 
    import_lines: list[str] = []
    for module, names in sorted(modules.items()):
        importable = sorted(n for n in names if n != "__init__")
        if importable:
            import_lines.append(f"from {module} import {', '.join(importable)}")
 
    body = "\n\n\n".join(_render_test_class(fn) for fn in functions)
 
    parts = [header]
    if import_lines:
        parts.append("\n".join(import_lines))
    parts.append("")
    parts.append(body)
    return "\n".join(parts) + "\n"
 
 
# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------
 
class PytestEmitter:
    """
    Reads a ``ProjectTestBank`` and writes ``test_*.py`` files.
 
    *output_dir*: where to write the files. Defaults to ``tests/generated/``.
    *one_file*: if True, write everything into a single ``test_all.py``.
                If False (default), one file per source module.
    """
 
    def __init__(
        self,
        output_dir: Path = Path("tests/generated"),
        one_file: bool = False,
    ) -> None:
        self.output_dir = output_dir
        self.one_file = one_file
 
    def emit(self, bank: ProjectTestBank) -> list[Path]:
        """Write test files. Returns list of paths written."""
        self.output_dir.mkdir(parents=True, exist_ok=True)
        written: list[Path] = []

        if self.one_file:
            out = self.output_dir / "test_all.py"
            out.write_text(_render_file(bank.functions), encoding="utf-8")
            print(f"[testgen] wrote {out}")
            written.append(out)
        else:
            # Group by module
            by_module: dict[str, list[FunctionTestBank]] = {}
            for fn in bank.functions:
                # Clean up the module path - strip leading dots/underscores
                clean_module = fn.module.lstrip("._") if fn.module else "root"
            
                by_module.setdefault(clean_module, []).append(fn)

            for module, fns in sorted(by_module.items()):
                stem = module.replace(".", "_")
                out = self.output_dir / f"test_{stem}.py"
                out.write_text(_render_file(fns), encoding="utf-8")
                print(f"[testgen] wrote {out}  ({len(fns)} function(s))")
                written.append(out)

        return written