import { html, nothing } from "lit";
import { api, conectado, conectar, ouvir } from "./api";
import { Elemento, digitando, formularioDeArquivos } from "./base";
import { resumoDaImportacao } from "./telas/acervo";
import "./telas/ajustes";
import "./telas/atividade";
import "./telas/confirmacao";
import "./telas/conversa";
import "./telas/estudo";
import type { PlFila } from "./telas/fila";
import "./telas/fila";
import "./telas/painel";
import "./telas/relatorio";
import "./telas/triagem";
import type { Estado } from "./tipos";

interface Torrada {
  id: number;
  texto: string;
  erro: boolean;
}

const atalhos: [string, string][] = [
  ["g f", "Fila de atenção"],
  ["g a", "Acervo"],
  ["g p", "Painel"],
  ["g j", "Ajustes"],
  ["n", "Nova atividade"],
  ["?", "Esta ajuda"],
];

/** A moldura: navegação, avisos, atalhos e arrastar para importar. */
class PlApp extends Elemento {
  static override properties = {
    rota: { state: true },
    torradas: { state: true },
    arrastando: { state: true },
    ligado: { state: true },
    estado: { state: true },
  };

  declare rota: string;
  declare torradas: Torrada[];
  declare arrastando: boolean;
  declare ligado: boolean;
  declare estado: Estado | undefined;

  private proximaTorrada = 1;
  private versao = "";
  private prefixo = 0;

  constructor() {
    super();
    this.rota = location.hash;
    this.torradas = [];
    this.arrastando = false;
    this.ligado = true;
  }

  override connectedCallback() {
    super.connectedCallback();
    window.addEventListener("hashchange", () => {
      this.rota = location.hash;
      window.scrollTo(0, 0);
    });
    window.addEventListener("keydown", (e) => this.atalho(e));
    window.addEventListener("dragover", (e) => this.sobre(e));
    window.addEventListener("dragleave", (e) => {
      if (!e.relatedTarget) this.arrastando = false;
    });
    window.addEventListener("drop", (e) => void this.soltar(e));
    this.addEventListener("pl-avisar", (e) => {
      const { texto, erro } = (e as CustomEvent<{ texto: string; erro: boolean }>).detail;
      this.torrada(texto, erro);
    });
    ouvir((a) => {
      if (a.tipo === "conexao") this.ligado = conectado;
      else void this.situacao();
    });
    conectar();
    void this.situacao();
  }

  private async situacao() {
    try {
      this.estado = await api.obter<Estado>("/api/estado");
      // Depois de uma atualização ou de voltar de versão, a interface antiga é
      // recarregada: no kiosk não há botão de recarregar.
      if (this.versao && this.versao !== this.estado.versao) location.reload();
      this.versao = this.estado.versao;
    } catch {
      // A faixa de desconexão já avisa.
    }
  }

  private torrada(texto: string, erro: boolean) {
    const id = this.proximaTorrada++;
    this.torradas = [...this.torradas.slice(-3), { id, texto, erro }];
    setTimeout(() => (this.torradas = this.torradas.filter((t) => t.id !== id)), erro ? 10_000 : 5_000);
  }

  private async atalho(e: KeyboardEvent) {
    if (digitando(e)) return;
    if (this.prefixo && Date.now() - this.prefixo < 1500) {
      this.prefixo = 0;
      const destino = ({ f: "#/", a: "#/acervo", p: "#/painel", j: "#/ajustes" } as Record<string, string>)[e.key];
      if (destino) {
        e.preventDefault();
        // As telas também ouvem o teclado; o atalho de navegação vem primeiro.
        e.stopImmediatePropagation();
        location.hash = destino;
      }
      return;
    }
    if (e.key === "g") {
      this.prefixo = Date.now();
    } else if (e.key === "n") {
      e.preventDefault();
      location.hash = "#/";
      // O evento hashchange é assíncrono; a tela é trocada já.
      this.rota = "#/";
      await this.updateComplete;
      const fila = this.querySelector<PlFila>("pl-fila");
      await fila?.updateComplete;
      fila?.focarNova();
    } else if (e.key === "?") {
      this.querySelector("dialog")?.showModal();
    }
  }

