// Compila a interface com mapa de fontes e os binários antes dos testes.
import { execFileSync } from "node:child_process";
import { rm } from "node:fs/promises";
import { binarios, raiz, web } from "./caminhos";

export default async function preparar() {
  await rm(web + "/cobertura", { recursive: true, force: true });
  execFileSync("node", ["construir.mjs", "--cobertura"], { cwd: web, stdio: "inherit" });
  execFileSync("go", ["build", "-o", binarios + "/", "./cmd/..."], { cwd: raiz, stdio: "inherit" });
}
