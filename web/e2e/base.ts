// Cada teste ganha uma instalação própria: simulador e daemon de verdade numa
// pasta temporária, com portas livres.
import { test as base, expect, type Page } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { binarios } from "./caminhos";
import { relatorio } from "./relatorio-cobertura";

async function portaLivre(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = createServer();
    s.once("error", reject);
    s.listen(0, "127.0.0.1", () => {
      const { port } = s.address() as { port: number };
      s.close(() => resolve(port));
    });
  });
}

function aguardar(p: ChildProcess, padrao: RegExp, nome: string): Promise<string> {
  return new Promise((resolve, reject) => {
    let saida = "";
    const prazo = setTimeout(() => reject(new Error(`${nome} não ficou pronto:\n${saida}`)), 20_000);
    const ler = (b: Buffer) => {
      saida += b.toString();
      if (padrao.test(saida)) {
        clearTimeout(prazo);
        resolve(saida);
      }
    };
    p.stdout?.on("data", ler);
    p.stderr?.on("data", ler);
    p.once("exit", (c) => reject(new Error(`${nome} saiu com ${c}:\n${saida}`)));
  });
}

async function encerrar(p: ChildProcess | undefined) {
  if (!p || p.exitCode !== null) return;
  const fim = new Promise((r) => p.once("exit", r));
  p.kill("SIGTERM");
  await fim;
}

export class Sistema {
  private daemon?: ChildProcess;
  private simulador?: ChildProcess;
  url = "";
  web = "";
  chave = "";
  pasta = "";

  async iniciar() {
    this.pasta = await mkdtemp(join(tmpdir(), "pl-"));
    const porta = await portaLivre();
    this.url = `http://127.0.0.1:${porta}`;
    this.simulador = spawn(join(binarios, "palantir-simulador"), [
      "--dir", this.pasta, "--endereco", `127.0.0.1:${porta}`, "--modelo", "127.0.0.1:0",
      "--imap", "127.0.0.1:0", "--web", "127.0.0.1:0", "--atraso", "40ms",
      "--ferramentas", join(binarios, "palantir-tool"),
    ]);
    const saida = await aguardar(this.simulador, /E-mail novo:.*\/entregar/, "simulador");
    this.web = /Página de exemplo:\s+(http:\/\/[\d.:]+)\/pagina/.exec(saida)![1]!;
    await this.ligar();
  }

  async ligar() {
    this.daemon = spawn(join(binarios, "palantird"), ["--dir", this.pasta]);
    await aguardar(this.daemon, /palantird pronto/, "palantird");
    this.chave = (await readFile(join(this.pasta, "config", "acesso"), "utf8")).trim();
  }

  /** Fala com a API como o dono da instalação. */
  api(rota: string, opcoes: RequestInit = {}) {
    const cabecalhos = { "X-Palantir": "1", Authorization: `Bearer ${this.chave}`, ...(opcoes.headers as object) };
    return fetch(`${this.url}${rota}`, { ...opcoes, headers: cabecalhos });
  }

  /** Reinicia o daemon sem mecanismo de busca. */
  async semBusca() {
    await this.desligar();
    const arquivo = join(this.pasta, "config", "config.toml");
    const config = await readFile(arquivo, "utf8");
    await writeFile(arquivo, config.replace(/(\[busca\]\nurl = ).*/, '$1""'));
    await this.ligar();
  }

  async desligar() {
    await encerrar(this.daemon);
  }

  exemplo(nome: string) {
    return join(this.pasta, "exemplos", nome);
  }

  async diario(): Promise<{ tipo: string; nome: string; decisao?: string; detalhe?: string }[]> {
    const t = await readFile(join(this.pasta, "dados", "diario.jsonl"), "utf8");
    return t.trim().split("\n").map((l) => JSON.parse(l));
  }

  async entregar(de: string, assunto: string) {
    const r = await fetch(`${this.web}/entregar`, { method: "POST", body: new URLSearchParams({ de, assunto }) });
    expect(r.status).toBe(204);
  }

  /** Importa os documentos de exemplo pela API e espera a extração. */
  async importarExemplos() {
    const r = await this.api("/api/acervo/importar-pasta", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ caminho: join(this.pasta, "exemplos") }),
    });
    expect(r.ok).toBeTruthy();
    await expect
      .poll(async () => {
        const d = await (await this.api("/api/acervo/documentos")).json();
        return d.numeros.documentos === 3 && d.numeros.pendentes === 0 && d.numeros.vetores >= 3;
      })
      .toBe(true);
  }

  async encerrar() {
    await this.desligar();
    await encerrar(this.simulador);
    await rm(this.pasta, { recursive: true, force: true });
  }
}

export const test = base.extend<{ sistema: Sistema; semErros: void }>({
  sistema: async ({}, usar) => {
    const s = new Sistema();
    try {
      await s.iniciar();
      await usar(s);
    } finally {
      await s.encerrar();
    }
  },
  // Coleta a cobertura e reprova o teste se a interface lançar um erro.
  semErros: [
    async ({ page }, usar) => {
      const erros: string[] = [];
      const externos: string[] = [];
      page.on("pageerror", (e) => erros.push(e.message));
      page.on("console", (m) => {
        // Respostas HTTP de erro são esperadas em alguns testes. A moldura do
        // original recusa a injeção do próprio Playwright, como deve.
        const esperado = /Failed to load resource|WebSocket connection|Blocked script execution in '[^']+\/original(\?bilhete=[\w-]+)?'/;
        if (m.type() === "error" && !esperado.test(m.text())) erros.push(m.text());
      });
      page.on("request", (r) => {
        if (!/^(http|ws):\/\/(127\.0\.0\.1|localhost)[:/]|^(data|blob|chrome-extension):/.test(r.url())) externos.push(r.url());
      });
      await page.coverage.startJSCoverage({ resetOnNavigation: false });
      await usar();
      await relatorio().add(await page.coverage.stopJSCoverage());
      expect(erros, "erros na interface").toEqual([]);
      expect(externos, "a interface só fala com a própria máquina").toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };

/** Abre a interface, autorizada, e espera o daemon responder. */
export async function abrir(page: Page, s: Sistema, rota = "") {
  await page.goto(`${s.url}/#entrar=${s.chave}`);
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
  if (rota) await page.goto(`${s.url}/${rota}`);
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
}

export const estado = (page: Page) => page.locator('[data-teste="estado"]');

/** Cria uma atividade pela fila de atenção. */
export async function criar(page: Page, tipo: string, objetivo: string) {
  await page.getByRole("group", { name: "Tipo de atividade" }).getByRole("button", { name: tipo }).click();
  if (objetivo) await page.locator("textarea[name=objetivo]").fill(objetivo);
  await page.getByRole("button", { name: "Começar" }).click();
  await expect(page).toHaveURL(/#\/atividade\/\d+/);
}