  private sobre(e: DragEvent) {
    if (!e.dataTransfer?.types.includes("Files")) return;
    e.preventDefault();
    this.arrastando = true;
  }

  private async soltar(e: DragEvent) {
    if (!e.dataTransfer?.types.includes("Files")) return;
    e.preventDefault();
    this.arrastando = false;
    const arquivos = [...e.dataTransfer.files];
    if (!arquivos.length) return;
    await this.tentar(async () => {
      const r = await api.enviar<{ relatorio: Parameters<typeof resumoDaImportacao>[0] }>(
        "/api/acervo/importar",
        formularioDeArquivos(arquivos),
      );
      this.torrada(resumoDaImportacao(r.relatorio), r.relatorio.falhas.length > 0);
    });
  }

  private tela() {
    const [, area, a, b] = this.rota.split("/");
    if (area === "atividade" && Number(a) > 0) return html`<pl-atividade .numero=${Number(a)}></pl-atividade>`;
    if (area === "acervo" && a === "documento" && Number(b) > 0) return html`<pl-documento .numero=${Number(b)}></pl-documento>`;
    if (area === "acervo") return html`<pl-acervo></pl-acervo>`;
    if (area === "painel") return html`<pl-painel></pl-painel>`;
    if (area === "ajustes") return html`<pl-ajustes></pl-ajustes>`;
    return html`<pl-fila></pl-fila>`;
  }

  override render() {
    const area = this.rota.split("/")[1] ?? "";
    const link = (destino: string, nome: string, ativa: boolean) =>
      html`<a href=${destino} aria-current=${ativa ? "page" : "false"}>${nome}</a>`;
    const e = this.estado;
    const pendencias = e ? e.confirmacoes.length + e.atividades.filter((x) => x.estado === "esperando_voce").length : 0;
    const modelos = { ok: "modelos ok", sem_chave: "sem chave", sem_rede: "sem rede", recusado: "chave recusada" };
    return html`
      <header class="topo">
        <a class="marca" href="#/"><img src="/icone.svg" alt="" />Palantír</a>
        <nav aria-label="Principal">
          ${link("#/", pendencias ? `Fila (${pendencias})` : "Fila", area === "" || area === "atividade")}
          ${link("#/acervo", "Acervo", area === "acervo")} ${link("#/painel", "Painel", area === "painel")}
          ${link("#/ajustes", "Ajustes", area === "ajustes")}
        </nav>
        ${!this.ligado
          ? html`<span class="selo erro" role="alert">daemon desconectado</span>`
          : e
            ? html`<span class="selo ${e.modelos}" data-teste="modelos">${modelos[e.modelos]}</span>`
            : nothing}
        <button type="button" class="discreto" aria-label="Atalhos de teclado" @click=${() => this.querySelector("dialog")?.showModal()}><kbd>?</kbd></button>
      </header>
      <main class="conteudo">${this.tela()}</main>
      <dialog aria-label="Atalhos de teclado">
        <h2>Atalhos</h2>
        <dl class="atalhos">${atalhos.map(([t, d]) => html`<dt><kbd>${t}</kbd></dt><dd>${d}</dd>`)}</dl>
        <p class="pequeno suave">A triagem e os cartões têm atalhos próprios, mostrados na tela. O console de emergência fica em <kbd>Ctrl+Alt+F2</kbd>.</p>
        <form method="dialog"><button type="submit">Fechar</button></form>
      </dialog>
      ${this.arrastando ? html`<div class="arrasto">Solte para importar no acervo</div>` : nothing}
      <div class="torradas" role="log" aria-live="polite">
        ${this.torradas.map((t) => html`<div class="torrada ${t.erro ? "erro" : ""}">${t.texto}</div>`)}
      </div>
    `;
  }
}
customElements.define("pl-app", PlApp);
