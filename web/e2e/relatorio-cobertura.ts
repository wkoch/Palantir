import MCR from "monocart-coverage-reports";
import { web } from "./caminhos";

export const MINIMO = 80;

export const relatorio = () =>
  MCR({
    outputDir: web + "/cobertura/interface",
    reports: [["console-summary"], ["text-summary"], ["v8"], ["lcovonly"]],
    // Só o código da interface conta; as bibliotecas ficam de fora.
    entryFilter: (e) => e.url.endsWith("/main.js"),
    sourceFilter: (caminho) => caminho.includes("src/") && !caminho.includes("node_modules"),
  });
