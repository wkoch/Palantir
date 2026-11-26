import { html, nothing } from "lit";
import { api } from "../api";
import { Visao, nomeDoEstado, tamanho, usd } from "../base";
import type { EntradaDiario, Painel } from "../tipos";
import { indicadores } from "./quadro";

/** Custos, acervo e o diário de auditoria. */
export class PlPainel extends Visao<{ painel: Painel; diario: EntradaDiario[] }> {
  protected override async buscar() {
    const [painel, diario] = await Promise.all([
      api.obter<Painel>("/api/painel"),
      api.obter<{ entradas: EntradaDiario[] | null }>("/api/diario?n=100"),
    ]);
    return { painel, diario: diario.entradas ?? [] };
  }

  private tabela(titulo: string, linhas: Painel["por_modelo"]) {
    if (!linhas.length) return nothing;
    return html`<h2>${titulo}</h2><div class="rolavel"><table>
      <thead><tr><th>${titulo.replace("Por ", "")}</th><th class="numero">Chamadas</th><th class="numero">Tokens</th><th class="numero">Custo</th></tr></thead>
      <tbody>${linhas.map(
        (l) => html`<tr><td>${l.chave}</td><td class="numero">${l.chamadas}</td><td class="numero">${l.tokens.toLocaleString("pt-BR")}</td><td class="numero">${usd(l.custo_usd)}</td></tr>`,
      )}</tbody>
    </table></div>`;
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Carregando…"}</p>`;
    const p = d.painel;
    const fracao = p.teto_mes_usd > 0 ? Math.min(100, (100 * p.gasto_mes_usd) / p.teto_mes_usd) : 100;
    const estados = Object.entries(p.atividades).map(([e, n]) => `${n} ${nomeDoEstado[e] ?? e}`);
    return html`
      <h1>Painel</h1>
      ${indicadores([
        { rotulo: "Gasto no mês", valor: usd(p.gasto_mes_usd), detalhe: `teto ${usd(p.teto_mes_usd)}` },
        { rotulo: "Documentos", valor: String(p.acervo.documentos), detalhe: tamanho(p.acervo.bytes) },
        { rotulo: "Trechos indexados", valor: String(p.acervo.trechos), detalhe: `${p.acervo.vetores} com vetor` },
        { rotulo: "Atividades", valor: String(Object.values(p.atividades).reduce((a, b) => a + b, 0)), detalhe: estados.join(", ") },
      ])}
      <div class="barra" role="progressbar" aria-label="Uso do teto mensal" aria-valuenow=${Math.round(fracao)} aria-valuemin="0" aria-valuemax="100" style="margin-top:0.8rem">
        <i style="width:${fracao}%"></i>
      </div>
      ${this.tabela("Por modelo", p.por_modelo)} ${this.tabela("Por finalidade", p.por_finalidade)}
      <h2>Diário</h2>
      <p class="pequeno suave">Cada chamada de modelo, ferramenta e rede, com a decisão tomada.</p>
      <div class="rolavel"><table>
        <thead><tr><th>Quando</th><th>Tipo</th><th>Nome</th><th>Decisão</th><th>Detalhe</th></tr></thead>
        <tbody>${d.diario.map(
          (e) => html`<tr>
            <td class="pequeno">${new Date(e.quando).toLocaleString("pt-BR")}</td>
            <td>${e.tipo}</td>
            <td>${e.atividade ? html`<a href="#/atividade/${e.atividade}">${e.nome}</a>` : e.nome}</td>
            <td><span class="selo ${e.decisao === "bloquear" || e.decisao === "bloqueado" || e.decisao === "erro" ? "erro" : ""}">${e.decisao}</span></td>
            <td class="pequeno">${[e.motivo, e.detalhe?.slice(0, 160), e.custo_usd ? usd(e.custo_usd) : ""].filter(Boolean).join(" · ")}</td>
          </tr>`,
        )}</tbody>
      </table></div>
    `;
  }
}
customElements.define("pl-painel", PlPainel);
