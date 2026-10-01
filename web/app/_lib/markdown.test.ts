import { test, expect } from "bun:test";
import { markdown, inline } from "./markdown";

test("un párrafo partido en varias líneas es un solo párrafo", () => {
  expect(markdown("una linea\nque sigue\n\ny otro")).toBe("<p>una linea que sigue</p><p>y otro</p>");
});

test("los bloques pegados sin línea en blanco arrancan otro bloque", () => {
  expect(markdown("texto\n# título")).toBe("<p>texto</p><h2>título</h2>");
  expect(markdown("texto\n- uno")).toBe("<p>texto</p><ul><li>uno</li></ul>");
});

test("las listas se anidan por indentación", () => {
  expect(markdown("- uno\n  - sub\n  - sub dos\n- dos")).toBe("<ul><li>uno<ul><li>sub</li><li>sub dos</li></ul></li><li>dos</li></ul>");
});

test("una lista se corta cuando el texto vuelve al margen", () => {
  expect(markdown("- uno\n- dos\n\ntexto")).toBe("<ul><li>uno</li><li>dos</li></ul><p>texto</p>");
});

test("las listas ordenadas se numeran y pueden mezclarse", () => {
  expect(markdown("1. uno\n2. dos")).toBe("<ol><li>uno</li><li>dos</li></ol>");
  expect(markdown("- uno\n1. dos")).toBe("<ul><li>uno</li></ul><ol><li>dos</li></ol>");
});

test("la continuación de un ítem va dentro del ítem", () => {
  expect(markdown("- uno\n  y sigue\n- dos")).toBe("<ul><li>uno y sigue</li><li>dos</li></ul>");
});

test("un ítem puede llevar un bloque de código", () => {
  expect(markdown("- corré esto:\n\n  ```bash\n  ls -la\n  ```")).toBe('<ul><li><p>corré esto:</p><pre><code>ls -la</code></pre></li></ul>');
});

test("las tareas salen con su casilla", () => {
  expect(markdown("- [ ] pendiente\n- [x] hecho")).toBe('<ul><li><input type="checkbox" disabled> pendiente</li><li><input type="checkbox" disabled checked> hecho</li></ul>');
});

test("los headings bajan a h2 y sueltan los # de cierre", () => {
  expect(markdown("# uno")).toBe("<h2>uno</h2>");
  expect(markdown("### tres ##")).toBe("<h4>tres</h4>");
  expect(markdown("#hashtag")).toBe("<p>#hashtag</p>");
});

test("la regla horizontal es un hr", () => {
  expect(markdown("texto\n\n---\n\ntexto")).toBe("<p>texto</p><hr><p>texto</p>");
  expect(markdown("***")).toBe("<hr>");
});

test("la cita agrupa sus líneas y bloquea adentro", () => {
  expect(markdown("> una cita\n> con dos líneas")).toBe("<blockquote><p>una cita con dos líneas</p></blockquote>");
  expect(markdown("> - uno\n> - dos")).toBe("<blockquote><ul><li>uno</li><li>dos</li></ul></blockquote>");
});

test("el código conserva sus líneas y su lenguaje", () => {
  expect(markdown("```rust\nlet x = 1;\n```")).toBe('<pre><code><span class="tok-keyword">let</span> x = <span class="tok-number">1</span>;</code></pre>');
  expect(markdown("~~~\nplano\n~~~")).toBe("<pre><code>plano</code></pre>");
  expect(markdown("```rust title=x\nlet x = 1;\n```")).toBe('<pre><code><span class="tok-keyword">let</span> x = <span class="tok-number">1</span>;</code></pre>');
});

test("un bloque de código abierto a mitad del stream no rompe", () => {
  expect(markdown("```bash\nls")).toBe('<pre><code>ls</code></pre>');
});

test("las tablas llevan encabezado, alineación y celdas", () => {
  expect(markdown("| a | b |\n| :-- | --: |\n| 1 | 2 |")).toBe('<div class="table-wrap"><table><thead><tr><th>a</th><th class="right">b</th></tr></thead><tbody><tr><td>1</td><td class="right">2</td></tr></tbody></table></div>');
});

test("una fila con pipes y sin regla no es tabla", () => {
  expect(markdown("| a | b |")).toBe("<p>| a | b |</p>");
});

test("el énfasis cruza asteriscos y se anida", () => {
  expect(inline("**hola *mundo* chau**")).toBe("<strong>hola <em>mundo</em> chau</strong>");
  expect(inline("***los dos***")).toBe("<strong><em>los dos</em></strong>");
  expect(inline("__fuerte__ y _cursiva_")).toBe("<strong>fuerte</strong> y <em>cursiva</em>");
});

test("los asteriscos sueltos quedan", () => {
  expect(inline("2 * 3 * 4")).toBe("2 * 3 * 4");
  expect(inline("snake_case_name")).toBe("snake_case_name");
  expect(inline("a * b")).toBe("a * b");
});

test("el código en línea admite cualquier cantidad de backticks", () => {
  expect(inline("usa `ls`")).toBe("usa <code>ls</code>");
  expect(inline("usa `` `ls` ``")).toBe("usa <code>`ls`</code>");
  expect(inline("con < y & adentro")).toBe("con &lt; y &amp; adentro");
});

test("el tachado", () => {
  expect(inline("~~viejo~~")).toBe("<del>viejo</del>");
});

test("los links y las URLs peladas", () => {
  expect(inline("[texto](https://ejemplo.com/a)")).toBe('<a href="https://ejemplo.com/a" target="_blank" rel="noreferrer">texto</a>');
  expect(inline("mirá https://ejemplo.com/x.")).toBe('mirá <a href="https://ejemplo.com/x" target="_blank" rel="noreferrer">https://ejemplo.com/x</a>.');
  expect(inline("(https://ejemplo.com/x)")).toBe('(<a href="https://ejemplo.com/x" target="_blank" rel="noreferrer">https://ejemplo.com/x</a>)');
});

test("el escape de puntuación", () => {
  expect(inline("un \\* literal")).toBe("un * literal");
  expect(inline("\\`no es código\\`")).toBe("`no es código`");
});

test("el HTML del texto se escapa", () => {
  expect(markdown("<script>alert(1)</script>")).toBe("<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>");
});

test("un link con esquema peligroso queda literal", () => {
  expect(inline("[a](javascript:alert(1))")).toBe("[a](javascript:alert(1))");
});

test("en Rust una vida no se come el resto de la línea como texto", () => {
  const html = markdown("```rust\nfn uno(nota: &'static str) -> u8 { 1 }\n```");
  expect(Boolean(!html.includes("tok-string"))).toBe(true);
});

test("un carácter de Rust sí es un texto, con o sin escape", () => {
  const html = markdown("```rust\nlet barra = '/'; let salto = '\\n';\n```");
  expect((html.match(/tok-string/g) || []).length).toBe(2);
});

test("en Python el apóstrofo sigue abriendo un texto", () => {
  const html = markdown("```python\nsaludo = 'hola'\n```");
  expect(Boolean(html.includes(`<span class="tok-string">&#39;hola&#39;</span>`))).toBe(true);
});

test("en JavaScript el apóstrofo también", () => {
  const html = markdown("```js\nconst saludo = 'hola';\n```");
  expect(Boolean(html.includes(`<span class="tok-string">&#39;hola&#39;</span>`))).toBe(true);
});
