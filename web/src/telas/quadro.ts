// Telas que o modelo escolhe e preenche. O daemon já validou os dados.
import { html, nothing } from "lit";
import type { Quadro } from "../tipos";

export function indicadores(lista: { rotulo: string; valor: string; detalhe?: string }[]) {
  return html`<div class="indicadores">
    ${lista.map(
      (i) => html`<div class="indicador">
        <span class="suave pequeno">${i.rotulo}</span><b>${i.valor}</b>
        ${i.detalhe ? html`<span class="suave pequeno">${i.detalhe}</span>` : nothing}
      </div>`,
    )}
  </div>`;
}

export function quadro(q: Quadro) {
  const d = q.dados;
  let corpo;
  if (q.tela === "indicadores") {
    corpo = indicadores(d.indicadores);
  } else if (q.tela === "tabela") {
    corpo = html`<div class="rolavel"><table>
      <thead><tr>${(d.colunas as string[]).map((c) => html`<th>${c}</th>`)}</tr></thead>
      <tbody>${(d.linhas as string[][]).map((l) => html`<tr>${l.map((v) => html`<td>${v}</td>`)}</tr>`)}</tbody>
    </table></div>`;
  } else {
    return html`<p class="erro-texto">Tela desconhecida: ${q.tela}</p>`;
  }
  return html`<section class="cartao" aria-label=${d.titulo}><h3>${d.titulo}</h3>${corpo}</section>`;
}
