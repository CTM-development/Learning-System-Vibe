import { useEffect } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, type NoteDetail } from "../api";
import { stageColors } from "./Notes";
import Markdown from "../Markdown";

const stages = ["skim", "deep", "synthesis"] as const;

// Accumulates the time this note is actually visible and reports it as one
// note_read event when the reader closes (or the tab hides for good).
function useReadTimer(path: string) {
  useEffect(() => {
    if (!path) return;
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
          body: JSON.stringify({
            kind: "note_read",
            ref: path,
            elapsed_ms: accumulated,
          }),
        });
      }
    };
  }, [path]);
}

export default function NoteReader() {
  const { "*": path = "" } = useParams();
  const queryClient = useQueryClient();
  useReadTimer(path);

  const note = useQuery({
    queryKey: ["note", path],
    queryFn: () => apiGet<NoteDetail>(`/api/notes/${path}`),
    enabled: path !== "",
  });

  const setStage = useMutation({
    mutationFn: (stage: string) =>
      apiPost<NoteDetail>("/api/notes/stage", { path, stage }),
    onSuccess: (updated) => {
      queryClient.setQueryData(["note", path], updated);
      queryClient.invalidateQueries({ queryKey: ["notes"] });
    },
  });

  if (note.isPending) {
    return <p className="text-sm text-zinc-500">Loading…</p>;
  }
  if (note.isError) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-red-600">{String(note.error)}</p>
        <Link to="/notes" className="text-sm text-zinc-500 hover:underline">
          ← Back to notes
        </Link>
      </div>
    );
  }

  const n = note.data;
  // Strip frontmatter from the rendered view; it is shown as chips instead.
  const body = n.content.replace(/^---\n[\s\S]*?\n---\n?/, "");

  return (
    <article className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Link to="/notes" className="text-sm text-zinc-500 hover:underline">
          ← Notes
        </Link>
        <span className="text-xs text-zinc-400">{n.path}</span>
        <div className="flex-1" />
        {stages.map((s) => (
          <button
            key={s}
            type="button"
            disabled={setStage.isPending || n.stage === s}
            onClick={() => setStage.mutate(s)}
            className={`rounded-full px-3 py-1 text-xs transition-colors ${
              n.stage === s
                ? (stageColors[s] ?? "") + " font-semibold"
                : "bg-zinc-100 text-zinc-500 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700"
            }`}
            title={n.stage === s ? "current stage" : `move to ${s}`}
          >
            {s}
          </button>
        ))}
      </div>

      {setStage.isError && (
        <p className="text-sm text-red-600">{String(setStage.error)}</p>
      )}

      {(n.tags.length > 0 || n.sources.length > 0) && (
        <div className="flex flex-wrap gap-1.5 text-xs">
          {n.tags.map((t) => (
            <span
              key={t}
              className="rounded bg-zinc-100 px-1.5 py-0.5 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
            >
              #{t}
            </span>
          ))}
          {n.sources.map((s) => (
            <span
              key={s}
              className="rounded bg-violet-100 px-1.5 py-0.5 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300"
              title="cited source"
            >
              ⌘ {s}
            </span>
          ))}
        </div>
      )}

      <div className="prose prose-zinc max-w-none rounded-lg border border-zinc-200 bg-white p-6 dark:prose-invert dark:border-zinc-800 dark:bg-zinc-900">
        <Markdown>{body}</Markdown>
      </div>

      <p className="text-xs text-zinc-400">
        {n.card_count} card{n.card_count === 1 ? "" : "s"} from this note
      </p>
    </article>
  );
}
