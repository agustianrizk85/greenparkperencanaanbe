#!/usr/bin/env python3
"""Render every page of two PDFs (GK Kontraktor + GK TTD) to PNG images, so the
Go backend (internal/service/gkcheck.go) can send them to an Ollama vision
model for comparison.

Usage:
    python render_pages.py <kontraktor.pdf> <ttd.pdf> <outdir>

Prints a single JSON object to stdout on success:
    {"kontraktor": {"pages": N, "images": ["<outdir>/k_01.png", ...]},
     "ttd":         {"pages": M, "images": ["<outdir>/t_01.png", ...]}}

On failure, prints an error message to stderr and exits non-zero.

Requires: pip install pymupdf
"""
import json
import sys

try:
    import fitz  # PyMuPDF
except ImportError:
    print("PyMuPDF belum terpasang. Jalankan: pip install pymupdf", file=sys.stderr)
    sys.exit(1)

ZOOM = 2.5  # ~180 DPI equivalent — cukup untuk baca teks/dimensi halus.


def render(src_path, outdir, prefix):
    doc = fitz.open(src_path)
    mat = fitz.Matrix(ZOOM, ZOOM)
    images = []
    for i in range(doc.page_count):
        page = doc[i]
        pix = page.get_pixmap(matrix=mat)
        out_path = f"{outdir}/{prefix}_{i+1:03d}.png"
        pix.save(out_path)
        images.append(out_path)
    return doc.page_count, images


def render_or_skip(path, outdir, prefix):
    """Render a PDF, or return an empty side when path is "-" / empty (single-doc
    mode, where only one of Kontraktor / TTD was uploaded)."""
    if not path or path == "-":
        return 0, []
    return render(path, outdir, prefix)


def main():
    if len(sys.argv) != 4:
        print("Usage: render_pages.py <kontraktor.pdf|-> <ttd.pdf|-> <outdir>", file=sys.stderr)
        sys.exit(2)
    kontraktor_path, ttd_path, outdir = sys.argv[1], sys.argv[2], sys.argv[3]
    try:
        k_pages, k_images = render_or_skip(kontraktor_path, outdir, "k")
        t_pages, t_images = render_or_skip(ttd_path, outdir, "t")
    except Exception as e:
        print(f"Gagal render PDF: {e}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps({
        "kontraktor": {"pages": k_pages, "images": k_images},
        "ttd": {"pages": t_pages, "images": t_images},
    }))


if __name__ == "__main__":
    main()
