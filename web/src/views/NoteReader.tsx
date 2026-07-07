import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, type NoteDetail, type Source } from "../api";
import { stageColors } from "./Notes";
import Markdown from "../Markdown";
import { useReadTimer } from "../useReadTimer";

const stages = ["skim", "deep", "synthesis"] as const;

export default function NoteReader() {
  const { "*": path = "" } = useParams();
  const queryClient = useQueryClient();
  useReadTimer("note_read", path);

  const allSources = useQuery({
    queryKey: ["sources"],
    queryFn: () => apiGet<Source[]>("/api/sources"),
  });
  const sourceByKey = new Map(
    (allSources.data ?? []).map((s) => [s.key, s]),
  );

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
  // Reading view: frontmatter is shown as chips instead; srs ID anchors are
  // plumbing; cloze markers collapse to their visible text; wikilinks become
  // real links — to the note when resolved, to wiki generation when red.
  const linkTargets = new Map(n.links.map((l) => [l.target, l.to_path]));
  const body = n.content
    .replace(/^---\n[\s\S]*?\n---\n?/, "")
    .replace(/\s*<!--\s*srs:[0-9a-f]+\s*-->/g, "")
    .replace(/\{\{c\d+::(.*?)(?:::.*?)?\}\}/g, "$1")
    .replace(
      /\[\[([^[\]|]+)(?:\|([^[\]]*))?\]\]/g,
      (whole, target: string, label?: string) => {
        const t = target.trim();
        const text = (label ?? "").trim() || t;
        const to = linkTargets.get(t);
        if (to) return `[${text}](/notes/${to})`;
        if (to === "") return `[${text}](/wiki?topic=${encodeURIComponent(t)})`;
        return whole; // inside a code fence — parser didn't record it
      },
    );
  const assetBase = path.includes("/")
    ? path.slice(0, path.lastIndexOf("/") + 1)
    : "";

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
          {n.sources.map((key) => {
            const src = sourceByKey.get(key);
            const chip = (
              <span
                className={`rounded bg-violet-100 px-1.5 py-0.5 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300 ${src ? "hover:bg-violet-200 dark:hover:bg-violet-900/70" : ""}`}
                title={src ? src.title : "cited source (not uploaded yet)"}
              >
                📄 {key}
              </span>
            );
            return src ? (
              <Link key={key} to={`/sources/${src.id}`}>
                {chip}
              </Link>
            ) : (
              <span key={key}>{chip}</span>
            );
          })}
        </div>
      )}

      <div className="prose prose-zinc max-w-none rounded-lg border border-zinc-200 bg-white p-6 dark:prose-invert dark:border-zinc-800 dark:bg-zinc-900">
        <Markdown assetBase={assetBase}>{body}</Markdown>
      </div>

      {n.backlinks.length > 0 && (
        <section className="rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
          <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">
            Linked from
          </h2>
          <ul className="flex flex-wrap gap-2 text-sm">
            {n.backlinks.map((b) => (
              <li key={b.path}>
                <Link
                  to={`/notes/${b.path}`}
                  className="rounded bg-zinc-100 px-2 py-1 text-zinc-700 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
                >
                  {b.title}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <p className="text-xs text-zinc-400">
        {n.card_count} card{n.card_count === 1 ? "" : "s"} from this note
      </p>
    </article>
  );
}
