import { html, nothing } from "lit";
import { api } from "../api";
import { Elemento } from "../base";
import type { Confirmacao } from "../tipos";

/** Uma ação esperando o clique do usuário. */
export class PlConfirmacao extends Elemento {
  static override properties = { c: { attribute: false }, titulo: {}, enviando: { state: true } };

  declare c: Confirmacao;
  declare titulo: string;
  declare enviando: boolean;

  constructor() {
    super();
    this.titulo = "";
    this.enviando = false;
  }

  private async decidir(aprovar: boolean) {
    this.enviando = true;
    await this.tentar(() => api.enviar(`/api/confirmacoes/${this.c.id}`, { aprovar }));
    this.enviando = false;
  }

  override render() {
    const url = typeof this.c.argumentos?.["url"] === "string" ? (this.c.argumentos["url"] as string) : "";
    return html`<section class="cartao confirmacao" aria-label="Confirmação pendente">
      <h3>${url ? "Permitir acesso a este endereço?" : `Permitir ${this.c.ferramenta}?`}</h3>
      ${this.titulo
        ? html`<p class="suave pequeno">Pedido por <a href="#/atividade/${this.c.atividade}">${this.titulo}</a></p>`
        : nothing}
      <code>${url || JSON.stringify(this.c.argumentos)}</code>
      <p class="pequeno">Motivo: ${this.c.motivos.join("; ")}.</p>
      <p class="pequeno suave">
        Confira o endereço inteiro: o que estiver nele será enviado para fora da máquina.
      </p>
      <div class="linha-flex">
        <button type="button" class="primario" ?disabled=${this.enviando} @click=${() => this.decidir(true)}>Permitir</button>
        <button type="button" ?disabled=${this.enviando} @click=${() => this.decidir(false)}>Recusar</button>
      </div>
    </section>`;
  }
}
customElements.define("pl-confirmacao", PlConfirmacao);
