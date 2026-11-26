// Formatação mínima do texto dos modelos: parágrafos, listas, títulos, código,
// negrito e citações. Tudo vira nós do Lit; nenhum HTML do texto é
// interpretado e nenhum endereço externo vira link navegável.
import { html, type TemplateResult } from "lit";
import type { Citacao } from "./tipos";

type Abrir = (c: Citacao) => void;

function emLinha(texto: string, citacoes: Map<string, Citacao>, abrir: Abrir): unknown[] {
  const partes: unknown[] = [];
  const padrao = /\*\*([^*]+)\*\*|`([^`]+)`|\[([TFW][0-9a-f]+)\]/g;
  let fim = 0;
  for (const m of texto.matchAll(padrao)) {
    partes.push(texto.slice(fim, m.index));
    fim = m.index + m[0].length;
    if (m[1] !== undefined) {
      partes.push(html`<strong>${m[1]}</strong>`);
    } else if (m[2] !== undefined) {
      partes.push(html`<code>${m[2]}</code>`);
    } else {
      const c = citacoes.get(m[3]!);
      // Uma marca sem fonte conhecida fica como texto.
      partes.push(
        c
          ? html`<button type="button" class="citacao" title=${descrever(c)} @click=${() => abrir(c)}>${c.marca}</button>`
          : m[0],
      );
    }
  }
  partes.push(texto.slice(fim));
  return partes;
}

export function descrever(c: Citacao) {
  return c.url ? `${c.titulo} — ${c.url}` : `${c.titulo}, página ${c.pagina}`;
}

export function formatar(texto: string, citacoes: Citacao[] = [], abrir: Abrir = () => {}): TemplateResult {
  const mapa = new Map(citacoes.map((c) => [c.marca, c]));
  const linha = (t: string) => emLinha(t, mapa, abrir);
  const blocos: TemplateResult[] = [];
  const linhas = texto.replace(/\r/g, "").split("\n");
  for (let i = 0; i < linhas.length; ) {
    const l = linhas[i]!;
    if (l.startsWith("```")) {
      const codigo: string[] = [];
      for (i++; i < linhas.length && !linhas[i]!.startsWith("```"); i++) codigo.push(linhas[i]!);
      i++;
      blocos.push(html`<pre><code>${codigo.join("\n")}</code></pre>`);
      continue;
    }
    const titulo = /^(#{1,4})\s+(.*)/.exec(l);
    if (titulo) {
      blocos.push(html`<h3>${linha(titulo[2]!)}</h3>`);
      i++;
      continue;
    }
    const marcador = /^\s*(?:[-*]|\d+[.)])\s+/;
    if (marcador.test(l)) {
      const numerada = /^\s*\d/.test(l);
      const itens: TemplateResult[] = [];
      for (; i < linhas.length && marcador.test(linhas[i]!); i++) {
        itens.push(html`<li>${linha(linhas[i]!.replace(marcador, ""))}</li>`);
      }
      blocos.push(numerada ? html`<ol>${itens}</ol>` : html`<ul>${itens}</ul>`);
      continue;
    }
    if (!l.trim()) {
      i++;
      continue;
    }
    const paragrafo: string[] = [];
    for (; i < linhas.length && linhas[i]!.trim() && !/^(```|#{1,4}\s|\s*(?:[-*]|\d+[.)])\s+)/.test(linhas[i]!); i++) {
      paragrafo.push(linhas[i]!);
    }
    blocos.push(html`<p>${linha(paragrafo.join(" "))}</p>`);
  }
  return html`<div class="texto">${blocos}</div>`;
}
