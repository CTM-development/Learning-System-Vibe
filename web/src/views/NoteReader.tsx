import { useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPost,
  type ChatMessage,
  type LLMStatus,
  type NoteDetail,
  type Source,
  type TutorResponse,
} from "../api";
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

  const llmStatus = useQuery({
    queryKey: ["llm", "status"],
    queryFn: () => apiGet<LLMStatus>("/api/llm/status"),
  });
  const [tutorOpen, setTutorOpen] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");

  // The reader doesn't remount between notes (same route pattern) — the
  // tutor transcript must not leak from one note into the next.
  useEffect(() => {
    setTutorOpen(false);
    setMessages([]);
    setInput("");
  }, [path]);

  const tutor = useMutation({
    mutationFn: (transcript: ChatMessage[]) =>
      apiPost<TutorResponse>("/api/llm/tutor", {
        note_path: path,
        messages: transcript,
      }),
    onSuccess: (res) => {
      setMessages((m) => [...m, { role: "assistant", content: res.reply }]);
    },
  });

  const sendMessage = (e: FormEvent) => {
    e.preventDefault();
    if (!input.trim() || tutor.isPending) return;
    const next: ChatMessage[] = [...messages, { role: "user", content: input }];
    setMessages(next);
    setInput("");
    tutor.mutate(next);
  };

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

      {llmStatus.data?.configured &&
        (tutorOpen ? (
          <section className="space-y-3 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
            <div className="flex items-center justify-between">
              <h2 className="text-xs font-semibold uppercase tracking-wide text-zinc-500">
                Tutor
              </h2>
              <button
                type="button"
                onClick={() => setTutorOpen(false)}
                className="text-xs text-zinc-400 hover:underline"
              >
                Close
              </button>
            </div>

            {messages.length > 0 && (
              <div className="space-y-2">
                {messages.map((m, i) =>
                  m.role === "user" ? (
                    <div key={i} className="flex justify-end">
                      <div className="max-w-[80%] rounded-lg bg-zinc-100 px-3 py-2 text-sm dark:bg-zinc-800">
                        {m.content}
                      </div>
                    </div>
                  ) : (
                    <div
                      key={i}
                      className="prose prose-zinc max-w-none text-sm dark:prose-invert"
                    >
                      <Markdown>{m.content}</Markdown>
                    </div>
                  ),
                )}
              </div>
            )}

            {tutor.isPending && (
              <p className="text-xs text-zinc-500">Tutor is thinking…</p>
            )}
            {tutor.isError && (
              <p className="text-sm text-red-600">
                {String(tutor.error)}{" "}
                <button
                  type="button"
                  onClick={() => tutor.mutate(messages)}
                  className="underline"
                >
                  Retry
                </button>
              </p>
            )}

            <form onSubmit={sendMessage} className="flex items-center gap-2">
              <input
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                disabled={tutor.isPending}
                placeholder="Ask a question about this note…"
                className="flex-1 rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 disabled:opacity-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
              />
              <button
                type="submit"
                disabled={tutor.isPending || !input.trim()}
                className="rounded-md bg-zinc-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
              >
                Send
              </button>
            </form>

            <p className="text-xs text-zinc-400">
              Grounded in this note · hints before answers
            </p>
          </section>
        ) : (
          <button
            type="button"
            onClick={() => setTutorOpen(true)}
            className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            💬 Ask the tutor about this note
          </button>
        ))}
    </article>
  );
}
