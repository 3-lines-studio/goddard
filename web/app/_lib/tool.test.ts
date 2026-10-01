import { test, expect } from "bun:test";
import { describeTool } from "./tool";

const work = "/data/workspace";

test("el cd de arranque sale del comando y queda como directorio", () => {
  expect(describeTool("bash", `{"command":"cd ${work}/projects/jimmy && git status"}`, work)).toEqual({ dir: "projects/jimmy", text: "git status" });
});

test("el directorio se lee aunque el cd silencie el error", () => {
  expect(describeTool("bash", `{"command":"cd ${work}/projects/jimmy 2>/dev/null && make test"}`, work)).toEqual({ dir: "projects/jimmy", text: "make test" });
});

test("el cd a la raíz del workspace es una tilde", () => {
  expect(describeTool("bash", `{"command":"cd ${work} && ls"}`, work)).toEqual({
    dir: "~",
    text: "ls",
  });
});

test("el cd fuera del workspace queda entero", () => {
  expect(describeTool("bash", `{"command":"cd /tmp && ls"}`, work)).toEqual({
    dir: "/tmp",
    text: "ls",
  });
});

test("varios cd seguidos dejan el último, que es donde corre el resto", () => {
  expect(describeTool("bash", `{"command":"cd /tmp && cd ${work}/projects/axe && git log"}`, work)).toEqual({ dir: "projects/axe", text: "git log" });
});

test("un comando sin cd va entero y sin directorio", () => {
  expect(describeTool("bash", '{"command":"node --test web/"}', work)).toEqual({
    dir: "",
    text: "node --test web/",
  });
});

test("un cd suelto, sin nada encadenado, no se toca", () => {
  expect(describeTool("bash", '{"command":"cd /tmp"}', work)).toEqual({
    dir: "",
    text: "cd /tmp",
  });
});

test("sin workspace el directorio queda como vino", () => {
  expect(describeTool("bash", `{"command":"cd ${work}/projects/jimmy && git status"}`, "")).toEqual({ dir: `${work}/projects/jimmy`, text: "git status" });
});

test("el path de read, write y edit también se acorta", () => {
  const args = `{"path":"${work}/projects/jimmy/src/web.rs"}`;
  for (const name of ["read", "write", "edit"]) {
    expect(describeTool(name, args, work)).toEqual({
      dir: "",
      text: "projects/jimmy/src/web.rs",
    });
  }
});

test("los args ilegibles se muestran crudos", () => {
  expect(describeTool("bash", "no es json", work)).toEqual({ dir: "", text: "no es json" });
});

test("una tool sin campos propios se muestra como json", () => {
  expect(describeTool("rara", '{"a":1}', work)).toEqual({ dir: "", text: '{"a":1}' });
});
