import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPost,
  type AcceptResponse,
  type GenerateResponse,
  type LLMModel,
  type LLMStatus,
  type NoteSummary,
  type ProposedCard,
} from "../api";

type Proposal = ProposedCard & { accepted: boolean };

export default function Generate() {
  const queryClient = useQueryClient();
  const [notePath, setNotePath] = useState("");
  const [model, setModel] = useState("");
  const [count, setCount] = useState(8);
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [usedModel, setUsedModel] = useState("");

  const status = useQuery({
    queryKey: ["llm", "status"],
    queryFn: () => apiGet<LLMStatus>("/api/llm/status"),
  });
  const notes = useQuery({
    queryKey: ["notes", "all"],
    queryFn: () => apiGet<NoteSummary[]>("/api/notes"),
  });
  const models = useQuery({
    queryKey: ["llm", "models"],
    queryFn: () => apiGet<LLMModel[]>("/api/llm/models"),
    enabled: status.data?.configured === true,
    staleTime: 60 * 60 * 1000,
    retry: 0,
  });

  const generate = useMutation({
    mutationFn: () =>
      apiPost<GenerateResponse>("/api/llm/generate-cards", {
        note_path: notePath,
        model: model || undefined,
        count,
      }),
    onSuccess: (res) => {
      setProposals(res.cards.map((c) => ({ ...c, accepted: true })));
      setUsedModel(res.model);
      queryClient.invalidateQueries({ queryKey: ["llm", "status"] });
    },
  });

  const accept = useMutation({
    mutationFn: () =>
      apiPost<AcceptResponse>("/api/llm/accept-cards", {
        note_path: notePath,
        model: usedModel,
        cards: proposals
          .filter((p) => p.accepted)
          .map(({ front, back }) => ({ front, back })),
      }),
    onSuccess: () => {
      setProposals([]);
      queryClient.invalidateQueries();
    },
  });

  if (status.isPending) return <p className="text-sm text-zinc-500">Loading…</p>;
  if (status.isError) return <p className="text-sm text-red-600">{String(status.error)}</p>;
  const s = status.data;

  if (!s.configured) {
    return (
      <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center dark:border-zinc-700">
        <h1 className="text-xl font-semibold">OpenRouter not configured</h1>
        <p className="mx-auto mt-3 max-w-md text-sm text-zinc-500">
          Add your API key to the server config to enable card generation:
          set{" "}
          <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">
            openrouter_api_key
          </code>{" "}
          in config.yaml (or the{" "}
          <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">
            LEARN_OPENROUTER_API_KEY
          </code>{" "}
          environment variable) and restart the server. The key never leaves
          the server.
        </p>
      </div>
    );
  }

  const acceptedCount = proposals.filter((p) => p.accepted).length;
  const budgetPct = Math.min(100, Math.round((s.tokens_today / s.daily_tokens) * 100));

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end gap-3 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
        <label className="flex flex-col gap-1 text-xs text-zinc-500">
          Note
          <select
            value={notePath}
            onChange={(e) => setNotePath(e.target.value)}
            className="min-w-64 rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
          >
            <option value="">Choose a note…</option>
            {notes.data?.map((n) => (
              <option key={n.path} value={n.path}>
                {n.title} — {n.path}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-zinc-500">
          Model
          <input
            list="llm-models"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder={s.model}
            className="w-72 rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
          />
          <datalist id="llm-models">
            {models.data?.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </datalist>
        </label>
        <label className="flex flex-col gap-1 text-xs text-zinc-500">
          Max cards
          <input
            type="number"
            min={1}
            max={20}
            value={count}
            onChange={(e) => setCount(Number(e.target.value))}
            className="w-20 rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
          />
        </label>
        <button
          type="button"
          disabled={!notePath || generate.isPending}
          onClick={() => generate.mutate()}
          className="rounded-md bg-zinc-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          {generate.isPending ? "Generating…" : "Generate cards"}
        </button>
      </div>

      <p className="text-xs text-zinc-500">
        Budget today: {s.tokens_today.toLocaleString()} /{" "}
        {s.daily_tokens.toLocaleString()} tokens ({budgetPct}%)
        {s.cost_today > 0 && <> · ${s.cost_today.toFixed(4)}</>}
      </p>

      {generate.isError && (
        <p className="text-sm text-red-600">{String(generate.error)}</p>
      )}
      {accept.isError && <p className="text-sm text-red-600">{String(accept.error)}</p>}
      {accept.isSuccess && (
        <p className="text-sm text-emerald-600 dark:text-emerald-400">
          Added {accept.data.added} card{accept.data.added === 1 ? "" : "s"} to{" "}
          <Link to={`/notes/${notePath}`} className="underline">
            {notePath}
          </Link>{" "}
          — anchors written, scheduling starts with the next queue.
        </p>
      )}

      {proposals.length > 0 && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
              Proposals — review, edit, accept
            </h2>
            <button
              type="button"
              disabled={acceptedCount === 0 || accept.isPending}
              onClick={() => accept.mutate()}
              className="rounded-md bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:opacity-50"
            >
              {accept.isPending
                ? "Writing…"
                : `Add ${acceptedCount} card${acceptedCount === 1 ? "" : "s"} to note`}
            </button>
          </div>

          {proposals.map((p, i) => (
            <div
              key={i}
              className={`space-y-2 rounded-lg border p-4 transition-opacity ${
                p.accepted
                  ? "border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900"
                  : "border-dashed border-zinc-300 opacity-50 dark:border-zinc-700"
              }`}
            >
              <label className="flex items-center gap-2 text-xs text-zinc-500">
                <input
                  type="checkbox"
                  checked={p.accepted}
                  onChange={(e) =>
                    setProposals((ps) =>
                      ps.map((q, j) => (j === i ? { ...q, accepted: e.target.checked } : q)),
                    )
                  }
                />
                accept
              </label>
              <textarea
                value={p.front}
                rows={2}
                onChange={(e) =>
                  setProposals((ps) =>
                    ps.map((q, j) => (j === i ? { ...q, front: e.target.value } : q)),
                  )
                }
                className="w-full rounded-md border border-zinc-300 bg-white px-2 py-1.5 font-medium dark:border-zinc-700 dark:bg-zinc-950"
              />
              <textarea
                value={p.back}
                rows={3}
                onChange={(e) =>
                  setProposals((ps) =>
                    ps.map((q, j) => (j === i ? { ...q, back: e.target.value } : q)),
                  )
                }
                className="w-full rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-950"
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
