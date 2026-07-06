import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPost,
  type Health,
  type Session,
  type SessionsResponse,
} from "./api";
import Notes from "./views/Notes";
import NoteReader from "./views/NoteReader";
import Review from "./views/Review";

const views = [
  { path: "/review", label: "Review" },
  { path: "/notes", label: "Notes" },
  { path: "/sources", label: "Sources" },
  { path: "/cards", label: "Cards" },
  { path: "/stats", label: "Stats" },
] as const;

export default function App() {
  return (
    <div className="min-h-screen bg-zinc-50 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
      <Header />
      <main className="mx-auto max-w-4xl px-4 py-6">
        <Routes>
          <Route path="/" element={<Navigate to="/review" replace />} />
          <Route path="/review" element={<Review />} />
          <Route path="/notes" element={<Notes />} />
          <Route path="/notes/*" element={<NoteReader />} />
          {views
            .filter((v) => v.path !== "/notes" && v.path !== "/review")
            .map((v) => (
              <Route
                key={v.path}
                path={v.path}
                element={<Placeholder label={v.label} />}
              />
            ))}
        </Routes>
      </main>
    </div>
  );
}

function Header() {
  const health = useQuery({
    queryKey: ["health"],
    queryFn: () => apiGet<Health>("/api/health"),
  });

  return (
    <header className="border-b border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div className="mx-auto flex max-w-4xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
        <span className="text-lg font-semibold tracking-tight">Learning</span>
        <nav className="flex flex-1 flex-wrap gap-1">
          {views.map((v) => (
            <NavLink
              key={v.path}
              to={v.path}
              className={({ isActive }) =>
                `rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900"
                    : "text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
                }`
              }
            >
              {v.label}
            </NavLink>
          ))}
        </nav>
        <SessionToggle />
        <span
          className="text-xs text-zinc-400"
          title={health.data ? `server ${health.data.version}` : "server unreachable"}
        >
          {health.isSuccess ? "●" : health.isError ? "○" : "…"}
        </span>
      </div>
    </header>
  );
}

// Session start/stop toggle with a live timer. While a session runs, every
// logged event is attributed to it server-side.
function SessionToggle() {
  const queryClient = useQueryClient();
  const [showPicker, setShowPicker] = useState(false);

  const sessions = useQuery({
    queryKey: ["sessions"],
    queryFn: () => apiGet<SessionsResponse>("/api/sessions"),
    refetchInterval: 60_000,
  });
  const active = sessions.data?.active ?? null;

  const start = useMutation({
    mutationFn: (kind: Session["kind"]) =>
      apiPost<Session>("/api/sessions/start", { kind }),
    onSuccess: () => {
      setShowPicker(false);
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
  const stop = useMutation({
    mutationFn: () => apiPost<Session>("/api/sessions/stop"),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["sessions"] }),
  });

  if (active) {
    return (
      <button
        type="button"
        onClick={() => stop.mutate()}
        className="flex items-center gap-2 rounded-md bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-500"
        title={`stop ${active.kind} session`}
      >
        <span className="h-2 w-2 animate-pulse rounded-full bg-white" />
        {active.kind === "learning" ? "Learning" : "Productivity"}
        <LiveTimer since={active.started_at} />
      </button>
    );
  }

  if (showPicker) {
    return (
      <span className="flex gap-1">
        <button
          type="button"
          onClick={() => start.mutate("learning")}
          className="rounded-md bg-emerald-600 px-2.5 py-1.5 text-sm text-white hover:bg-emerald-500"
        >
          Learning
        </button>
        <button
          type="button"
          onClick={() => start.mutate("productivity")}
          className="rounded-md bg-sky-600 px-2.5 py-1.5 text-sm text-white hover:bg-sky-500"
        >
          Productivity
        </button>
        <button
          type="button"
          onClick={() => setShowPicker(false)}
          className="rounded-md px-2 py-1.5 text-sm text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-800"
        >
          ✕
        </button>
      </span>
    );
  }

  return (
    <button
      type="button"
      onClick={() => setShowPicker(true)}
      className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
    >
      Start session
    </button>
  );
}

function LiveTimer({ since }: { since: string }) {
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);
  const secs = Math.max(0, Math.floor((Date.now() - new Date(since).getTime()) / 1000));
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    <span className="tabular-nums text-white/80">
      {h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`}
    </span>
  );
}

function Placeholder({ label }: { label: string }) {
  return (
    <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center dark:border-zinc-700">
      <h1 className="text-xl font-semibold">{label}</h1>
      <p className="mt-2 text-sm text-zinc-500">
        This view arrives in a later milestone.
      </p>
    </div>
  );
}
