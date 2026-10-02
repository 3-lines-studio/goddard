export type Line = {
  id: string;
  title: string;
  source: string;
  running: boolean;
};

export type Owner = {
  kind: string;
  id: string;
};

export type Project = {
  id: string;
  slug: string;
  name: string;
  owner: Owner;
  workspace: string;
  conversations: Line[];
};

export type Org = {
  id: string;
  slug: string;
  name: string;
  role: string;
};

export type Member = {
  user_id: string;
  email: string;
  name: string;
  role: string;
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
  kind: "thread" | "agenda" | "memoria" | "orgs" | "sandbox" | "model";
  title: string;
  slug: string;
};

// Sandbox is the workspace of an owner as the app hands it over: where its
// projects live and how to reach the machine that runs the tools over them.
// The key never comes back, only whether there is one.
export type Sandbox = {
  owner: Owner;
  path: string;
  addr: string;
  user: string;
  has_key: boolean;
};

// SandboxInput is what the panel sends: the same, and the key only when
// somebody typed a new one.
export type SandboxInput = {
  path: string;
  addr: string;
  user: string;
  key: string;
};

// SandboxCheck is what the panel gets when it asks a machine if it answers.
export type SandboxCheck = {
  ok: boolean;
  message: string;
};

// Model is the model of an owner as the app hands it over: the one it brought,
// or nothing when it answers with the one of the house, whose name always
// comes. The key never comes back, only whether there is one.
export type Model = {
  owner: Owner;
  own: boolean;
  base: string;
  name: string;
  has_key: boolean;
  house: string;
};

// ModelInput is what the panel sends: the same, and the key only when somebody
// typed a new one.
export type ModelInput = {
  base: string;
  name: string;
  key: string;
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
