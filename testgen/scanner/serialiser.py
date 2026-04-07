"""
Serialisation — converts scanner dataclasses to plain dicts
so they can be JSON-encoded and handed to the LLM.
"""
 
from __future__ import annotations
 
from dataclasses import asdict
from typing import Any
 
from .ast_scanner import FunctionInfo, ParamInfo
 
 
def function_to_dict(fn: FunctionInfo) -> dict[str, Any]:
    """Minimal, LLM-friendly dict for a single function."""
    d: dict[str, Any] = {
        "name": fn.name,
        "module": fn.module,
        "source_file": fn.source_file,
        "lineno": fn.lineno,
        "is_async": fn.is_async,
        "params": [_param_dict(p) for p in fn.params],
        "return_annotation": fn.return_annotation,
        "decorators": fn.decorators,
        "docstring": fn.docstring,
    }
    if fn.is_method:
        d["class_name"] = fn.class_name
    if fn.http_methods:
        d["http_methods"] = fn.http_methods
        d["route_path"] = fn.route_path
        d["framework"] = fn.framework
    return d
 
 
def _param_dict(p: ParamInfo) -> dict[str, Any]:
    d: dict[str, Any] = {"name": p.name, "kind": p.kind}
    if p.annotation:
        d["type"] = p.annotation
    if p.default is not None:
        d["default"] = p.default
    return d
 
 
def functions_to_payload(functions: list[FunctionInfo]) -> list[dict[str, Any]]:
    """Convert a list of FunctionInfo objects to a JSON-serialisable list."""
    return [function_to_dict(fn) for fn in functions]
 