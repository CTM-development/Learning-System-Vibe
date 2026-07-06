import { useEffect } from "react";

// Accumulates the time a view is actually visible and reports it as one
// activity event when the view closes (or the tab hides for good). Used by
// the note reader (note_read) and the PDF viewer (pdf_read).
export function useReadTimer(kind: string, ref: string) {
  useEffect(() => {
    if (!ref) return;
    let visibleSince: number | null = document.hidden ? null : Date.now();
    let accumulated = 0;

    const onVisibility = () => {
      if (document.hidden && visibleSince !== null) {
        accumulated += Date.now() - visibleSince;
        visibleSince = null;
      } else if (!document.hidden && visibleSince === null) {
        visibleSince = Date.now();
      }
    };
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      if (visibleSince !== null) accumulated += Date.now() - visibleSince;
      if (accumulated >= 3000) {
        void fetch("/api/events", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          keepalive: true,
          body: JSON.stringify({ kind, ref, elapsed_ms: accumulated }),
        });
      }
    };
  }, [kind, ref]);
}
