import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPatch,
  apiPost,
  type NoteSummary,
  type NoteType,
  type OpenQuestion,
  type ProjectInfo,
  type SearchResponse,
  type Stage,
  type SyncResult,
} from "../api";
import NoteTree from "../components/NoteTree";

const stageFilters: { value: Stage | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "skim", label: "Skim · to deepen" },
  { value: "deep", label: "Deep · ready to fold" },
  { value: "synthesis", label: "Synthesis" },
];

const typeFilters: { value: NoteType | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "reading", label: "Reading" },
  { value: "thought", label: "Thoughts" },
];

export const stageColors: Record<string, string> = {
  skim: "bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300",
  deep: "bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-300",
  synthesis:
    "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300",
};

export default function Notes() {
  const [filter, setFilter] = useState<Stage | "all">("all");
  const [typeFilter, setTypeFilter] = useState<NoteType | "all">("all");
  const [search, setSearch] = useState("");
  const queryClient = useQueryClient();

  const notes = useQuery({
    queryKey: ["notes", filter, typeFilter],
    queryFn: () => {
      const q = new URLSearchParams();
      if (filter !== "all") q.set("stage", filter);
      if (typeFilter !== "all") q.set("type", typeFilter);
      const qs = q.toString();
      return apiGet<NoteSummary[]>(qs ? `/api/notes?${qs}` : "/api/notes");
    },
  });

  const sync = useMutation({
    mutationFn: () => apiPost<SyncResult>("/api/sync"),
    onSuccess: () => queryClient.invalidateQueries(),
  });

  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => apiGet<ProjectInfo[]>("/api/projects"),
  });

  return (
    <div className="space-y-6">
      <input
        type="search"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder="Search notes (full text)…"
        className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />

      <div className="flex items-start gap-2">
        <div className="flex-1">
          <QuickCapture />
        </div>
        <Link
          to="/workbench?type=thought"
          className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          + New thought
        </Link>
      </div>

      {search.trim().length >= 2 ? (
        <SearchResults query={search.trim()} />
      ) : (
        <>
      <div className="flex flex-wrap items-center gap-2">
        {stageFilters.map((f) => (
          <button
            key={f.value}
            type="button"
            onClick={() => setFilter(f.value)}
            className={`rounded-full px-3 py-1 text-sm transition-colors ${
              filter === f.value
                ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900"
                : "bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700"
            }`}
          >
            {f.label}
          </button>
        ))}
        <span className="mx-1 text-zinc-300 dark:text-zinc-700">|</span>
        {typeFilters.map((f) => (
          <button
            key={f.value}
            type="button"
            onClick={() => setTypeFilter(f.value)}
            className={`rounded-full px-3 py-1 text-sm transition-colors ${
              typeFilter === f.value
                ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900"
                : "bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700"
            }`}
          >
            {f.label}
          </button>
        ))}
        <div className="flex-1" />
        <button
          type="button"
          onClick={() => sync.mutate()}
          disabled={sync.isPending}
          className="rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          {sync.isPending ? "Syncing…" : "Sync"}
        </button>
      </div>

      {sync.isSuccess && (
        <p className="text-sm text-zinc-500">
          Synced {sync.data.notes} notes: {sync.data.cards_created} new cards,{" "}
          {sync.data.cards_updated} updated, {sync.data.cards_orphaned}{" "}
          orphaned, {sync.data.anchors_written} anchors written.
        </p>
      )}
      {sync.isError && (
        <p className="text-sm text-red-600">{String(sync.error)}</p>
      )}

      {notes.isPending && <p className="text-sm text-zinc-500">Loading…</p>}
      {notes.isError && (
        <p className="text-sm text-red-600">{String(notes.error)}</p>
      )}

      {notes.data && notes.data.length === 0 && (
        <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center text-sm text-zinc-500 dark:border-zinc-700">
          No notes yet. Drop markdown files into the notes directory and hit
          Sync.
        </div>
      )}

      {notes.data && notes.data.length > 0 && (
        <NoteTree
          notes={notes.data}
          forceExpand={filter !== "all" || typeFilter !== "all"}
          projects={projects.data?.map((p) => ({ name: p.name, dirs: p.dirs }))}
          renderNote={(n) => <NoteRow note={n} hidePath />}
        />
      )}

      <OpenQuestions />
        </>
      )}
    </div>
  );
}

function NoteRow({ note: n, hidePath }: { note: NoteSummary; hidePath?: boolean }) {
  return (
    <Link
      to={`/notes/${n.path}`}
      className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
    >
      <span className="font-medium">{n.title}</span>
      {n.type === "thought" && (
        <span className="rounded bg-fuchsia-100 px-1.5 py-0.5 text-xs text-fuchsia-700 dark:bg-fuchsia-900/40 dark:text-fuchsia-300">
          thought
        </span>
      )}
      {n.stage && (
        <span
          className={`rounded-full px-2 py-0.5 text-xs ${stageColors[n.stage] ?? ""}`}
        >
          {n.stage}
        </span>
      )}
      {!hidePath && <span className="text-xs text-zinc-400">{n.path}</span>}
      <span className="ml-auto text-xs text-zinc-500">
        {n.card_count} card{n.card_count === 1 ? "" : "s"}
      </span>
    </Link>
  );
}

