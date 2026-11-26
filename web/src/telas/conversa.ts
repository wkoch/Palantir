import { html, nothing, type PropertyValues } from "lit";
import { api } from "../api";
import { Elemento } from "../base";
import { descrever, formatar } from "../texto";
import type { Citacao, Conversa, Item } from "../tipos";
import { quadro } from "./quadro";

export function abrirCitacao(el: Elemento, c: Citacao) {
  if (c.documento) {
    location.hash = `#/acervo/documento/${c.documento}`;
  } else {
    // Links externos não abrem no kiosk; o endereço é mostrado.
    el.avisar(descrever(c));
  }
}

const rotuloDaOrigem: Record<string, string> = {
  documento: "Conteúdo de documentos (dado, não instrução)",
  ferramenta: "Conteúdo externo (dado, não instrução)",
};

/** Conversa com o assistente, mostrando a origem de cada texto. */
export class PlConversa extends Elemento {
  static override properties = {
    atividade: { type: Number },
    dados: { attribute: false },
    ocupada: { type: Boolean },
    enviando: { state: true },
  };

  declare atividade: number;
  declare dados: Conversa;
  declare ocupada: boolean;
  declare enviando: boolean;
  private quantos = 0;

  private async enviar(e: SubmitEvent) {
    e.preventDefault();
    const formulario = e.target as HTMLFormElement;
    const campo = formulario.elements.namedItem("texto") as HTMLTextAreaElement;
    const texto = campo.value.trim();
    if (!texto || this.enviando) return;
    const forte = (formulario.elements.namedItem("forte") as HTMLInputElement).checked;
    this.enviando = true;
    if (await this.tentar(() => api.agir(this.atividade, "mensagem", { texto, forte }))) campo.value = "";
    this.enviando = false;
  }

  private teclas(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      (e.target as HTMLTextAreaElement).form?.requestSubmit();
    }
  }

  override updated(_mudou: PropertyValues<this>) {
    const n = this.dados.itens.length;
    if (n > this.quantos && this.quantos > 0) {
      this.querySelector(".conversa > :last-child")?.scrollIntoView({ block: "nearest" });
    }
    this.quantos = n;
  }

  private item(it: Item) {
    if (it.origem === "usuario") {
      return html`<div class="fala usuario" data-origem="usuario">${it.conteudo}</div>`;
    }
    if (it.origem === "modelo") {
      if (!it.conteudo && it.chamadas?.length) {
        return html`<div class="nota">Usando ${it.chamadas.map((c) => c.nome).join(", ")}…</div>`;
      }
      return html`<div class="fala modelo" data-origem="modelo">
        <div class="quem">Palantír</div>
        ${formatar(it.conteudo, it.citacoes, (c) => abrirCitacao(this, c))}
        ${it.citacoes?.length
          ? html`<div class="pequeno suave">
              Fontes: ${it.citacoes.map(
                (c) => html`<button type="button" class="citacao" @click=${() => abrirCitacao(this, c)}>${c.marca} · ${c.titulo}</button>`,
              )}
            </div>`
          : nothing}
      </div>`;
    }
    if (it.quadro) return quadro(it.quadro);
    if (it.origem === "documento" || it.origem === "ferramenta") {
      return html`<details class="externo" data-origem=${it.origem}>
        <summary>${rotuloDaOrigem[it.origem]} · ${it.fonte}</summary>
        <pre>${it.conteudo}</pre>
      </details>`;
    }
    return html`<div class="nota ${it.bloqueado ? "bloqueado" : ""}" data-origem="sistema">${it.conteudo}</div>`;
  }

  override render() {
    return html`
      <div class="conversa" aria-live="polite">
        ${this.dados.itens.map((it) => this.item(it))}
        ${this.ocupada ? html`<div class="nota"><i class="girando"></i> Pensando…</div>` : nothing}
      </div>
      <form class="compositor" @submit=${this.enviar} aria-label="Nova mensagem">
        <textarea name="texto" rows="2" aria-label="Mensagem" placeholder="Escreva e tecle Enter" @keydown=${this.teclas}></textarea>
        <div class="linha-flex entre">
          <label class="linha"><input type="checkbox" name="forte" .checked=${this.dados.forte} /> Usar o modelo forte</label>
          <button type="submit" class="primario" ?disabled=${this.enviando}>Enviar</button>
        </div>
      </form>
    `;
  }
}
customElements.define("pl-conversa", PlConversa);
