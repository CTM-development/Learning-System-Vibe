import { useEffect, useState } from "react";
import { scanPageCount, type Source } from "../api";

// Page viewer for a "scan" source: /api/sources/{id}/page/{n}, 1-based.
// Reused by SourceViewer (read view) and Workbench (transcription).
export default function ScanPager({ source }: { source: Source }) {
  const total = scanPageCount(source);
  const [page, setPage] = useState(1);

  useEffect(() => {
    setPage(1);
  }, [source.id]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Don't steal arrow keys from form fields (the Workbench editor
      // sits next to this pager).
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) {
        return;
      }
      if (e.key === "ArrowLeft") setPage((p) => Math.max(1, p - 1));
      else if (e.key === "ArrowRight") setPage((p) => Math.min(total, p + 1));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [total]);

  if (total === 0) {
    return <p className="text-sm text-zinc-500">No pages.</p>;
  }

  return (
    <div className="space-y-2">
      <img
        src={`/api/sources/${source.id}/page/${page}`}
        alt={`Page ${page}`}
        className="max-w-full rounded-lg border border-zinc-200 dark:border-zinc-800"
      />
      <div className="flex items-center justify-center gap-3">
        <button
          type="button"
          disabled={page <= 1}
          onClick={() => setPage((p) => Math.max(1, p - 1))}
          className="rounded-md border border-zinc-300 px-3 py-1 text-sm text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          ← Prev
        </button>
        <span className="text-sm tabular-nums text-zinc-500">
          page {page} / {total}
        </span>
        <button
          type="button"
          disabled={page >= total}
          onClick={() => setPage((p) => Math.min(total, p + 1))}
          className="rounded-md border border-zinc-300 px-3 py-1 text-sm text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          Next →
        </button>
      </div>
    </div>
  );
}
