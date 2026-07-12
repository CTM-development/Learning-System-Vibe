import { useState } from "react";
import type { ReactNode } from "react";
import type { NoteSummary } from "../api";

export type TreeDir = {
  name: string;
  path: string;
  dirs: TreeDir[];
  notes: NoteSummary[];
};

const STORAGE_KEY = "notesTreeExpanded";

function loadExpanded(): Set<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return new Set();
    return new Set(JSON.parse(raw) as string[]);
  } catch {
    return new Set();
  }
}

function saveExpanded(expanded: Set<string>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify([...expanded]));
  } catch {
    // storage unavailable (private browsing, quota, etc.) — state still
    // works for this session, it just won't persist.
  }
}

// buildTree groups notes by directory (everything before the last "/" in
// path, "" = root), creating intermediate directories even when they hold
// no notes directly, so e.g. "a/b/c/note.md" produces nested a > b > c.
function buildTree(notes: NoteSummary[]): TreeDir {
  const root: TreeDir = { name: "", path: "", dirs: [], notes: [] };
  const dirsByPath = new Map<string, TreeDir>([["", root]]);

  function getDir(path: string): TreeDir {
    const existing = dirsByPath.get(path);
    if (existing) return existing;
    const idx = path.lastIndexOf("/");
    const parentPath = idx === -1 ? "" : path.slice(0, idx);
    const name = idx === -1 ? path : path.slice(idx + 1);
    const parent = getDir(parentPath);
    const dir: TreeDir = { name, path, dirs: [], notes: [] };
    parent.dirs.push(dir);
    dirsByPath.set(path, dir);
    return dir;
  }

  for (const n of notes) {
    const idx = n.path.lastIndexOf("/");
    const dirPath = idx === -1 ? "" : n.path.slice(0, idx);
    const dir = dirPath === "" ? root : getDir(dirPath);
    dir.notes.push(n);
  }

  return root;
}

function countNotes(dir: TreeDir): number {
  return dir.notes.length + dir.dirs.reduce((sum, d) => sum + countNotes(d), 0);
}

function sortedDirs(dirs: TreeDir[]): TreeDir[] {
  return [...dirs].sort((a, b) => a.name.localeCompare(b.name));
}

// matchesProject mirrors "dir '' means root only" so a project rooted at ""
// never lights up every folder in the tree.
function matchesProject(dirPath: string, projectDirs: string[]): boolean {
  return projectDirs.some((d) =>
    d === "" ? dirPath === "" : dirPath === d || dirPath.startsWith(`${d}/`),
  );
}

interface NoteTreeProps {
  notes: NoteSummary[];
  renderNote: (n: NoteSummary) => ReactNode;
  forceExpand?: boolean;
  projects?: { name: string; dirs: string[] }[];
}

export default function NoteTree({
  notes,
  renderNote,
  forceExpand,
  projects,
}: NoteTreeProps) {
  const [expanded, setExpanded] = useState<Set<string>>(loadExpanded);

  function toggle(path: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      saveExpanded(next);
      return next;
    });
  }

  const root = buildTree(notes);

  return (
    <div className="rounded-lg border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <DirChildren
        dir={root}
        expanded={expanded}
        onToggle={toggle}
        forceExpand={forceExpand}
        projects={projects}
        renderNote={renderNote}
      />
    </div>
  );
}

interface ChildrenProps {
  dir: TreeDir;
  expanded: Set<string>;
  onToggle: (path: string) => void;
  forceExpand?: boolean;
  projects?: { name: string; dirs: string[] }[];
  renderNote: (n: NoteSummary) => ReactNode;
}

function DirChildren({
  dir,
  expanded,
  onToggle,
  forceExpand,
  projects,
  renderNote,
}: ChildrenProps) {
  const dirs = sortedDirs(dir.dirs).filter(
    (d) => !forceExpand || countNotes(d) > 0,
  );

  return (
    <div className="divide-y divide-zinc-200 dark:divide-zinc-800">
      {dirs.map((d) => (
        <DirNode
          key={d.path}
          dir={d}
          expanded={expanded}
          onToggle={onToggle}
          forceExpand={forceExpand}
          projects={projects}
          renderNote={renderNote}
        />
      ))}
      {dir.notes.map((n) => (
        <div key={n.path}>{renderNote(n)}</div>
      ))}
    </div>
  );
}

interface DirNodeProps extends ChildrenProps {
  dir: TreeDir;
}

function DirNode({
  dir,
  expanded,
  onToggle,
  forceExpand,
  projects,
  renderNote,
}: DirNodeProps) {
  const isExpanded = forceExpand || expanded.has(dir.path);
  const project = projects?.find((p) => matchesProject(dir.path, p.dirs));

  return (
    <div>
      <button
        type="button"
        onClick={() => onToggle(dir.path)}
        className="flex w-full items-center gap-2 px-4 py-2 text-left hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
      >
        <span
          className={`inline-block text-zinc-400 transition-transform ${isExpanded ? "rotate-90" : ""}`}
        >
          ▸
        </span>
        <span className="text-sm font-medium text-zinc-600 dark:text-zinc-300">
          {dir.name}
        </span>
        {project && (
          <span className="rounded-full bg-violet-100 px-2 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
            {project.name}
          </span>
        )}
        <span className="text-xs text-zinc-400">{countNotes(dir)}</span>
      </button>
      {isExpanded && (
        <div className="border-l border-zinc-200 pl-4 dark:border-zinc-800">
          <DirChildren
            dir={dir}
            expanded={expanded}
            onToggle={onToggle}
            forceExpand={forceExpand}
            projects={projects}
            renderNote={renderNote}
          />
        </div>
      )}
    </div>
  );
}
