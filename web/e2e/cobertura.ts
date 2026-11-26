// Fecha o relatório de cobertura da interface e reprova a execução completa
// abaixo do mínimo.
import type { FullConfig } from "@playwright/test";
import { MINIMO, relatorio } from "./relatorio-cobertura";

export default async function cobertura(_config: FullConfig) {
  const r = await relatorio().generate();
  if (!r) throw new Error("nenhuma cobertura coletada");
  const { lines, statements, functions, branches } = r.summary as Record<string, { pct: number }>;
  const medidas = { linhas: lines!.pct, comandos: statements!.pct, funções: functions!.pct, ramos: branches!.pct };
  console.log("Cobertura da interface:", medidas);
  // Uma execução filtrada não representa o conjunto.
  const completa = !process.argv.some((a) => /\.spec|^-g$|--grep/.test(a));
  if (completa && (medidas.linhas < MINIMO || medidas.funções < MINIMO || medidas.comandos < MINIMO)) {
    throw new Error(`cobertura da interface abaixo de ${MINIMO}%: ${JSON.stringify(medidas)}`);
  }
}
