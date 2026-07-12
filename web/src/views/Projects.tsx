import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiDelete,
  apiGet,
  apiPatch,
  apiPost,
  type ProjectInfo,
  type TimeBucket,
} from "../api";

// mapDirLabel is the single translation point between the backend's dir
// value ("" = root) and the decks endpoint's display label ("(root)"). It's
// its own inverse, so the same call works in both directions.
function mapDirLabel(dir: string): string {
  if (dir === "") return "(root)";
  if (dir === "(root)") return "";
  return dir;
}

export default function Projects() {
  const navigate = useNavigate();
  const [showCreate, setShowCreate] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);

  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => apiGet<ProjectInfo[]>("/api/projects"),
  });
  const decks = useQuery({
    queryKey: ["decks"],
    queryFn: () => apiGet<TimeBucket[]>("/api/decks"),
  });

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Projects</h1>
        {!showCreate && (
          <button
            type="button"
            onClick={() => setShowCreate(true)}
            className="rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
          >
            + New project
          </button>
        )}
      </div>

      {showCreate && (
        <ProjectForm decks={decks.data ?? []} onDone={() => setShowCreate(false)} />
      )}

      {projects.isPending && <p className="text-sm text-zinc-500">Loading…</p>}
      {projects.isError && (
        <p className="text-sm text-red-600">{String(projects.error)}</p>
      )}

      {projects.data && projects.data.length === 0 && !showCreate && (
        <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center text-sm text-zinc-500 dark:border-zinc-700">
          No projects yet. Group note folders under a deadline to see focused
          progress and a daily new-card target.
        </div>
      )}

      <div className="space-y-3">
        {projects.data?.map((p) =>
          editingId === p.id ? (
            <ProjectForm
              key={p.id}
              decks={decks.data ?? []}
              initial={p}
              onDone={() => setEditingId(null)}
            />
          ) : (
            <ProjectCard
              key={p.id}
              project={p}
              onStudy={() => navigate(`/review?project=${p.id}`)}
              onEdit={() => setEditingId(p.id)}
            />
          ),
        )}
      </div>
    </div>
  );
}

function DeadlineLine({ project: p }: { project: ProjectInfo }) {
  if (!p.deadline || p.days_left === null) {
    return <p className="text-xs text-zinc-400">no deadline</p>;
  }
  if (p.days_left <= 0) {
    return (
      <p className="text-xs font-medium text-red-600 dark:text-red-400">
        {p.deadline} · deadline passed
      </p>
    );
  }
  if (p.days_left === 1) {
    return (
      <p className="text-xs font-medium text-amber-600 dark:text-amber-400">
        {p.deadline} · due today
      </p>
    );
  }
  return (
    <p className="text-xs text-zinc-500">
      {p.deadline} · {p.days_left} days left
    </p>
  );
}

function ProjectCard({
  project: p,
  onStudy,
  onEdit,
}: {
  project: ProjectInfo;
  onStudy: () => void;
  onEdit: () => void;
}) {
  const queryClient = useQueryClient();

  const del = useMutation({
    mutationFn: () => apiDelete(`/api/projects/${p.id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });

  return (
    <div className="space-y-2 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="font-medium">{p.name}</h2>
          <DeadlineLine project={p} />
        </div>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={onStudy}
            className="rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
          >
            Study now
          </button>
          <button
            type="button"
            onClick={onEdit}
            className="rounded-md border border-zinc-300 px-3 py-1.5 text-xs text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            Edit
          </button>
          <button
            type="button"
            disabled={del.isPending}
            onClick={() => {
              if (confirm(`Delete project "${p.name}"? Notes and cards are untouched.`))
                del.mutate();
            }}
            className="rounded-md border border-zinc-300 px-3 py-1.5 text-xs text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-zinc-700 dark:text-red-400 dark:hover:bg-red-950/30"
          >
            Delete
          </button>
        </div>
      </div>

      <div className="flex flex-wrap gap-1.5">
        {p.dirs.map((d) => (
          <span
            key={d}
            className="rounded-full bg-violet-100 px-2 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300"
          >
            {mapDirLabel(d)}
          </span>
        ))}
      </div>

      <p className="text-xs text-zinc-500">
        {p.total_cards} card{p.total_cards === 1 ? "" : "s"} · {p.new_cards} new ·{" "}
        {p.due_now} due now
      </p>
      {del.isError && <p className="text-sm text-red-600">{String(del.error)}</p>}
    </div>
  );
}

function ProjectForm({
  decks,
  initial,
  onDone,
}: {
  decks: TimeBucket[];
  initial?: ProjectInfo;
  onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(initial?.name ?? "");
  const [deadline, setDeadline] = useState(initial?.deadline ?? "");
  const [dirs, setDirs] = useState<Set<string>>(new Set(initial?.dirs ?? []));

  const toggleDir = (dir: string) => {
    setDirs((prev) => {
      const next = new Set(prev);
      if (next.has(dir)) next.delete(dir);
      else next.add(dir);
      return next;
    });
  };

  const save = useMutation({
    mutationFn: () => {
      const body = { name: name.trim(), dirs: [...dirs], deadline };
      return initial
        ? apiPatch<ProjectInfo>(`/api/projects/${initial.id}`, body)
        : apiPost<ProjectInfo>("/api/projects", body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      onDone();
    },
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (name.trim()) save.mutate();
      }}
      className="space-y-3 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900"
    >
      <div className="flex flex-wrap items-center gap-2">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Project name"
          className="w-56 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />
        <input
          type="date"
          value={deadline}
          onChange={(e) => setDeadline(e.target.value)}
          className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />
        {deadline && (
          <button
            type="button"
            onClick={() => setDeadline("")}
            className="text-xs text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200"
          >
            Clear date
          </button>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {decks.map((d) => (
          <label
            key={d.key}
            className="flex cursor-pointer items-center gap-1.5 rounded-full border border-zinc-300 px-2 py-1 text-xs text-zinc-600 dark:border-zinc-700 dark:text-zinc-300"
          >
            <input
              type="checkbox"
              checked={dirs.has(mapDirLabel(d.key))}
              onChange={() => toggleDir(mapDirLabel(d.key))}
            />
            {d.key}
          </label>
        ))}
      </div>

      <div className="flex items-center gap-2">
        <button
          type="submit"
          disabled={save.isPending || !name.trim()}
          className="rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          {save.isPending ? "Saving…" : initial ? "Save" : "Create project"}
        </button>
        <button
          type="button"
          onClick={onDone}
          className="rounded-md border border-zinc-300 px-3 py-1.5 text-xs text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          Cancel
        </button>
        {save.isError && (
          <span className="text-sm text-red-600">{String(save.error)}</span>
        )}
      </div>
    </form>
  );
}
