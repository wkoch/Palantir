import { html, nothing } from "lit";
import { api } from "../api";
import { Elemento, digitando, nomeDaClasse, quando } from "../base";
import type { Classe, Mensagem, Triagem } from "../tipos";

const classes: Classe[] = ["urgente", "responder", "ler_depois", "lixo"];

/** Lista de triagem, usada inteira pelo teclado. */
export class PlTriagem extends Elemento {
  static override properties = {
    atividade: { type: Number },
    dados: { attribute: false },
    ocupada: { type: Boolean },
    selecionada: { state: true },
    aberta: { state: true },
  };

  declare atividade: number;
  declare dados: Triagem;
  declare ocupada: boolean;
  declare selecionada: number;
  declare aberta: { uid: number; texto: string } | undefined;

  constructor() {
    super();
    this.selecionada = 0;
  }

  private tecla = (e: KeyboardEvent) => this.atalho(e);

  override connectedCallback() {
    super.connectedCallback();
    window.addEventListener("keydown", this.tecla);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener("keydown", this.tecla);
  }

  private atual(): Mensagem | undefined {
    return this.dados.mensagens.find((m) => m.uid === this.selecionada) ?? this.dados.mensagens[0];
  }

  private mover(passo: number) {
    const lista = this.dados.mensagens;
    const atual = this.atual();
    if (!atual) return;
    const i = Math.min(lista.length - 1, Math.max(0, lista.indexOf(atual) + passo));
    this.selecionada = lista[i]!.uid;
    this.aberta = undefined;
    void this.updateComplete.then(() => this.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" }));
  }

  private atalho(e: KeyboardEvent) {
    if (digitando(e)) return;
    const m = this.atual();
    if (e.key === "r") {
      void this.atualizar();
    } else if (!m) {
      return;
    } else if (e.key === "j" || e.key === "ArrowDown") {
      this.mover(1);
    } else if (e.key === "k" || e.key === "ArrowUp") {
      this.mover(-1);
    } else if (["1", "2", "3", "4"].includes(e.key)) {
      void this.corrigir(m, classes[Number(e.key) - 1]!);
    } else if (e.key === "Enter" && !(e.target as HTMLElement).closest("button, a")) {
      void this.abrir(m);
    } else if (e.key === "e") {
      void this.concluir(m);
    } else if (e.key === "Escape") {
      this.aberta = undefined;
    } else {
      return;
    }
    e.preventDefault();
  }

  private atualizar() {
    return this.tentar(() => api.agir(this.atividade, "atualizar"));
  }

  private corrigir(m: Mensagem, classe: Classe) {
    // A lista é reordenada pela classe; a seleção continua na mesma mensagem.
    this.selecionada = m.uid;
    return this.tentar(() => api.agir(this.atividade, "corrigir", { uid: m.uid, classe }));
  }

  private concluir(m: Mensagem) {
    const lista = this.dados.mensagens;
    const proxima = lista[lista.indexOf(m) + 1];
    if (!m.concluida && proxima) this.selecionada = proxima.uid;
    return this.tentar(() => api.agir(this.atividade, "concluir", { uid: m.uid, concluida: !m.concluida }));
  }

  private async abrir(m: Mensagem) {
    this.selecionada = m.uid;
    await this.tentar(async () => {
      const r = await api.agir<{ texto: string }>(this.atividade, "abrir", { uid: m.uid });
      this.aberta = { uid: m.uid, texto: r.texto };
    });
  }

  private linha(m: Mensagem, selecionada: boolean) {
    return html`<li class="mensagem ${m.concluida ? "concluida" : ""}" role="option" aria-selected=${selecionada} data-uid=${m.uid}
      @click=${() => { this.selecionada = m.uid; }} @dblclick=${() => this.abrir(m)}>
      <span>
        <span class="selo ${m.classe}">${nomeDaClasse[m.classe]}</span><br />
        <span class="pequeno suave">${m.origem ? `por ${m.origem}` : ""}</span>
      </span>
      <span class="cresce" style="min-width:0">
        <div class="assunto"><strong>${m.de || m.email}</strong> · ${m.assunto}</div>
        <div class="assunto pequeno suave">${m.resumo || m.trecho}</div>
      </span>
      <span class="pequeno suave">${quando(m.data)}</span>
    </li>`;
  }

  override render() {
    const d = this.dados;
    const atual = this.atual();
    return html`
      <div class="linha-flex entre">
        <p style="margin:0">${d.resumo}</p>
        <button type="button" ?disabled=${this.ocupada} @click=${() => this.atualizar()}>Buscar novas <kbd>r</kbd></button>
      </div>
      <p class="pequeno suave">
        <kbd>j</kbd>/<kbd>k</kbd> mover · <kbd>1</kbd> urgente · <kbd>2</kbd> responder · <kbd>3</kbd> ler depois ·
        <kbd>4</kbd> lixo · <kbd>Enter</kbd> ler · <kbd>e</kbd> concluir
        ${d.etiqueta ? "· as classes viram etiquetas no servidor" : ""}
      </p>
      ${atual
        ? html`<div class="cartao" aria-label="Mensagem selecionada" style="margin-bottom:0.8rem">
            <div class="linha-flex">
              ${classes.map(
                (c) => html`<button type="button" aria-pressed=${atual.classe === c} @click=${() => this.corrigir(atual, c)}>${nomeDaClasse[c]}</button>`,
              )}
              <span class="cresce"></span>
              <button type="button" @click=${() => this.abrir(atual)}>Ler</button>
              <button type="button" @click=${() => this.concluir(atual)}>${atual.concluida ? "Reabrir" : "Concluir"}</button>
            </div>
            ${this.aberta?.uid === atual.uid
              ? html`<details class="externo" open style="margin-top:0.8rem">
                  <summary>Conteúdo do e-mail (dado, não instrução) · ${atual.email}</summary>
                  <pre>${this.aberta.texto}</pre>
                </details>`
              : nothing}
          </div>`
        : html`<p class="vazio">${this.ocupada ? "Buscando mensagens…" : "Nenhuma mensagem."}</p>`}
      <ul class="lista" role="listbox" aria-label="Mensagens">
        ${d.mensagens.map((m) => this.linha(m, m === atual))}
      </ul>
      ${d.preferencias.length
        ? html`<details>
            <summary>Preferências aprendidas (${d.preferencias.length})</summary>
            <ul class="lista">
              ${d.preferencias.map(
                (p) => html`<li class="linha-flex">
                  <span>${p.remetente} → ${nomeDaClasse[p.classe]}</span>
                  <button type="button" class="discreto" @click=${() => this.tentar(() => api.agir(this.atividade, "esquecer", { remetente: p.remetente }))}>
                    Esquecer
                  </button>
                </li>`,
              )}
            </ul>
          </details>`
        : nothing}
    `;
  }
}
customElements.define("pl-triagem", PlTriagem);