const snippetClass =
  "mt-1 text-sm text-zinc-500 [&_mark]:rounded-sm [&_mark]:bg-amber-200 [&_mark]:px-0.5 dark:[&_mark]:bg-amber-500/40 dark:[&_mark]:text-inherit";

function SearchResults({ query }: { query: string }) {
  const hits = useQuery({
    queryKey: ["search", query],
    queryFn: () =>
      apiGet<SearchResponse>(`/api/search?q=${encodeURIComponent(query)}`),
  });

  if (hits.isPending) return <p className="text-sm text-zinc-500">Searching…</p>;
  if (hits.isError) return <p className="text-sm text-red-600">{String(hits.error)}</p>;
  if (hits.data.notes.length === 0 && hits.data.sources.length === 0)
    return <p className="text-sm text-zinc-500">No matches for “{query}”.</p>;

  return (
    <div className="space-y-4">
      {hits.data.notes.length > 0 && (
        <ul className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 bg-white dark:divide-zinc-800 dark:border-zinc-800 dark:bg-zinc-900">
          {hits.data.notes.map((h) => (
            <li key={h.path}>
              <Link
                to={`/notes/${h.path}`}
                className="block px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
              >
                <span className="font-medium">{h.title}</span>
                <span className="ml-2 text-xs text-zinc-400">{h.path}</span>
                {/* snippet is server-generated: escaped text + <mark> only */}
                <p
                  className={snippetClass}
                  dangerouslySetInnerHTML={{ __html: h.snippet }}
                />
              </Link>
            </li>
          ))}
        </ul>
      )}

      {hits.data.sources.length > 0 && (
        <section>
          <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            In PDF sources
          </h2>
          <ul className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 bg-white dark:divide-zinc-800 dark:border-zinc-800 dark:bg-zinc-900">
            {hits.data.sources.map((h) => (
              <li key={h.source_id}>
                <Link
                  to={`/sources/${h.source_id}`}
                  className="block px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
                >
                  <span className="font-medium">📄 {h.title}</span>
                  <p
                    className={snippetClass}
                    dangerouslySetInnerHTML={{ __html: h.snippet }}
                  />
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

// QuickCapture appends a fleeting question to notes/inbox.md so nothing is
// lost when there is no editor at hand (couch, phone).
function QuickCapture() {
  const [text, setText] = useState("");
  const queryClient = useQueryClient();

  const capture = useMutation({
    mutationFn: (t: string) => apiPost("/api/capture", { text: t }),
    onSuccess: () => {
      setText("");
      queryClient.invalidateQueries({ queryKey: ["questions"] });
      queryClient.invalidateQueries({ queryKey: ["notes"] });
    },
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (text.trim()) capture.mutate(text.trim());
      }}
      className="flex gap-2"
    >
      <input
        type="text"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Quick capture: a question or thought → inbox.md"
        className="flex-1 rounded-md border border-dashed border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />
      <button
        type="submit"
        disabled={capture.isPending || !text.trim()}
        className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
      >
        {capture.isPending ? "Saving…" : "Capture"}
      </button>
      {capture.isError && (
        <span className="self-center text-sm text-red-600">{String(capture.error)}</span>
      )}
    </form>
  );
}

const questionActions = [
  { status: "carded", label: "→ carded", title: "turned into card(s)" },
  { status: "folded", label: "→ folded", title: "folded into an essay" },
  { status: "dropped", label: "✕", title: "drop this question" },
] as const;

function OpenQuestions() {
  const queryClient = useQueryClient();
  const questions = useQuery({
    queryKey: ["questions", "open"],
    queryFn: () => apiGet<OpenQuestion[]>("/api/questions?status=open"),
  });

  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: number; status: string }) =>
      apiPatch(`/api/questions/${id}`, { status }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["questions"] }),
  });

  if (!questions.data || questions.data.length === 0) return null;

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Open questions ({questions.data.length})
      </h2>
      <ul className="space-y-1">
        {questions.data.map((q) => (
          <li
            key={q.id}
            className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm dark:border-zinc-800 dark:bg-zinc-900"
          >
            <span>{q.text}</span>
            <Link
              to={`/notes/${q.note_path}`}
              className="text-xs text-zinc-400 hover:underline"
            >
              {q.note_path}
            </Link>
            <span className="ml-auto flex gap-1">
              {questionActions.map((a) => (
                <button
                  key={a.status}
                  type="button"
                  title={a.title}
                  disabled={setStatus.isPending}
                  onClick={() => setStatus.mutate({ id: q.id, status: a.status })}
                  className="rounded px-1.5 py-0.5 text-xs text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
                >
                  {a.label}
                </button>
              ))}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
