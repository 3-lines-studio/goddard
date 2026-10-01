export type Line = {
  id: string;
  title: string;
  source: string;
  running: boolean;
};

export type Project = {
  id: string;
  slug: string;
  name: string;
  conversations: Line[];
};

export type Task = {
  name: string;
  project: string;
  when: string;
  at: string;
  every: string;
  prompt: string;
  paused: boolean;
  silent: boolean;
  target: string;
  unread: number;
  runs: { ts: number; date: string; ms: number; ok: boolean; text: string }[];
};

export type Machine = {
  memory: { used: number; total: number; anon: number; cache: number; kernel: number };
  disk: { used: number; total: number };
  processes: number;
};

export type Fact = {
  key: string;
  kind: string;
  body: string;
  project: string;
  date: number;
};

export type Skill = {
  name: string;
  description: string;
  owner: string;
  role: string;
  updated: number;
};

export type Tab = {
  key: string;
  kind: "thread" | "agenda" | "memoria";
  title: string;
  slug: string;
};

export type Event = {
  event: string;
  text?: string;
  name?: string;
  mime?: string;
  caption?: string;
  args?: string;
  id?: string;
  ms?: number;
  message?: string;
};

export type Upload = {
  id: string;
  name: string;
  mime: string;
};

export const VISIBLE = 10;
