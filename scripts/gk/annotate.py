#!/usr/bin/env python3
"""Draw "coretan" (circle + strike-through + correction note) onto GK
Kontraktor for every finding Deep Revisi AI produced, and save the result as
a new annotated PDF — the automated version of the manual Microsoft Edge
markup procedure described in dashboard/skillmd/pengecekan-gambar-kerja.md.

Usage:
    python annotate.py <kontraktor.pdf> <findings.json> <output.pdf>

findings.json is a JSON array of objects:
    [{"page": 1, "wrong": "-0.50", "correct": "-0.45",
      "explain": "...", "confidence": "tinggi"}, ...]
"page" is 1-based (matches the page the finding was found on in Kontraktor).

For each finding, this script searches the page's text layer for the "wrong"
value. If found, it circles + strikes it and writes a "SALAH -> SEHARUSNYA"
note beside it. If the value cannot be found as searchable text (common for
CAD-exported dimension text rendered as vector shapes, not real text), it
falls back to a floating banner note at the top of the page instead of
failing silently — per the skill's "jangan gagal diam-diam" rule.

Prints {"ok": true, "annotated": N} on success. Requires: pip install pymupdf
"""
import json
import sys

try:
    import fitz  # PyMuPDF
except ImportError:
    print("PyMuPDF belum terpasang. Jalankan: pip install pymupdf", file=sys.stderr)
    sys.exit(1)

RED = (1, 0, 0)
NOTE_FILL = (1, 1, 0.85)


def circle(page, rect, pad=10):
    r = fitz.Rect(rect)
    circ = fitz.Rect(r.x0 - pad, r.y0 - pad, r.x1 + pad, r.y1 + pad)
    ann = page.add_circle_annot(circ)
    ann.set_colors(stroke=RED)
    ann.set_border(width=1.6)
    ann.update()
    return circ


def strike(page, rect, pad=2):
    r = fitz.Rect(rect)
    midy = (r.y0 + r.y1) / 2
    ln = page.add_line_annot(fitz.Point(r.x0 - pad, midy), fitz.Point(r.x1 + pad, midy))
    ln.set_colors(stroke=RED)
    ln.set_border(width=1.8)
    ln.update()


def note(page, text_rect, text):
    fta = page.add_freetext_annot(text_rect, text, fontsize=6.5, text_color=RED,
                                   fill_color=NOTE_FILL)
    if page.rotation:
        # PyMuPDF freetext annots don't auto-follow the page's own /Rotate —
        # without this the note text renders sideways/upside-down on rotated
        # CAD-exported sheets (validated empirically: page.rotation itself,
        # not 360-page.rotation, is the correct compensation here).
        fta.set_rotation(page.rotation)
    fta.update()


def place_note_near(page, anchor_rect, text, pad=10):
    circ = fitz.Rect(anchor_rect).x0, fitz.Rect(anchor_rect).y0
    r = fitz.Rect(anchor_rect)
    tx0 = r.x1 + pad + 6
    ty0 = r.y0 - pad
    tw, th = 160, 62
    text_rect = fitz.Rect(tx0, ty0, tx0 + tw, ty0 + th)
    if text_rect.x1 > page.rect.width or text_rect.y0 < 0:
        text_rect = fitz.Rect(r.x0 - pad - tw - 6, r.y1 + pad + 4, r.x0 - pad - 6, r.y1 + pad + 4 + th)
        # clamp fully inside the page as a last resort
        text_rect = text_rect & page.rect
        if text_rect.is_empty or text_rect.width < 40:
            text_rect = fitz.Rect(20, 20, 20 + tw, 20 + th)
    note(page, text_rect, text)


def apply_finding(doc, f):
    page_no = int(f.get("page", 0))
    if page_no < 1 or page_no > doc.page_count:
        return False
    page = doc[page_no - 1]
    wrong = str(f.get("wrong", "")).strip()
    correct = str(f.get("correct", ""))
    explain = str(f.get("explain", ""))
    msg = f"SALAH: {wrong}\nSEHARUSNYA: {correct}\n{explain}"

    hits = page.search_for(wrong) if wrong else []
    if hits:
        anchor = hits[0]
        circ = circle(page, anchor)
        strike(page, anchor)
        tx0 = circ.x1 + 6
        ty0 = circ.y0
        text_rect = fitz.Rect(tx0, ty0, tx0 + 155, ty0 + 58)
        if text_rect.x1 > page.rect.width:
            text_rect = fitz.Rect(circ.x0 - 155 - 6, circ.y1 + 4, circ.x0 - 6, circ.y1 + 4 + 58)
        note(page, text_rect, msg)
        return True

    # Fallback: value isn't searchable text on this page (vector-shaped CAD
    # dimension) — still surface it, don't drop the finding silently.
    banner = fitz.Rect(20, 20, 220, 20 + 62)
    note(page, banner, f"[Tidak ditemukan lokasi presisi]\n{msg}")
    return True


def main():
    if len(sys.argv) != 4:
        print("Usage: annotate.py <kontraktor.pdf> <findings.json> <output.pdf>", file=sys.stderr)
        sys.exit(2)
    src_path, findings_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
    try:
        with open(findings_path, "r", encoding="utf-8") as fh:
            findings = json.load(fh)
        doc = fitz.open(src_path)
        count = 0
        for f in findings:
            if apply_finding(doc, f):
                count += 1
        doc.save(out_path)
    except Exception as e:
        print(f"Gagal anotasi PDF: {e}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps({"ok": True, "annotated": count}))


if __name__ == "__main__":
    main()
