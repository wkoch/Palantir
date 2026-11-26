import { html, nothing } from "lit";
import { api } from "../api";
import { Elemento, quando } from "../base";
import type { Fonte, Relatorio, Secao } from "../tipos";

/** Documento com fontes e comentários por seção. */
export class PlRelatorio extends Elemento {
  static override properties = {
    atividade: { type: Number },
    dados: { attribute: false },
    ocupada: { type: Boolean },
    fonte: { state: true },
  };

  declare atividade: number;
  declare dados: Relatorio;
  declare ocupada: boolean;
  declare fonte: string;

  constructor() {
    super();
    this.fonte = "";
  }

  private async responder(e: SubmitEvent) {
    e.preventDefault();
    const respostas = [...new FormData(e.target as HTMLFormElement).values()].map(String);
    await this.tentar(() => api.agir(this.atividade, "responder", { respostas }));
  }

  private async comentar(e: SubmitEvent, secao: number) {
    e.preventDefault();
    const formulario = e.target as HTMLFormElement;
    const texto = String(new FormData(formulario).get("texto") ?? "").trim();
    if (!texto) return;
    if (await this.tentar(() => api.agir(this.atividade, "comentar", { secao, texto }))) formulario.reset();
  }

  private perguntas() {
    return html`<form class="cartao" @submit=${this.responder} aria-label="Perguntas">
      <h2>Antes de escrever</h2>
      ${this.dados.perguntas.map(
        (p, i) => html`<label for="r${i}">${p.texto}</label><input id="r${i}" name="r${i}" required value=${p.resposta} />`,
      )}
      <p></p>
      <button type="submit" class="primario">Responder e continuar</button>
    </form>`;
  }

  private marcas(ids: string[]) {
    return ids.map(
      (id) => html`<button type="button" class="citacao" aria-pressed=${this.fonte === id} @click=${() => (this.fonte = id)}>${id}</button>`,
    );
  }

  private secao(s: Secao) {
    return html`<section class="cartao" aria-label=${s.titulo} data-secao=${s.id}>
      <div class="linha-flex entre">
        <h2 style="margin:0">${s.titulo}</h2>
        <span class="selo ${s.estado}">${s.estado === "pendente" ? html`<i class="girando"></i> escrevendo` : s.estado}</span>
      </div>
      ${s.paragrafos.map((p) => html`<p>${p.texto} ${this.marcas(p.fontes)}</p>`)}
      ${!s.paragrafos.length && s.estado !== "pendente"
        ? html`<p class="suave">Nenhuma fonte sustentou esta seção.</p>`
        : nothing}
      ${s.descartadas ? html`<p class="pequeno erro-texto">${s.descartadas} afirmação(ões) sem fonte foram descartadas.</p>` : nothing}
      ${s.aviso ? html`<p class="pequeno suave">${s.aviso}</p>` : nothing}
      ${s.comentarios.map((c) => html`<p class="pequeno suave">Correção pedida: ${c.texto}${c.atendido ? " (atendida)" : ""}</p>`)}
      <form class="linha-flex" @submit=${(e: SubmitEvent) => this.comentar(e, s.id)}>
        <input class="cresce" name="texto" aria-label=${`Correção para ${s.titulo}`} placeholder="Peça uma correção só nesta seção" style="width:auto" />
        <button type="submit">Corrigir</button>
        <button type="button" ?disabled=${s.estado === "pendente"} aria-pressed=${s.estado === "aprovada"}
          @click=${() => this.tentar(() => api.agir(this.atividade, "aprovar_secao", { secao: s.id, aprovada: s.estado !== "aprovada" }))}>
          ${s.estado === "aprovada" ? "Aprovada ✓" : "Aprovar seção"}
        </button>
      </form>
    </section>`;
  }

  private painelDaFonte(f: Fonte | undefined) {
    if (!f) return html`<aside class="vazio">Clique numa marca como F1 para ver a fonte.</aside>`;
    return html`<aside class="cartao fonte-aberta" aria-label="Fonte">
      <h3>${f.id} · ${f.titulo}</h3>
      <p class="pequeno suave">
        ${f.tipo === "documento"
          ? html`Página ${f.pagina} · <a href="#/acervo/documento/${f.documento}">abrir no acervo</a>`
          : html`<span style="overflow-wrap:anywhere">${f.url}</span>`}
      </p>
      <pre>${f.texto}</pre>
    </aside>`;
  }

  override render() {
    const d = this.dados;
    if (!d.planejado) {
      if (d.perguntas.length && !this.ocupada) return this.perguntas();
      return html`<p class="suave"><i class="girando"></i> Planejando o relatório…</p>`;
    }
    const prontas = d.secoes.every((s) => s.estado !== "pendente");
    return html`<div class="duas">
      <div class="documento">
        ${d.secoes.map((s) => this.secao(s))}
        <div class="linha-flex" style="margin-top:1rem">
          <button type="button" class="primario" ?disabled=${!prontas || this.ocupada}
            @click=${() => this.tentar(() => api.agir(this.atividade, "aprovar"), "Versão guardada no acervo.")}>
            Aprovar e guardar no acervo
          </button>
          ${d.versoes.map(
            (v, i) => html`<a href="#/acervo/documento/${v.documento}">Versão ${i + 1} (${quando(v.quando)})</a>`,
          )}
        </div>
      </div>
      ${this.painelDaFonte(d.fontes.find((f) => f.id === this.fonte))}
    </div>`;
  }
}
customElements.define("pl-relatorio", PlRelatorio);
