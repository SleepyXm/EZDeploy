from __future__ import annotations
 
import argparse
import json
import sys
from pathlib import Path
 
from scanner.ast_scanner import scan_file, scan_project
from scanner.serialiser import functions_to_payload
from generator.llm import TestBankGenerator
from generator.schema import bank_to_json, bank_from_json
from emitter.pytest_emitter import PytestEmitter
 
 
# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------
 
def cmd_scan(args: argparse.Namespace) -> int:
    """Scan source files / directories and print the structured payload."""
    target = Path(args.target)
 
    if target.is_dir():
        functions = scan_project(target)
    else:
        functions = scan_file(target)
 
    payload = functions_to_payload(functions)
 
    if not payload:
        print("[testgen] no public functions found.", file=sys.stderr)
        return 1
 
    output = json.dumps(payload, indent=2)
 
    if args.output:
        Path(args.output).write_text(output, encoding="utf-8")
        print(f"[testgen] wrote payload → {args.output}  ({len(payload)} function(s))")
    else:
        print(output)
 
    return 0
 
 
def cmd_generate(args: argparse.Namespace) -> int:
    """Scan + call LLM + write tests.json."""
    target = Path(args.target)
 
    # --- scan ---
    if target.is_dir():
        functions = scan_project(target)
    elif target.suffix == ".json":
        # Accept a pre-built payload JSON directly (skip scanning)
        payload = json.loads(target.read_text(encoding="utf-8"))
        functions = None
    else:
        functions = scan_file(target)
 
    if functions is not None:
        payload = functions_to_payload(functions)
 
    if not payload:
        print("[testgen] nothing to generate — no public functions found.", file=sys.stderr)
        return 1
 
    print(f"[testgen] {len(payload)} function(s) found, calling LLM …")
 
    # --- apply filter if given ---
    if args.only:
        names = {n.strip() for n in args.only.split(",")}
        payload = [f for f in payload if f["name"] in names]
        if not payload:
            print(f"[testgen] --only filter matched nothing.", file=sys.stderr)
            return 1
 
    # --- generate ---
    gen = TestBankGenerator(model="deepseek-chat", batch_size=args.batch_size)
    bank = gen.generate(payload, verbose=args.verbose)
 
    # --- save ---
    out_path = Path(args.output)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(bank_to_json(bank), encoding="utf-8")
 
    total_cases = sum(len(f.cases) for f in bank.functions)
    print(
        f"[testgen] wrote {out_path}  "
        f"({len(bank.functions)} function(s), {total_cases} test case(s))"
    )
 
    # --- optionally emit immediately ---
    if args.emit:
        emitter = PytestEmitter(
            output_dir=Path(args.emit_dir),
            one_file=args.one_file,
        )
        emitter.emit(bank)
 
    return 0
 
 
def cmd_emit(args: argparse.Namespace) -> int:
    """Read tests.json and write pytest files."""
    bank_path = Path(args.bank)
    if not bank_path.exists():
        print(f"[testgen] file not found: {bank_path}", file=sys.stderr)
        return 1
 
    bank = bank_from_json(bank_path.read_text(encoding="utf-8"))
 
    emitter = PytestEmitter(
        output_dir=Path(args.output_dir),
        one_file=args.one_file,
    )
    written = emitter.emit(bank)
    print(f"[testgen] {len(written)} file(s) written.")
    return 0
 
 
# ---------------------------------------------------------------------------
# Parser
# ---------------------------------------------------------------------------
 
def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="testgen",
        description="AST-driven, LLM-powered test bank generator for Python.",
    )
    sub = parser.add_subparsers(dest="command", required=True)
 
    # ---- scan ----
    p_scan = sub.add_parser(
        "scan",
        help="Scan source files and print the structured payload (no LLM).",
    )
    p_scan.add_argument("target", help="Python file or project directory to scan.")
    p_scan.add_argument("-o", "--output", help="Write JSON payload to this file instead of stdout.")
 
    # ---- generate ----
    p_gen = sub.add_parser(
        "generate",
        help="Scan + LLM → tests.json",
    )
    p_gen.add_argument(
        "target",
        help="Python file, project directory, or pre-built payload JSON.",
    )
    p_gen.add_argument(
        "-o", "--output",
        default="tests.json",
        help="Output path for the test bank JSON (default: tests.json).",
    )
    p_gen.add_argument(
        "--only",
        help="Comma-separated list of function names to include (e.g. 'charge_user,refund').",
    )
    p_gen.add_argument(
        "--model",
        default="claude-sonnet-4-20250514",
        help="Anthropic model to use.",
    )
    p_gen.add_argument(
        "--batch-size",
        type=int,
        default=20,
        help="Max functions per LLM call (default: 20).",
    )
    p_gen.add_argument(
        "--emit",
        action="store_true",
        help="Also emit pytest files immediately after generating.",
    )
    p_gen.add_argument(
        "--emit-dir",
        default="tests/generated",
        help="Output directory for emitted pytest files (default: tests/generated).",
    )
    p_gen.add_argument(
        "--one-file",
        action="store_true",
        help="Emit everything into a single test_all.py instead of per-module files.",
    )
    p_gen.add_argument("-v", "--verbose", action="store_true")
 
    # ---- emit ----
    p_emit = sub.add_parser(
        "emit",
        help="Read tests.json and write pytest files (no LLM).",
    )
    p_emit.add_argument("bank", help="Path to tests.json.")
    p_emit.add_argument(
        "-o", "--output-dir",
        default="tests/generated",
        help="Output directory (default: tests/generated).",
    )
    p_emit.add_argument(
        "--one-file",
        action="store_true",
        help="Write everything into test_all.py.",
    )
 
    return parser
 
 
def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
 
    handlers = {
        "scan": cmd_scan,
        "generate": cmd_generate,
        "emit": cmd_emit,
    }
    sys.exit(handlers[args.command](args))
 
 
if __name__ == "__main__":
    main()