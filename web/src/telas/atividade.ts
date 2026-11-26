import { html, nothing, type PropertyValues } from "lit";
import { keyed } from "lit/directives/keyed.js";
import { api, type Aviso } from "../api";
import { Visao, nomeDoEstado, nomeDoTipo, usd } from "../base";
import type { Detalhe } from "../tipos";

/** Moldura de uma atividade: estado, custo, controles e a tela do tipo. */
export class PlAtividade extends Visao<Detalhe> {
  static override properties = { ...Visao.properties, numero: { type: Number } };

  declare numero: number;

  protected override buscar() {
    return api.obter<Detalhe>(`/api/atividades/${this.numero}`);
  }

  protected override interessa(a: Aviso) {
    return a.tipo === "ajustes" || (a.tipo === "atividade" && a.atividade === this.numero);
  }

  override willUpdate(mudou: PropertyValues<this>) {
    // Trocar de atividade descarta o que era da anterior.
    if (mudou.has("numero") && mudou.get("numero") !== undefined) this.reiniciar();
  }

  private mudar(estado: string) {
    return this.tentar(() => api.enviar(`/api/atividades/${this.numero}/estado`, { estado }));
  }

  private async orcamento(e: SubmitEvent) {
    e.preventDefault();
    const usdNovo = Number(new FormData(e.target as HTMLFormElement).get("usd"));
    await this.tentar(() => api.enviar(`/api/atividades/${this.numero}/orcamento`, { usd: usdNovo }), "Limite atualizado.");
  }

  private tela(d: Detalhe) {
    const id = d.atividade.id;
    switch (d.tela) {
      case "conversa":
        return html`<pl-conversa .atividade=${id} .dados=${d.dados} ?ocupada=${d.atividade.estado === "rodando"}></pl-conversa>`;
      case "relatorio":
        return html`<pl-relatorio .atividade=${id} .dados=${d.dados} ?ocupada=${d.atividade.estado === "rodando"}></pl-relatorio>`;
      case "triagem":
        return html`<pl-triagem .atividade=${id} .dados=${d.dados} ?ocupada=${d.atividade.estado === "rodando"}></pl-triagem>`;
      case "estudo":
        return html`<pl-estudo .atividade=${id} .dados=${d.dados} ?ocupada=${d.atividade.estado === "rodando"}></pl-estudo>`;
    }
    return html`<p class="erro-texto">Tela desconhecida: ${d.tela}</p>`;
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Carregando…"}</p>`;
    const a = d.atividade;
    return keyed(
      a.id,
      html`
        <p class="pequeno"><a href="#/">← Fila de atenção</a></p>
        <div class="linha-flex entre">
          <div class="cresce">
            <h1>${a.titulo}</h1>
            <p class="suave pequeno">
              ${nomeDoTipo[a.tipo] ?? a.tipo} ·
              <span class="selo ${a.estado}" data-teste="estado">${a.estado === "rodando" ? html`<i class="girando"></i> ` : nothing}${nomeDoEstado[a.estado]}</span>
              · gasto ${usd(a.custo_usd)} de ${usd(a.orcamento_usd)}
            </p>
          </div>
          <div class="linha-flex">
            ${a.estado === "rodando" ? html`<button type="button" @click=${() => this.mudar("pausada")}>Pausar</button>` : nothing}
            ${a.estado === "pausada" || a.estado === "esperando_voce" || a.estado === "arquivada"
              ? html`<button type="button" class="primario" @click=${() => this.mudar("rodando")}>Retomar</button>`
              : nothing}
            ${a.estado !== "arquivada"
              ? html`<button type="button" class="discreto" @click=${() => this.mudar("arquivada")}>Arquivar</button>`
              : nothing}
          </div>
        </div>
        <details class="pequeno suave">
          <summary>Limite e histórico</summary>
          <form class="linha-flex" @submit=${this.orcamento}>
            <label class="linha" for="usd">Limite desta atividade (US$)</label>
            <input id="usd" name="usd" type="number" min="0" step="0.01" value=${a.orcamento_usd} style="width:8rem" />
            <button type="submit">Salvar limite</button>
          </form>
          <ul class="lista">
            ${d.eventos.map((e) => html`<li>${new Date(e.quando).toLocaleString("pt-BR")} · ${e.origem} · ${e.tipo}: ${e.texto.slice(0, 200)}</li>`)}
          </ul>
        </details>
        ${a.aviso ? html`<div class="aviso" role="status">${a.aviso}</div>` : nothing}
        ${d.confirmacoes.map((c) => html`<pl-confirmacao .c=${c}></pl-confirmacao>`)}
        ${this.falha ? html`<p class="erro-texto">${this.falha}</p>` : nothing}
        ${this.tela(d)}
      `,
    );
  }
}
customElements.define("pl-atividade", PlAtividade);
