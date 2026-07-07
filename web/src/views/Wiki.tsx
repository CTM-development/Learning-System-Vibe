import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  apiGet,
  apiPost,
  type LLMModel,
  type LLMStatus,
  type NoteSummary,
  type WikiGenerateResponse,
} from "../api";

// Karpathy-style study wiki: ask for a topic, get an article grounded in
// your own notes and sources, saved as an ordinary note under notes/wiki/.
// Red [[wikilinks]] all over the app land here with the topic prefilled.
export default function Wiki() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [topic, setTopic] = useState(params.get("topic") ?? "");
  const [model, setModel] = useState("");

  const status = useQuery({
    queryKey: ["llm", "status"],
    queryFn: () => apiGet<LLMStatus>("/api/llm/status"),
  });
  const models = useQuery({
    queryKey: ["llm", "models"],
    queryFn: () => apiGet<LLMModel[]>("/api/llm/models"),
    enabled: status.data?.configured === true,
    staleTime: 60 * 60 * 1000,
    retry: 0,
  });
  const notes = useQuery({
    queryKey: ["notes", "all"],
    queryFn: () => apiGet<NoteSummary[]>("/api/notes"),
  });
  const articles = (notes.data ?? []).filter((n) => n.path.startsWith("wiki/"));

  const generate = useMutation({
    mutationFn: () =>
      apiPost<WikiGenerateResponse>("/api/wiki/generate", {
        topic,
        model: model || undefined,
      }),
    onSuccess: (res) => navigate(`/notes/${res.path}`),
  });

  if (status.isPending) return <p className="text-sm text-zinc-500">Loading…</p>;
  if (status.isError) return <p className="text-sm text-red-600">{String(status.error)}</p>;
  const s = status.data;

  return (
    <div className="space-y-6">
      {s.configured ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (topic.trim() && !generate.isPending) generate.mutate();
          }}
          className="space-y-3 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900"
        >
          <div className="flex flex-wrap items-end gap-3">
            <label className="flex flex-1 flex-col gap-1 text-xs text-zinc-500">
              Topic
              <input
                type="text"
                value={topic}
                onChange={(e) => setTopic(e.target.value)}
                placeholder="e.g. KL divergence"
                className="min-w-64 rounded-md border border-zinc-300 bg-white px-3 py-2 text-base text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-zinc-500">
              Model
              <input
                list="llm-models"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder={s.model}
                className="w-64 rounded-md border border-zinc-300 bg-white px-2 py-2 text-sm text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
              />
              <datalist id="llm-models">
                {models.data?.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </datalist>
            </label>
            <button
              type="submit"
              disabled={!topic.trim() || generate.isPending}
              className="rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
            >
              {generate.isPending ? "Writing article…" : "Generate article"}
            </button>
          </div>
          <p className="text-xs text-zinc-500">
            Grounded in your notes and PDF text via search; the article marks
            what comes from your notes vs. general knowledge. Budget today:{" "}
            {s.tokens_today.toLocaleString()} / {s.daily_tokens.toLocaleString()}{" "}
            tokens.
          </p>
          {generate.isError && (
            <p className="text-sm text-red-600">{String(generate.error)}</p>
          )}
        </form>
      ) : (
        <div className="rounded-lg border border-dashed border-zinc-300 p-6 text-sm text-zinc-500 dark:border-zinc-700">
          Configure <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">openrouter_api_key</code>{" "}
          to generate wiki articles. Existing articles below stay readable.
        </div>
      )}

      <section>
        <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Articles ({articles.length})
        </h2>
        {articles.length === 0 ? (
          <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center text-sm text-zinc-500 dark:border-zinc-700">
            No wiki articles yet. Generate one above, or click a red{" "}
            <span className="text-red-600 dark:text-red-400">[[wikilink]]</span>{" "}
            in any note.
          </div>
        ) : (
          <ul className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 bg-white dark:divide-zinc-800 dark:border-zinc-800 dark:bg-zinc-900">
            {articles.map((n) => (
              <li key={n.path}>
                <Link
                  to={`/notes/${n.path}`}
                  className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
                >
                  <span className="font-medium">{n.title}</span>
                  <span className="rounded bg-violet-100 px-1.5 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
                    generated
                  </span>
                  <span className="ml-auto text-xs text-zinc-500">
                    {n.card_count} card{n.card_count === 1 ? "" : "s"}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
