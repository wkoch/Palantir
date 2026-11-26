// Compila a interface para internal/ui/dist, de onde o daemon a embute.
import { build } from "esbuild";
import { copyFile, readdir, rm } from "node:fs/promises";

const saida = new URL("../internal/ui/dist/", import.meta.url).pathname;
// Com --cobertura, o mapa de fontes acompanha o pacote para medir os testes.
const cobertura = process.argv.includes("--cobertura");

for (const nome of await readdir(saida)) {
  if (nome !== ".gitignore") await rm(saida + nome, { recursive: true });
}
await build({
  entryPoints: ["src/main.ts", "src/estilo.css"],
  outdir: saida,
  entryNames: "[name]",
  bundle: true,
  format: "esm",
  target: "chrome130",
  minify: !cobertura,
  sourcemap: cobertura ? "inline" : false,
  legalComments: "eof",
  logLevel: "warning",
});
for (const nome of ["index.html", "icone.svg"]) await copyFile("src/" + nome, saida + nome);
