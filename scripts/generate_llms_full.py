"""Pre-build script: concatenates all markdown docs into llms-full.txt."""

import re
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
DOCS_DIR = PROJECT_ROOT / "docs"
OUTPUT = DOCS_DIR / "llms-full.txt"

FM_RE = re.compile(
    r"^-{3}[ \r\t]*?\n.*?\n(?:\.{3}|-{3})[ \r\t]*\n",
    re.UNICODE | re.DOTALL,
)


def main() -> int:
    sections: list[str] = []

    for md_file in sorted(DOCS_DIR.rglob("*.md")):
        if md_file.name == "llms-full.txt":
            continue
        text = md_file.read_text(encoding="utf-8")
        body = FM_RE.sub("", text).strip()
        if not body:
            continue
        rel = md_file.relative_to(DOCS_DIR)
        header = f"# {rel}"
        sections.append(f"{header}\n\n{body}\n")

    OUTPUT.write_text("\n".join(sections), encoding="utf-8")
    print(f"Written {len(sections)} files to {OUTPUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
